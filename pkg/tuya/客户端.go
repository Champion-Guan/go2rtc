package tuya

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

type Client struct {
	api       TuyaAPI
	conn      *webrtc.Conn
	pc        *pion.PeerConnection
	connected core.Waiter
	closed    bool

	// 仅适用于 HEVC：
	dc         *pion.DataChannel
	videoSSRC  *uint32
	audioSSRC  *uint32
	streamType int
	isHEVC     bool
	handlersMu sync.RWMutex
	handlers   map[uint32]func(*rtp.Packet)
}

type DataChannelMessage struct {
	Type string `json:"type"` // "codec", "start", "recv", "complete"
	Msg  string `json:"msg"`
}

// RecvMessage 包含视频和音频流的 SSRC
type RecvMessage struct {
	Video struct {
		SSRC uint32 `json:"ssrc"`
	} `json:"video"`
	Audio struct {
		SSRC uint32 `json:"ssrc"`
	} `json:"audio"`
}

func Dial(rawURL string) (core.Producer, error) {
	escapedURL := strings.ReplaceAll(rawURL, "#", "%23")
	u, err := url.Parse(escapedURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	// 涂鸦智能接口
	email := query.Get("email")
	password := query.Get("password")

	// 涂鸦云接口
	uid := query.Get("uid")
	clientId := query.Get("client_id")
	clientSecret := query.Get("client_secret")

	// 共享参数
	deviceId := query.Get("device_id")

	// 流参数
	streamResolution := query.Get("resolution")

	useSmartApi := deviceId != "" && email != "" && password != ""
	useCloudApi := deviceId != "" && uid != "" && clientId != "" && clientSecret != ""

	if streamResolution == "" || (streamResolution != "hd" && streamResolution != "sd") {
		streamResolution = "hd"
	}

	if !useSmartApi && !useCloudApi {
		return nil, errors.New("tuya: wrong query params")
	}

	client := &Client{
		handlers: make(map[uint32]func(*rtp.Packet)),
	}

	if useSmartApi {
		if client.api, err = NewTuyaSmartApiClient(nil, u.Hostname(), email, password, deviceId); err != nil {
			return nil, fmt.Errorf("tuya: %w", err)
		}
	} else {
		if client.api, err = NewTuyaCloudApiClient(u.Hostname(), uid, deviceId, clientId, clientSecret); err != nil {
			return nil, fmt.Errorf("tuya: %w", err)
		}
	}

	if err := client.api.Init(); err != nil {
		return nil, fmt.Errorf("tuya: %w", err)
	}

	client.streamType = client.api.GetStreamType(streamResolution)
	client.isHEVC = client.api.IsHEVC(client.streamType)

	// 创建新的 PeerConnection
	conf := pion.Configuration{
		ICEServers:         client.api.GetICEServers(),
		ICETransportPolicy: pion.ICETransportPolicyAll,
		BundlePolicy:       pion.BundlePolicyMaxBundle,
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		client.Close(err)
		return nil, err
	}

	client.pc, err = api.NewPeerConnection(conf)
	if err != nil {
		client.Close(err)
		return nil, err
	}

	// 防止在连接提议前发送 ICE 候选地址
	var sendOffer core.Waiter

	// 防止错误导致阻塞
	defer sendOffer.Done(nil)

	// 创建新的 WebRTC 连接
	client.conn = webrtc.NewConn(client.pc)
	client.conn.FormatName = "tuya/webrtc"
	client.conn.Mode = core.ModeActiveProducer
	client.conn.Protocol = "mqtt"

	mqttClient := client.api.GetMqtt()
	if mqttClient == nil {
		err = errors.New("tuya: no mqtt client")
		client.Close(err)
		return nil, err
	}

	// 设置 MQTT 处理器
	mqttClient.handleAnswer = func(answer AnswerFrame) {
		// fmt.Printf("tuya: answer: %s\n", answer.Sdp)

		desc := pion.SessionDescription{
			Type: pion.SDPTypePranswer,
			SDP:  answer.Sdp,
		}

		if err = client.pc.SetRemoteDescription(desc); err != nil {
			client.Close(err)
			return
		}

		if err = client.conn.SetAnswer(answer.Sdp); err != nil {
			client.Close(err)
			return
		}

		if client.isHEVC {
			// 即使 HEVC 流，涂鸦也可能返回 H264/90000
			// 因此需用接口返回的 HEVC 编码替换视频编码
			for _, media := range client.conn.Medias {
				if media.Kind == core.KindVideo {
					codecs := client.api.GetVideoCodecs()
					if codecs != nil {
						media.Codecs = codecs
					}
				}
			}

			// 音频编码也使用接口返回值
			// 涂鸦返回多个音频编码，如 PCMU、PCMA，
			// Skill 仅有 PCMA 时，使用 PCMU 会导致质量较差
			for _, media := range client.conn.Medias {
				if media.Kind == core.KindAudio {
					codecs := client.api.GetAudioCodecs()
					if codecs != nil {
						media.Codecs = codecs
					}
				}
			}
		}
	}

	mqttClient.handleCandidate = func(candidate CandidateFrame) {
		// fmt.Printf("tuya: candidate: %s\n", candidate.Candidate)

		if candidate.Candidate != "" {
			client.conn.AddCandidate(candidate.Candidate)
			if err != nil {
				client.Close(err)
			}
		}
	}

	mqttClient.handleDisconnect = func() {
		// fmt.Println("tuya: disconnect")
		client.Close(errors.New("mqtt: disconnect"))
	}

	mqttClient.handleError = func(err error) {
		// fmt.Printf("tuya: error: %s\n", err.Error())
		client.Close(err)
	}

	if client.isHEVC {
		maxRetransmits := uint16(5)
		ordered := true
		client.dc, err = client.pc.CreateDataChannel("fmp4Stream", &pion.DataChannelInit{
			MaxRetransmits: &maxRetransmits,
			Ordered:        &ordered,
		})

		// 数据通道接收两种消息：
		// 1. 字符串消息：控制消息（codec、recv）
		// 2. 二进制消息：包含视频/音频的 RTP 数据包
		client.dc.OnMessage(func(msg pion.DataChannelMessage) {
			if msg.IsString {
				// 处理控制消息，如 codec、recv
				if connected, err := client.probe(msg); err != nil {
					client.Close(err)
				} else if connected {
					client.connected.Done(nil)
				}
			} else {
				// 按 recv 消息的 SSRC 路由 RTP 数据包
				packet := &rtp.Packet{}
				if err := packet.Unmarshal(msg.Data); err != nil {
					// 跳过无效数据包
					return
				}

				if handler, ok := client.getHandler(packet.SSRC); ok {
					handler(packet)
				}
			}
		})

		client.dc.OnError(func(err error) {
			// fmt.Printf("tuya: datachannel error: %s\n", err.Error())
			client.Close(err)
		})

		client.dc.OnClose(func() {
			// fmt.Println("tuya: datachannel closed")
			client.Close(errors.New("datachannel: closed"))
		})

		client.dc.OnOpen(func() {
			// fmt.Println("tuya: datachannel opened")

			codecRequest, _ := json.Marshal(DataChannelMessage{
				Type: "codec",
				Msg:  "",
			})

			if err := client.sendMessageToDataChannel(codecRequest); err != nil {
				client.Close(fmt.Errorf("failed to send codec request: %w", err))
			}
		})
	}

	// 设置 PeerConnection 处理器
	client.conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOffer.Wait()
			if err := mqttClient.SendCandidate("a=" + msg.ToJSON().Candidate); err != nil {
				client.Close(err)
			}

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateNew:
				break
			case pion.PeerConnectionStateConnecting:
				break
			case pion.PeerConnectionStateConnected:
				// HEVC 需要等待数据通道打开，并收到摄像头编码信息
				if !client.isHEVC {
					if streamResolution == "hd" {
						_ = mqttClient.SendResolution(0)
					}
					client.connected.Done(nil)
				}
			case pion.PeerConnectionStateClosed:
				client.Close(errors.New("webrtc: " + msg.String()))
			default:
				// client.Close(errors.New("webrtc: " + msg.String()))
			}
		}
	})

	// 先添加音频，否则涂鸦可能返回损坏的 SDP
	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// 创建连接提议
	offer, err := client.conn.CreateOffer(medias)
	if err != nil {
		client.Close(err)
		return nil, err
	}

	// 移除 a=extmap 等行以缩短 SDP，设备仅允许 8 KB JSON 负载
	// https://github.com/tuya/webrtc-demo-go/blob/04575054f18ccccb6bc9d82939dd46d449544e20/static/js/main.js#L224
	re := regexp.MustCompile(`\r\na=extmap[^\r\n]*`)
	offer = re.ReplaceAllString(offer, "")

	// 发送连接提议
	if err := mqttClient.SendOffer(offer, streamResolution, client.streamType, client.isHEVC); err != nil {
		err = fmt.Errorf("tuya: %w", err)
		client.Close(err)
		return nil, err
	}

	sendOffer.Done(nil)

	// 等待连接
	if err = client.connected.Wait(); err != nil {
		err = fmt.Errorf("tuya: %w", err)
		client.Close(err)
		return nil, err
	}

	return client, nil
}

