package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

func (c *Conn) CreateOffer(medias []*core.Media) (string, error) {
	// 1. 创建类型和方向正确的收发器
	for _, media := range medias {
		var err error
		switch media.Direction {
		case core.DirectionRecvonly:
			_, err = c.pc.AddTransceiverFromKind(
				webrtc.NewRTPCodecType(media.Kind),
				webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
			)
		case core.DirectionSendonly:
			_, err = c.pc.AddTransceiverFromTrack(
				NewTrack(media.Kind),
				webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly},
			)
		case core.DirectionSendRecv:
			// 默认收发器方向为 sendrecv
			_, err = c.pc.AddTransceiverFromTrack(NewTrack(media.Kind))
		default:
			// Nest 摄像头需要数据通道
			_, err = c.pc.CreateDataChannel(media.Kind, nil)
		}

		if err != nil {
			return "", err
		}
	}

	// 2. 创建本地提议
	desc, err := c.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	// 3. 开始收集候选地址
	if err = c.pc.SetLocalDescription(desc); err != nil {
		return "", err
	}

	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) CreateCompleteOffer(medias []*core.Media) (string, error) {
	if _, err := c.CreateOffer(medias); err != nil {
		return "", err
	}

	<-webrtc.GatheringCompletePromise(c.pc)
	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) SetAnswer(answer string) (err error) {
	desc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  fakeFormatsInAnswer(c.pc.LocalDescription().SDP, answer),
	}
	if err = c.pc.SetRemoteDescription(desc); err != nil {
		return err
	}

	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal([]byte(answer)); err != nil {
		return err
	}

	c.Medias = UnmarshalMedias(sd.MediaDescriptions)

	return nil
}

// fakeFormatsInAnswer 修正 pion 解析远端 SDP 的问题：
// pion 仅处理每种媒体的第一个条目的格式，
// 因此将首个提议媒体的全部格式添加到首个应答媒体
func fakeFormatsInAnswer(offer, answer string) string {
	sd2 := &sdp.SessionDescription{}
	if err := sd2.Unmarshal([]byte(answer)); err != nil {
		return answer
	}

	// 检查应答是否包含只接收音频
	var ok bool
	for _, md2 := range sd2.MediaDescriptions {
		if md2.MediaName.Media == "audio" {
			if _, ok = md2.Attribute("recvonly"); ok {
				break
			}
		}
	}
	if !ok {
		return answer
	}

	sd1 := &sdp.SessionDescription{}
	if err := sd1.Unmarshal([]byte(offer)); err != nil {
		return answer
	}

	var formats []string
	var attrs []sdp.Attribute

	for _, md1 := range sd1.MediaDescriptions {
		if md1.MediaName.Media == "audio" {
			for _, attr := range md1.Attributes {
				switch attr.Key {
				case "rtpmap", "fmtp", "rtcp-fb", "extmap":
					attrs = append(attrs, attr)
				}
			}

			formats = md1.MediaName.Formats
			break
		}
	}

	for _, md2 := range sd2.MediaDescriptions {
		if md2.MediaName.Media == "audio" {
			for _, attr := range md2.Attributes {
				switch attr.Key {
				case "rtpmap", "fmtp", "rtcp-fb", "extmap":
				default:
					attrs = append(attrs, attr)
				}
			}

			md2.MediaName.Formats = formats
			md2.Attributes = attrs
			break
		}
	}

	b, err := sd2.Marshal()
	if err != nil {
		return answer
	}

	return string(b)
}
