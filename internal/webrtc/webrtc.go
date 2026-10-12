package webrtc

import (
	"errors"
	"net"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen     string           `yaml:"listen"`
			Candidates []string         `yaml:"candidates"`
			IceServers []pion.ICEServer `yaml:"ice_servers"`
			Filters    webrtc.Filters   `yaml:"filters"`
		} `yaml:"webrtc"`
	}

	cfg.Mod.Listen = ":8555"
	cfg.Mod.IceServers = []pion.ICEServer{
		{URLs: []string{"stun:stun.cloudflare.com:3478", "stun:stun.l.google.com:19302"}},
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("webrtc")

	if log.Debug().Enabled() {
		itfs, _ := net.Interfaces()
		for _, itf := range itfs {
			addrs, _ := itf.Addrs()
			log.Debug().Msgf("[webrtc] interface %+v addrs %v", itf, addrs)
		}
	}

	address, network, _ := strings.Cut(cfg.Mod.Listen, "/")
	for _, candidate := range cfg.Mod.Candidates {
		AddCandidate(network, candidate)

		if strings.HasPrefix(candidate, "stun:") && stuns == nil {
			for _, ice := range cfg.Mod.IceServers {
				for _, url := range ice.URLs {
					if strings.HasPrefix(url, "stun:") {
						stuns = append(stuns, url[5:])
					}
				}
			}
		}
	}

	webrtc.OnNewListener = func(ln any) {
		switch ln := ln.(type) {
		case *net.TCPListener:
			log.Info().Stringer("addr", ln.Addr()).Msg("[webrtc] listen tcp")
		case *net.UDPConn:
			log.Info().Stringer("addr", ln.LocalAddr()).Msg("[webrtc] listen udp")
		}
	}

	var err error

	// 使用自定义编码列表和网络设置创建 pionAPI
	serverAPI, err = webrtc.NewServerAPI(network, address, &cfg.Mod.Filters)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	// 未指定地址时，WebRTC 服务器和客户端使用同一接口
	clientAPI = serverAPI

	if address != "" {
		clientAPI, _ = webrtc.NewAPI()
	}

	pionConf := pion.Configuration{
		ICEServers:   cfg.Mod.IceServers,
		SDPSemantics: pion.SDPSemanticsUnifiedPlanWithFallback,
	}

	PeerConnection = func(active bool) (*pion.PeerConnection, error) {
		// active 为客户端，passive 为服务器
		if active {
			return clientAPI.NewPeerConnection(pionConf)
		} else {
			return serverAPI.NewPeerConnection(pionConf)
		}
	}

	// 异步 WebRTC 服务器，支持两个接口版本
	ws.HandleFunc("webrtc", asyncHandler)
	ws.HandleFunc("webrtc/offer", asyncHandler)
	ws.HandleFunc("webrtc/candidate", candidateHandler)

	// 同步 WebRTC 服务器，支持两个接口版本
	api.HandleFunc("api/webrtc", syncHandler)

	// WebRTC 客户端
	streams.HandleFunc("webrtc", streamsHandler)
}

var serverAPI, clientAPI *pion.API

var log zerolog.Logger

var PeerConnection func(active bool) (*pion.PeerConnection, error)

