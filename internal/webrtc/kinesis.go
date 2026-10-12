package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
)

type kinesisRequest struct {
	Action   string `json:"action"`
	ClientID string `json:"recipientClientId"`
	Payload  []byte `json:"messagePayload"`
}

func (k kinesisRequest) String() string {
	return fmt.Sprintf("action=%s, payload=%s", k.Action, k.Payload)
}

type kinesisResponse struct {
	Payload []byte `json:"messagePayload"`
	Type    string `json:"messageType"`
}

func (k kinesisResponse) String() string {
	return fmt.Sprintf("type=%s, payload=%s", k.Type, k.Payload)
}

func kinesisClient(
	rawURL string, query url.Values, format string,
	sdpOffer func(prod *webrtc.Conn, query url.Values) (any, error),
) (core.Producer, error) {
	// 1. 连接信令服务器
	conn, _, err := websocket.DefaultDialer.Dial(rawURL, nil)
	if err != nil {
		return nil, err
	}

	// 2. 从查询参数加载 ICEServers（Base64 编码的 JSON）
	conf := pion.Configuration{}

	if s := query.Get("ice_servers"); s != "" {
		conf.ICEServers, err = webrtc.UnmarshalICEServers([]byte(s))
		if err != nil {
			log.Warn().Err(err).Caller().Send()
		}
	}

	// 准备返回生产者或连接错误时关闭 WebSocket
	defer conn.Close()

	// 3. 创建对等连接
	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		return nil, err
	}

	// 防止在连接提议前发送 ICE 候选地址
	var sendOffer core.Waiter

	// 防止错误导致阻塞
	defer sendOffer.Done(nil)

	// 等待 PeerConnection 错误、WebSocket 错误，或表示连接成功的空值
	var connState core.Waiter

	req := kinesisRequest{
		ClientID: query.Get("client_id"),
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = format
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOffer.Wait()

			req.Action = "ICE_CANDIDATE"
			req.Payload, _ = json.Marshal(msg.ToJSON())
			if err = conn.WriteJSON(&req); err != nil {
				connState.Done(err)
				return
			}

			log.Trace().Msgf("[webrtc] kinesis send: %s", req)

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

	var payload any

	if sdpOffer == nil {
		medias := []*core.Media{
			{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
			{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		}

		// 4. 创建提议
		var offer string
		if offer, err = prod.CreateOffer(medias); err != nil {
			return nil, err
		}

		// 5. 发送提议
		payload = pion.SessionDescription{
			Type: pion.SDPTypeOffer,
			SDP:  offer,
		}
	} else {
		if payload, err = sdpOffer(prod, query); err != nil {
			return nil, err
		}
	}

	req.Action = "SDP_OFFER"
	req.Payload, _ = json.Marshal(payload)
	if err = conn.WriteJSON(req); err != nil {
		return nil, err
	}

	log.Trace().Msgf("[webrtc] kinesis send: %s", req)

	sendOffer.Done(nil)

	go func() {
		var err error

		// 底层连接关闭时同步关闭
		for {
			var res kinesisResponse
			if err = conn.ReadJSON(&res); err != nil {
				// 处理 Amazon 服务器中的异常消息
				if errors.Is(err, io.ErrUnexpectedEOF) {
					continue
				}
				break
			}

			log.Trace().Msgf("[webrtc] kinesis recv: %s", res)

			switch res.Type {
			case "SDP_ANSWER":
				// 6. 获取应答
				var sd pion.SessionDescription
				if err = json.Unmarshal(res.Payload, &sd); err != nil {
					break
				}

				if err = prod.SetAnswer(sd.SDP); err != nil {
					break
				}

			case "ICE_CANDIDATE":
				// 7. 继续接收候选地址
				var ci pion.ICECandidateInit
				if err = json.Unmarshal(res.Payload, &ci); err != nil {
					break
				}

				if err = prod.AddCandidate(ci.Candidate); err != nil {
					break
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

type wyzeKVS struct {
	ClientId string          `json:"ClientId"`
	Cam      string          `json:"cam"`
	Result   string          `json:"result"`
	Servers  json.RawMessage `json:"servers"`
	URL      string          `json:"signalingUrl"`
}

func wyzeClient(rawURL string) (core.Producer, error) {
	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var kvs wyzeKVS
	if err = json.Unmarshal(b, &kvs); err != nil {
		return nil, err
	}

	if kvs.Result != "ok" {
		return nil, errors.New("wyse: wrong result: " + kvs.Result)
	}

	query := url.Values{
		"client_id":   []string{kvs.ClientId},
		"ice_servers": []string{string(kvs.Servers)},
	}

	return kinesisClient(kvs.URL, query, "webrtc/wyze", nil)
}
