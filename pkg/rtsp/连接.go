package rtsp

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

type Conn struct {
	core.Connection
	core.Listener

	// public

	Backchannel bool
	Media       string
	OnClose     func() error
	PacketSize  uint16
	SessionName string
	Timeout     int
	Transport   string // custom transport support, ex. RTSP over WebSocket

	URL *url.URL

	// internal

	auth      *tcp.Auth
	conn      net.Conn
	keepalive int
	mode      core.Mode
	playOK    bool
	playErr   error
	reader    *bufio.Reader
	sequence  int
	session   string
	uri       string

	state   State
	stateMu sync.Mutex

	udpConn []*net.UDPConn
	udpAddr []*net.UDPAddr
}

const (
	ProtoRTSP      = "RTSP/1.0"
	MethodOptions  = "OPTIONS"
	MethodSetup    = "SETUP"
	MethodTeardown = "TEARDOWN"
	MethodDescribe = "DESCRIBE"
	MethodPlay     = "PLAY"
	MethodPause    = "PAUSE"
	MethodAnnounce = "ANNOUNCE"
	MethodRecord   = "RECORD"
)

type State byte

func (s State) String() string {
	switch s {
	case StateNone:
		return "NONE"
	case StateConn:
		return "CONN"
	case StateSetup:
		return MethodSetup
	case StatePlay:
		return MethodPlay
	}
	return strconv.Itoa(int(s))
}

const (
	StateNone State = iota
	StateConn
	StateSetup
	StatePlay
)

func (c *Conn) Handle() (err error) {
	var timeout time.Duration

	switch c.mode {
	case core.ModeActiveProducer:
		var keepaliveDT time.Duration

		if c.keepalive > 5 {
			keepaliveDT = time.Duration(c.keepalive-5) * time.Second
		} else {
			keepaliveDT = 25 * time.Second
		}

		ctx, cancel := context.WithCancel(context.Background())
		go c.handleKeepalive(ctx, keepaliveDT)
		defer cancel()

		if c.Timeout == 0 {
			// 从远端 RTSP 服务器获取帧，例如摄像头
			timeout = time.Second * 5

			if len(c.Receivers) == 0 || c.Transport == "udp" {
				// 仅向摄像头发送音频时
				// https://github.com/AlexxIT/go2rtc/issues/659
				timeout += keepaliveDT
			}
		} else {
			timeout = time.Second * time.Duration(c.Timeout)
		}

	case core.ModePassiveProducer:
		// 从远端 RTSP 客户端获取帧，例如 FFmpeg
		if c.Timeout == 0 {
			timeout = time.Second * 15
		} else {
			timeout = time.Second * time.Duration(c.Timeout)
		}

	case core.ModePassiveConsumer:
		// 向远端 RTSP 客户端推送帧，例如 VLC
		timeout = time.Second * 60

	default:
		return fmt.Errorf("wrong RTSP conn mode: %d", c.mode)
	}

	for i := 0; i < len(c.udpConn); i++ {
		go c.handleUDPData(byte(i))
	}

	for c.state != StateNone {
		ts := time.Now()

		_ = c.conn.SetReadDeadline(ts.Add(timeout))

		if err = c.handleTCPData(); err != nil {
			return
		}
	}

	return
}