func asyncHandler(tr *ws.Transport, msg *ws.Message) (err error) {
	var stream *streams.Stream
	var mode core.Mode

	query := tr.Request.URL.Query()
	if name := query.Get("src"); name != "" {
		stream, _ = streams.GetOrPatch(query)
		mode = core.ModePassiveConsumer
		log.Debug().Str("src", name).Msg("[webrtc] new consumer")
	} else if name = query.Get("dst"); name != "" {
		stream = streams.Get(name)
		mode = core.ModePassiveProducer
		log.Debug().Str("src", name).Msg("[webrtc] new producer")
	}

	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	var offer struct {
		Type       string           `json:"type"`
		SDP        string           `json:"sdp"`
		ICEServers []pion.ICEServer `json:"ice_servers"`
	}

	// V2 交换 JSON 对象，V1 交换原始 SDP
	apiV2 := msg.Type == "webrtc"

	if apiV2 {
		if err = msg.Unmarshal(&offer); err != nil {
			return err
		}
	} else {
		offer.SDP = msg.String()
	}

	// 创建新的 PeerConnection 实例
	var pc *pion.PeerConnection
	if offer.ICEServers == nil {
		pc, err = PeerConnection(false)
	} else {
		pc, err = serverAPI.NewPeerConnection(pion.Configuration{ICEServers: offer.ICEServers})
	}
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	var sendAnswer core.Waiter

	// 防止错误导致阻塞
	defer sendAnswer.Done(nil)

	conn := webrtc.NewConn(pc)
	conn.Mode = mode
	conn.Protocol = "ws"
	conn.UserAgent = tr.Request.UserAgent()
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg != pion.PeerConnectionStateClosed {
				return
			}
			switch mode {
			case core.ModePassiveConsumer:
				stream.RemoveConsumer(conn)
			case core.ModePassiveProducer:
				stream.RemoveProducer(conn)
			}

		case *pion.ICECandidate:
			if !FilterCandidate(msg) {
				return
			}
			_ = sendAnswer.Wait()

			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local ")
			tr.Write(&ws.Message{Type: "webrtc/candidate", Value: s})
		}
	})

	log.Trace().Msgf("[webrtc] offer:\n%s", offer.SDP)

	// 1. 设置提议，获取远端客户端编码
	if err = conn.SetOffer(offer.SDP); err != nil {
		log.Warn().Err(err).Caller().Send()
		return err
	}

	switch mode {
	case core.ModePassiveConsumer:
		// 2. 添加消费者，获取新轨道
		if err = stream.AddConsumer(conn); err != nil {
			log.Debug().Err(err).Msg("[webrtc] add consumer")
			_ = conn.Close()
			return err
		}
	case core.ModePassiveProducer:
		stream.AddProducer(conn)
	}

	// 3. 交换 SDP，无需等待全部候选地址
	answer, err := conn.GetAnswer()
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	if apiV2 {
		desc := pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}
		tr.Write(&ws.Message{Type: "webrtc", Value: desc})
	} else {
		tr.Write(&ws.Message{Type: "webrtc/answer", Value: answer})
	}

	sendAnswer.Done(nil)

	asyncCandidates(tr, conn)

	return nil
}

func ExchangeSDP(stream *streams.Stream, offer, desc, userAgent string) (answer string, err error) {
	pc, err := PeerConnection(false)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	// 创建新的 WebRTC 实例
	conn := webrtc.NewConn(pc)
	conn.FormatName = desc
	conn.UserAgent = userAgent
	conn.Protocol = "http"
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg != pion.PeerConnectionStateClosed {
				return
			}
			if conn.Mode == core.ModePassiveConsumer {
				stream.RemoveConsumer(conn)
			} else {
				stream.RemoveProducer(conn)
			}
		}
	})

	// 1. 设置提议，获取远端客户端编码
	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Caller().Send()
		return
	}

	if IsConsumer(conn) {
		conn.Mode = core.ModePassiveConsumer

		// 2. 添加消费者，获取新轨道
		if err = stream.AddConsumer(conn); err != nil {
			log.Warn().Err(err).Caller().Send()
			_ = conn.Close()
			return
		}
	} else {
		conn.Mode = core.ModePassiveProducer

		stream.AddProducer(conn)
	}

	answer, err = conn.GetCompleteAnswer(GetCandidates(), FilterCandidate)
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
	}

	return
}

func IsConsumer(conn *webrtc.Conn) bool {
	// 接收视频时作为消费者
	for _, media := range conn.GetMedias() {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionSendonly {
			return true
		}
	}
	// 发送视频时作为生产者
	for _, media := range conn.GetMedias() {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionRecvonly {
			return false
		}
	}
	// 请求媒体时作为消费者
	for _, media := range conn.GetMedias() {
		if media.Direction == core.DirectionSendonly {
			return true
		}
	}
	return false
}
