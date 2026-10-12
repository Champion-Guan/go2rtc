package rtmp

import (
	"github.com/AlexxIT/go2rtc/pkg/flv"
)

func (c *Conn) Producer() (*flv.Producer, error) {
	c.rdBuf = []byte{
		'F', 'L', 'V', // signature
		1,          // version
		0,          // flags (has video/audio)
		0, 0, 0, 9, // header size
	}

	prod, err := flv.Open(c)
	if err != nil {
		return nil, err
	}

	prod.FormatName = "rtmp"
	prod.Protocol = "rtmp"
	prod.RemoteAddr = c.conn.RemoteAddr().String()
	prod.URL = c.url

	return prod, nil
}

// Read 将 RTMP 转换为 FLV
func (c *Conn) Read(p []byte) (n int, err error) {
	// 1. 检查临时缓冲区
	if len(c.rdBuf) == 0 {
		msgType, timeMS, payload, err2 := c.readMessage()
		if err2 != nil {
			return 0, err2
		}

		// 上一标签大小 4 字节 + 头部 11 字节 + 负载
		n = 4 + 11 + len(payload)

		// 2. 检查消息是否能放入缓冲区
		if n <= len(p) {
			encodeFLV(p, msgType, timeMS, payload)
			return
		}

		// 3. 将消息放入临时缓冲区
		c.rdBuf = make([]byte, n)
		encodeFLV(c.rdBuf, msgType, timeMS, payload)
	}

	// 4. 发送临时缓冲区
	n = copy(p, c.rdBuf)
	c.rdBuf = c.rdBuf[n:]
	return
}

func encodeFLV(b []byte, msgType byte, time uint32, payload []byte) {
	_ = b[4+11]

	b[0] = 0
	b[1] = 0
	b[2] = 0
	b[3] = 0
	b[4+0] = msgType
	PutUint24(b[4+1:], uint32(len(payload)))
	PutUint24(b[4+4:], time)
	b[4+7] = byte(time >> 24)

	copy(b[4+11:], payload)
}

// Write 将 FLV 转换为 RTMP
func (c *Conn) Write(p []byte) (n int, err error) {
	n = len(p)

	if p[0] == 'F' {
		p = p[9+4:] // skip first msg with FLV header

		for len(p) > 0 {
			size := 11 + uint16(p[2])<<8 + uint16(p[3]) + 4
			if _, err = c.Write(p[:size]); err != nil {
				return 0, err
			}
			p = p[size:]
		}
		return
	}

	// 解码 FLV：11 字节头部 + 负载 + 4 字节大小
	tagType := p[0]
	timeMS := uint32(p[4])<<16 | uint32(p[5])<<8 | uint32(p[6]) | uint32(p[7])<<24
	payload := p[11 : len(p)-4]

	err = c.writeMessage(4, tagType, timeMS, payload)
	return
}