func (c *Conn) handleKeepalive(ctx context.Context, d time.Duration) {
	ticker := time.NewTicker(d)
	for {
		select {
		case <-ticker.C:
			req := &tcp.Request{Method: MethodOptions, URL: c.URL}
			if err := c.WriteRequest(req); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Conn) handleUDPData(channel byte) {
	// 待办：处理超时，任意错误后断开 TCP 连接
	conn := c.udpConn[channel]

	for {
		// TP-Link Tapo 摄像头可能使用 10000 字节的数据包
		buf := make([]byte, 10240)

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}

		if err = c.handleRawPacket(channel, buf[:n]); err != nil {
			return
		}
	}
}

func (c *Conn) handleTCPData() error {
	// 可读取以下数据：
	// 1. RTP 交错包：$ + 1 字节通道号 + 2 字节长度
	// 2. RTSP 响应：RTSP/1.0 200 OK
	// 3. RTSP 请求：OPTIONS ...
	var buf4 []byte // `$` + 1B channel number + 2B size
	var err error

	buf4, err = c.reader.Peek(4)
	if err != nil {
		return err
	}

	var channel byte
	var size uint16

	if buf4[0] != '$' {
		switch string(buf4) {
		case "RTSP":
			var res *tcp.Response
			if res, err = c.ReadResponse(); err != nil {
				return err
			}
			c.Fire(res)
			// 仅在 PLAY 返回成功后播放回传音频
			c.playOK = true
			return nil

		case "OPTI", "TEAR", "DESC", "SETU", "PLAY", "PAUS", "RECO", "ANNO", "GET_", "SET_":
			var req *tcp.Request
			if req, err = c.ReadRequest(); err != nil {
				return err
			}
			c.Fire(req)
			if req.Method == MethodOptions {
				res := &tcp.Response{Request: req}
				if err = c.WriteResponse(res); err != nil {
					return err
				}
			}
			return nil

		default:
			c.Fire("RTSP wrong input")

			for i := 0; ; i++ {
				// 查找下一个起始标记
				if _, err = c.reader.ReadBytes('$'); err != nil {
					return err
				}

				if channel, err = c.reader.ReadByte(); err != nil {
					return err
				}

				// 待办：改进最大有效通道标识检查
				if channel >= 20 {
					continue
				}

				buf4 = make([]byte, 2)
				if _, err = io.ReadFull(c.reader, buf4); err != nil {
					return err
				}

				// 检查大小是否适用于 RTP
				size = binary.BigEndian.Uint16(buf4)
				if size <= 1500 {
					break
				}

				// 尝试 10 次查找有效数据包
				if i >= 10 {
					return fmt.Errorf("RTSP wrong input")
				}
			}
		}
	} else {
		// 假设奇数通道均为 RTCP
		channel = buf4[1]

		// 获取数据大小
		size = binary.BigEndian.Uint16(buf4[2:])

		// 跳过 c.reader.Peek 中的 4 个字节
		if _, err = c.reader.Discard(4); err != nil {
			return err
		}
	}

	// 初始化数据内存
	buf := make([]byte, size)
	if _, err = io.ReadFull(c.reader, buf); err != nil {
		return err
	}

	c.Recv += int(size)

	return c.handleRawPacket(channel, buf)
}

func (c *Conn) handleRawPacket(channel byte, buf []byte) error {
	if channel&1 == 0 {
		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buf); err != nil {
			return err
		}

		for _, receiver := range c.Receivers {
			if receiver.ID == channel {
				receiver.WriteRTP(packet)
				break
			}
		}
	} else {
		msg := &RTCP{Channel: channel}

		if err := msg.Header.Unmarshal(buf); err != nil {
			return nil
		}

		//var err error
		//msg.Packets, err = rtcp.Unmarshal(buf)
		//if err != nil {
		//	return nil
		//}

		c.Fire(msg)
	}

	return nil
}

func (c *Conn) WriteRequest(req *tcp.Request) error {
	if req.Proto == "" {
		req.Proto = ProtoRTSP
	}

	if req.Header == nil {
		req.Header = make(map[string][]string)
	}

	c.sequence++
	// CSeq 的大小写必须正确
	// https://github.com/AlexxIT/go2rtc/issues/7
	req.Header["CSeq"] = []string{strconv.Itoa(c.sequence)}

	c.auth.Write(req)

	if c.session != "" {
		req.Header.Set("Session", c.session)
	}

	if req.Body != nil {
		val := strconv.Itoa(len(req.Body))
		req.Header.Set("Content-Length", val)
	}

	c.Fire(req)

	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}

	return req.Write(c.conn)
}

func (c *Conn) ReadRequest() (*tcp.Request, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return nil, err
	}
	return tcp.ReadRequest(c.reader)
}

func (c *Conn) WriteResponse(res *tcp.Response) error {
	if res.Proto == "" {
		res.Proto = ProtoRTSP
	}

	if res.Status == "" {
		res.Status = "200 OK"
	}

	if res.Header == nil {
		res.Header = make(map[string][]string)
	}

	if res.Request != nil && res.Request.Header != nil {
		seq := res.Request.Header.Get("CSeq")
		if seq != "" {
			res.Header.Set("CSeq", seq)
		}
	}

	if c.session != "" {
		if res.Request != nil && res.Request.Method == MethodSetup {
			res.Header.Set("Session", c.session+";timeout=60")
		} else {
			res.Header.Set("Session", c.session)
		}
	}

	if res.Body != nil {
		val := strconv.Itoa(len(res.Body))
		res.Header.Set("Content-Length", val)
	}

	c.Fire(res)

	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}

	return res.Write(c.conn)
}

func (c *Conn) ReadResponse() (*tcp.Response, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return nil, err
	}
	return tcp.ReadResponse(c.reader)
}
