package ring

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/google/uuid"
	pion "github.com/pion/webrtc/v4"
)

type Client struct {
	api       *RingApi
	wsClient  *WSClient
	prod      core.Producer
	cameraID  int
	dialogID  string
	connected core.Waiter
	closed    bool
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	encodedToken := query.Get("refresh_token")
	cameraID := query.Get("camera_id")
	deviceID := query.Get("device_id")
	_, isSnapshot := query["snapshot"]

	if encodedToken == "" || deviceID == "" || cameraID == "" {
		return nil, errors.New("ring: wrong query")
	}

	client := &Client{
		dialogID: uuid.NewString(),
	}

	client.cameraID, err = strconv.Atoi(cameraID)
	if err != nil {
		return nil, fmt.Errorf("ring: invalid camera_id: %w", err)
	}

	refreshToken, err := url.QueryUnescape(encodedToken)
	if err != nil {
		return nil, fmt.Errorf("ring: invalid refresh token encoding: %w", err)
	}

	client.api, err = NewRestClient(RefreshTokenAuth{RefreshToken: refreshToken}, nil)
	if err != nil {
		return nil, err
	}

	// 快照流程
	if isSnapshot {
		client.prod = NewSnapshotProducer(client.api, client.cameraID)
		return client, nil
	}

	client.wsClient, err = StartWebsocket(client.cameraID, client.api)
	if err != nil {
		client.Stop()
		return nil, err
	}

	// 创建 PeerConnection
	conf := pion.Configuration{
		ICEServers: []pion.ICEServer{
			{URLs: []string{
				"stun:stun.kinesisvideo.us-east-1.amazonaws.com:443",
				"stun:stun.kinesisvideo.us-east-2.amazonaws.com:443",
				"stun:stun.kinesisvideo.us-west-2.amazonaws.com:443",
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
				"stun:stun2.l.google.com:19302",
				"stun:stun3.l.google.com:19302",
				"stun:stun4.l.google.com:19302",
			}},
		},
		ICETransportPolicy: pion.ICETransportPolicyAll,
		BundlePolicy:       pion.BundlePolicyBalanced,
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		client.Stop()
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		client.Stop()
		return nil, err
	}

	// 防止在连接提议前发送 ICE 候选地址
	var sendOffer core.Waiter

	// 防止错误导致阻塞
	defer sendOffer.Done(nil)

	prod := webrtc.NewConn(pc)
	prod.FormatName = "ring/webrtc"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL

	client.wsClient.onMessage = func(msg WSMessage) {
		client.onWSMessage(msg)
	}

	client.wsClient.onError = func(err error) {
		// fmt.Printf("ring: error: %s\n", err.Error())
		client.Stop()
		client.connected.Done(err)
	}

	client.wsClient.onClose = func() {
		// fmt.Println("ring: disconnect")
		client.Stop()
		client.connected.Done(errors.New("ring: disconnect"))
	}

	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOffer.Wait()

			iceCandidate := msg.ToJSON()

			// 跳过空 ICE 候选地址
			if iceCandidate.Candidate == "" {
				return
			}

			icePayload := map[string]interface{}{
				"ice":        iceCandidate.Candidate,
				"mlineindex": iceCandidate.SDPMLineIndex,
			}

			if err = client.wsClient.sendSessionMessage("ice", icePayload); err != nil {
				client.connected.Done(err)
				return
			}

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateNew:
				break
			case pion.PeerConnectionStateConnecting:
				break
			case pion.PeerConnectionStateConnected:
				client.connected.Done(nil)
			default:
				client.Stop()
				client.connected.Done(errors.New("ring: " + msg.String()))
			}
		}
	})

	client.prod = prod

	// 设置媒体配置
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendRecv,
			Codecs: []*core.Codec{
				{
					Name:      "opus",
					ClockRate: 48000,
					Channels:  2,
				},
			},
		},
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:      "H264",
					ClockRate: 90000,
				},
			},
		},
	}

	// 创建连接提议
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		client.Stop()
		return nil, err
	}

	// 发送连接提议
	offerPayload := map[string]interface{}{
		"stream_options": map[string]bool{
			"audio_enabled": true,
			"video_enabled": true,
		},
		"sdp": offer,
	}

	if err = client.wsClient.sendSessionMessage("live_view", offerPayload); err != nil {
		client.Stop()
		return nil, err
	}

	sendOffer.Done(nil)

	if err = client.connected.Wait(); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *Client) onWSMessage(msg WSMessage) {
	rawMsg, _ := json.Marshal(msg)

	// fmt.Printf("ring: onWSMessage: %s\n", string(rawMsg))

	// 检查是否存在 doorbot_id
	if _, ok := msg.Body["doorbot_id"]; !ok {
		return
	}

	// 检查消息是否来自正确门铃
	doorbotID := msg.Body["doorbot_id"].(float64)
	if int(doorbotID) != c.cameraID {
		return
	}

	if msg.Method == "session_created" || msg.Method == "session_started" {
		if _, ok := msg.Body["session_id"]; ok && c.wsClient.sessionID == "" {
			c.wsClient.sessionID = msg.Body["session_id"].(string)
		}
	}

	// 检查消息是否来自正确会话
	if _, ok := msg.Body["session_id"]; ok {
		if msg.Body["session_id"].(string) != c.wsClient.sessionID {
			return
		}
	}

	switch msg.Method {
	case "sdp":
		if prod, ok := c.prod.(*webrtc.Conn); ok {
			// 获取应答
			var msg AnswerMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				c.Stop()
				c.connected.Done(err)
				return
			}

			if err := prod.SetAnswer(msg.Body.SDP); err != nil {
				c.Stop()
				c.connected.Done(err)
				return
			}

			if err := c.wsClient.activateSession(); err != nil {
				c.Stop()
				c.connected.Done(err)
				return
			}

			prod.SDP = msg.Body.SDP
		}

	case "ice":
		if prod, ok := c.prod.(*webrtc.Conn); ok {
			var msg IceCandidateMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				break
			}

			// 跳过空候选地址
			if msg.Body.Ice == "" {
				break
			}

			if err := prod.AddCandidate(msg.Body.Ice); err != nil {
				c.Stop()
				c.connected.Done(err)
				return
			}
		}

	case "close":
		c.Stop()
		c.connected.Done(errors.New("ring: close"))

	case "pong":
		// Ignore
	}
}

func (c *Client) GetMedias() []*core.Media {
	return c.prod.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.prod.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if webrtcProd, ok := c.prod.(*webrtc.Conn); ok {
		if media.Kind == core.KindAudio {
			// 启用扬声器
			speakerPayload := map[string]interface{}{
				"stealth_mode": false,
			}
			_ = c.wsClient.sendSessionMessage("camera_options", speakerPayload)
		}
		return webrtcProd.AddTrack(media, codec, track)
	}

	return fmt.Errorf("add track not supported for snapshot")
}

func (c *Client) Start() error {
	return c.prod.Start()
}

func (c *Client) Stop() error {
	if c.closed {
		return nil
	}

	c.closed = true

	if c.prod != nil {
		_ = c.prod.Stop()
	}

	if c.wsClient != nil {
		_ = c.wsClient.Close()
	}

	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.prod)
}