func (c *Client) GetMedias() []*core.Media {
	return c.conn.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	localTrack := c.conn.GetSenderTrack(media.ID)
	if localTrack == nil {
		return errors.New("webrtc: can't get track")
	}

	// 已禁用：扬声器协议 312 命令
	// JavaScript 客户端首次调用也不发送此命令
	// 后续调用设置 speakerChloron 后才发送协议 312
	// mqttClient := c.api.GetMqtt()
	// if mqttClient != nil {
	// 	_ = mqttClient.SendSpeaker(1)
	// }

	payloadType := codec.PayloadType

	sender := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML:
		// 帧大小会影响涂鸦摄像头音频延迟：
		// 浏览器发送标准 20 ms 帧，即 G.711 的 160 字节，
		// 但部分摄像头延迟可达 4 秒；增至 240 字节，即 30 ms，
		// 可降至约 2 秒。320 字节以上无法正常工作。
		// 使用 240 字节，在延迟和稳定性之间取得平衡。
		frameSize := 240

		var buf []byte
		var seq uint16
		var ts uint32

		sender.Handler = func(packet *rtp.Packet) {
			buf = append(buf, packet.Payload...)

			for len(buf) >= frameSize {
				payload := buf[:frameSize]

				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    payloadType,
						SequenceNumber: seq,
						Timestamp:      ts,
						SSRC:           packet.SSRC,
					},
					Payload: payload,
				}

				seq++
				ts += uint32(frameSize)
				buf = buf[frameSize:]

				c.conn.Send += pkt.MarshalSize()
				_ = localTrack.WriteRTP(payloadType, pkt)
			}
		}

	default:
		sender.Handler = func(packet *rtp.Packet) {
			c.conn.Send += packet.MarshalSize()
			_ = localTrack.WriteRTP(payloadType, packet)
		}
	}

	sender.HandleRTP(track)
	c.conn.Senders = append(c.conn.Senders, sender)

	return nil
}

