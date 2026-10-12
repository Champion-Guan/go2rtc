package webrtc

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
)

// streamsHandler 支持：
//  1. WHEP:    webrtc:http://192.168.1.123:1984/api/webrtc?src=camera1
//  2. go2rtc:  webrtc:ws://192.168.1.123:1984/api/ws?src=camera1
//  3. Wyze:    webrtc:http://192.168.1.123:5000/signaling/camera1?kvs#format=wyze
//  4. Kinesis: webrtc:wss://...amazonaws.com/?...#format=kinesis#client_id=...#ice_servers=[{...},{...}]
func streamsHandler(rawURL string) (core.Producer, error) {
	var query url.Values
	if i := strings.IndexByte(rawURL, '#'); i > 0 {
		query = streams.ParseQuery(rawURL[i+1:])
		rawURL = rawURL[:i]
	}

	rawURL = rawURL[7:] // remove webrtc:
	if i := strings.IndexByte(rawURL, ':'); i > 0 {
		scheme := rawURL[:i]
		format := query.Get("format")

		switch scheme {
		case "ws", "wss":
			if format == "kinesis" {
				// https://aws.amazon.com/kinesis/video-streams/
				// https://docs.aws.amazon.com/kinesisvideostreams-webrtc-dg/latest/devguide/what-is-kvswebrtc.html
				// https://github.com/orgs/awslabs/repositories?q=kinesis+webrtc
				return kinesisClient(rawURL, query, "webrtc/kinesis", nil)
			} else if format == "openipc" {
				return openIPCClient(rawURL, query)
			} else if format == "switchbot" {
				return switchbotClient(rawURL, query)
			} else {
				return go2rtcClient(rawURL)
			}

		case "http", "https":
			if format == "milestone" {
				return milestoneClient(rawURL, query)
			} else if format == "wyze" {
				// https://github.com/mrlt8/docker-wyze-bridge
				return wyzeClient(rawURL)
			} else if format == "creality" {
				return crealityClient(rawURL)
			} else {
				return whepClient(rawURL)
			}
		}
	}
	return nil, errors.New("unsupported url: " + rawURL)
}

// go2rtcClient 仅能连接 go2rtc 服务器
// 例如：ws://localhost:1984/api/ws?src=camera1
func go2rtcClient(url string) (core.Producer, error) {
	// 1. 连接信令服务器
	conn, _, err := Dial(url)
	if err != nil {
		return nil, err
	}

	// 准备返回生产者或连接错误时关闭 WebSocket
	defer conn.Close()

	// 2. 创建对等连接
	pc, err := PeerConnection(true)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			_ = pc.Close()
		}
	}()

	// 等待 PeerConnection 错误、WebSocket 错误，或表示连接成功的空值
	var connState core.Waiter
	var connMu sync.Mutex

	prod := webrtc.NewConn(pc)
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = url
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local ")
			connMu.Lock()
			_ = conn.WriteJSON(&ws.Message{Type: "webrtc/candidate", Value: s})
			connMu.Unlock()

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateConnecting:
			case pion.PeerConnectionStateConnected:
				connState.Done(nil)
			default:
				connState.Done(errors.New("webrtc: " + msg.String()))
			}
		}
	})

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionSendonly},
	}

	// 3. 创建提议
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		return nil, err
	}

	// 4. 发送提议
	msg := &ws.Message{Type: "webrtc/offer", Value: offer}
	connMu.Lock()
	_ = conn.WriteJSON(msg)
	connMu.Unlock()

	// 5. 获取应答
	if err = conn.ReadJSON(msg); err != nil {
		return nil, err
	}

	if msg.Type != "webrtc/answer" {
		err = errors.New("wrong answer: " + msg.String())
		return nil, err
	}

	answer := msg.String()
	if err = prod.SetAnswer(answer); err != nil {
		return nil, err
	}

	// 6. 继续接收候选地址
	go func() {
		var err error

		for {
			// 接收远端数据
			var msg ws.Message
			if err = conn.ReadJSON(&msg); err != nil {
				break
			}

			switch msg.Type {
			case "webrtc/candidate":
				if msg.Value != nil {
					_ = prod.AddCandidate(msg.String())
				}
			}
		}

		connState.Done(err)
	}()

	if err = connState.Wait(); err != nil {
		return nil, err
	}

	return prod, nil
}

// whepClient 支持 WebRTC HTTP 输出协议（WHEP）
// 例如：http://localhost:1984/api/webrtc?src=camera1
func whepClient(url string) (core.Producer, error) {
	// 2. 创建对等连接
	pc, err := PeerConnection(true)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "http"
	prod.URL = url

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
	}

	// 3. 创建提议
	offer, err := prod.CreateCompleteOffer(medias)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(offer))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", MimeSDP)

	client := http.Client{Timeout: time.Second * 5000}
	defer client.CloseIdleConnections()

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	answer, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if err = prod.SetAnswer(string(answer)); err != nil {
		return nil, err
	}

	return prod, nil
}

// Dial 是支持基本身份验证的 websocket.Dial
func Dial(rawURL string) (*websocket.Conn, *http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}

	if u.User == nil {
		return websocket.DefaultDialer.Dial(rawURL, nil)
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	u.User = nil

	header := http.Header{
		"Authorization": []string{
			"Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)),
		},
	}

	return websocket.DefaultDialer.Dial(u.String(), header)
}