func (c *Client) Start() error {
	if len(c.conn.Receivers) == 0 {
		return errors.New("tuya: no receivers")
	}

	var video, audio *core.Receiver
	for _, receiver := range c.conn.Receivers {
		if receiver.Codec.IsVideo() {
			video = receiver
		} else if receiver.Codec.IsAudio() {
			audio = receiver
		}
	}

	if c.videoSSRC != nil {
		c.setHandler(*c.videoSSRC, func(packet *rtp.Packet) {
			if video != nil {
				video.WriteRTP(packet)
			}
		})
	}

	if c.audioSSRC != nil {
		c.setHandler(*c.audioSSRC, func(packet *rtp.Packet) {
			if audio != nil {
				audio.WriteRTP(packet)
			}
		})
	}

	return c.conn.Start()
}

func (c *Client) Stop() error {
	if c.closed {
		return nil
	}

	c.closed = true

	c.clearHandlers()

	if c.conn != nil {
		_ = c.conn.Stop()
	}

	if c.api != nil {
		c.api.Close()
	}

	return nil
}

func (c *Client) Close(err error) error {
	c.connected.Done(err)
	return c.Stop()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}

func (c *Client) setHandler(ssrc uint32, handler func(*rtp.Packet)) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.handlers[ssrc] = handler
}

func (c *Client) getHandler(ssrc uint32) (func(*rtp.Packet), bool) {
	c.handlersMu.RLock()
	defer c.handlersMu.RUnlock()
	handler, ok := c.handlers[ssrc]
	return handler, ok
}

func (c *Client) clearHandlers() {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	for ssrc := range c.handlers {
		delete(c.handlers, ssrc)
	}
}

func (c *Client) probe(msg pion.DataChannelMessage) (bool, error) {
	// fmt.Printf("[tuya] Received string message: %s\n", string(msg.Data))

	var message DataChannelMessage
	if err := json.Unmarshal([]byte(msg.Data), &message); err != nil {
		return false, err
	}

	switch message.Type {
	case "codec":
		// 摄像头已响应编码请求，现在请求开始发送帧
		frameRequest, _ := json.Marshal(DataChannelMessage{
			Type: "start",
			Msg:  "frame",
		})

		err := c.sendMessageToDataChannel(frameRequest)
		if err != nil {
			return false, err
		}

	case "recv":
		// 摄像头发送视频和音频 SSRC，
		// 用于正确路由收到的 RTP 包
		var recvMessage RecvMessage
		if err := json.Unmarshal([]byte(message.Msg), &recvMessage); err != nil {
			return false, err
		}

		videoSSRC := recvMessage.Video.SSRC
		audioSSRC := recvMessage.Audio.SSRC
		c.videoSSRC = &videoSSRC
		c.audioSSRC = &audioSSRC

		// 发送 complete，告知摄像头已准备接收 RTP
		completeMsg, _ := json.Marshal(DataChannelMessage{
			Type: "complete",
			Msg:  "",
		})

		err := c.sendMessageToDataChannel(completeMsg)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (c *Client) sendMessageToDataChannel(message []byte) error {
	if c.dc != nil {
		// fmt.Printf("[tuya] sending message to data channel: %s\n", message)
		return c.dc.Send(message)
	}

	return nil
}
