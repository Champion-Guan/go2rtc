package tuya

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/core"
	pionWebrtc "github.com/pion/webrtc/v4"
)

type TuyaAPI interface {
	GetMqtt() *TuyaMqttClient

	GetStreamType(streamResolution string) int
	IsHEVC(streamType int) bool

	GetVideoCodecs() []*core.Codec
	GetAudioCodecs() []*core.Codec

	GetStreamUrl(streamUrl string) (string, error)
	GetICEServers() []pionWebrtc.ICEServer

	Init() error
	Close()
}

type TuyaClient struct {
	TuyaAPI

	httpClient *http.Client
	mqtt       *TuyaMqttClient
	baseUrl    string
	expireTime int64
	deviceId   string
	localKey   string
	skill      *Skill
	iceServers []pionWebrtc.ICEServer
}

type AudioAttributes struct {
	CallMode           []int `json:"call_mode"`           // 1 = one way, 2 = two way
	HardwareCapability []int `json:"hardware_capability"` // 1 = mic, 2 = speaker
}

type ICEServer struct {
	Urls       string `json:"urls"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
	TTL        int    `json:"ttl,omitempty"`
}

type WebICE struct {
	Urls       string `json:"urls"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

type P2PConfig struct {
	Ices []ICEServer `json:"ices"`
}

type AudioSkill struct {
	Channels   int `json:"channels"`
	DataBit    int `json:"dataBit"`
	CodecType  int `json:"codecType"`
	SampleRate int `json:"sampleRate"`
}

type VideoSkill struct {
	StreamType int    `json:"streamType"` // 2 = main stream (HD), 4 = sub stream (SD)
	CodecType  int    `json:"codecType"`  // 2 = H264, 4 = H265 (HEVC)
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SampleRate int    `json:"sampleRate"`
	ProfileId  string `json:"profileId,omitempty"`
}

type Skill struct {
	WebRTC   int          `json:"webrtc"`             // Bit flags: bit 4=speaker, bit 5=clarity, bit 6=record
	LowPower int          `json:"lowPower,omitempty"` // 1 = battery-powered camera
	Audios   []AudioSkill `json:"audios"`
	Videos   []VideoSkill `json:"videos"`
}

type WebRTCConfig struct {
	AudioAttributes      AudioAttributes `json:"audio_attributes"`
	Auth                 string          `json:"auth"`
	ID                   string          `json:"id"`
	LocalKey             string          `json:"local_key,omitempty"`
	MotoID               string          `json:"moto_id"`
	P2PConfig            P2PConfig       `json:"p2p_config"`
	ProtocolVersion      string          `json:"protocol_version"`
	Skill                string          `json:"skill"`
	SupportsWebRTCRecord bool            `json:"supports_webrtc_record"`
	SupportsWebRTC       bool            `json:"supports_webrtc"`
	VedioClaritiy        int             `json:"vedio_clarity"`
	VideoClaritiy        int             `json:"video_clarity"`
	VideoClarities       []int           `json:"video_clarities"`
}

type MQTTConfig struct {
	Url            string `json:"url"`
	PublishTopic   string `json:"publish_topic"`
	SubscribeTopic string `json:"subscribe_topic"`
	ClientID       string `json:"client_id"`
	Username       string `json:"username"`
	Password       string `json:"password"`
}

type Allocate struct {
	URL string `json:"url"`
}

type AllocateRequest struct {
	Type string `json:"type"`
}

type AllocateResponse struct {
	Success bool     `json:"success"`
	Result  Allocate `json:"result"`
	Msg     string   `json:"msg,omitempty"`
}

func (c *TuyaClient) GetICEServers() []pionWebrtc.ICEServer {
	return c.iceServers
}

func (c *TuyaClient) GetMqtt() *TuyaMqttClient {
	return c.mqtt
}

// GetStreamType 返回指定分辨率的 Skill 流类型
// 返回 Skill 值 2 或 4，而非 MQTT 值 0 或 1
// - hd：最高分辨率的 streamType（通常 2 为主码流）
// - sd：最低分辨率的 streamType（通常 4 为子码流）
//
// 发送到 MQTT 前必须映射这些值：
// - streamType 2 对应 MQTT stream_type 0
// - streamType 4 对应 MQTT stream_type 1
func (c *TuyaClient) GetStreamType(streamResolution string) int {
	// 未找到匹配时使用默认流类型
	defaultStreamType := 1

	if c.skill == nil || len(c.skill.Videos) == 0 {
		return defaultStreamType
	}

	// 按像素数量查找最高和最低分辨率
	var highestResType = defaultStreamType
	var highestRes = 0
	var lowestResType = defaultStreamType
	var lowestRes = 0

	for _, video := range c.skill.Videos {
		res := video.Width * video.Height

		// 最高分辨率
		if res > highestRes {
			highestRes = res
			highestResType = video.StreamType
		}

		// 最低分辨率，未设置时使用第一项
		if lowestRes == 0 || res < lowestRes {
			lowestRes = res
			lowestResType = video.StreamType
		}
	}

	// 根据选择返回流类型
	switch streamResolution {
	case "hd":
		return highestResType
	case "sd":
		return lowestResType
	default:
		return defaultStreamType
	}
}

// IsHEVC 检查流类型是否使用 H265 编码
// HEVC 摄像头使用数据通道，H264 使用 RTP 轨道
// - codecType 4 = H265（HEVC），对应 DataChannel 模式
// - codecType 2 = H264，对应标准 RTP 模式
func (c *TuyaClient) IsHEVC(streamType int) bool {
	for _, video := range c.skill.Videos {
		if video.StreamType == streamType {
			return video.CodecType == 4 // 4 = H265/HEVC
		}
	}

	return false
}

func (c *TuyaClient) GetVideoCodecs() []*core.Codec {
	if len(c.skill.Videos) > 0 {
		codecs := make([]*core.Codec, 0)

		for _, video := range c.skill.Videos {
			name := core.CodecH264
			if c.IsHEVC(video.StreamType) {
				name = core.CodecH265
			}

			codec := &core.Codec{
				Name:      name,
				ClockRate: uint32(video.SampleRate),
			}

			codecs = append(codecs, codec)
		}

		if len(codecs) > 0 {
			return codecs
		}
	}

	return nil
}

func (c *TuyaClient) GetAudioCodecs() []*core.Codec {
	if len(c.skill.Audios) > 0 {
		codecs := make([]*core.Codec, 0)

		for _, audio := range c.skill.Audios {
			name := getAudioCodecName(&audio)

			codec := &core.Codec{
				Name:      name,
				ClockRate: uint32(audio.SampleRate),
				Channels:  uint8(audio.Channels),
			}
			codecs = append(codecs, codec)
		}

		if len(codecs) > 0 {
			return codecs
		}
	}

	return nil
}

func (c *TuyaClient) Close() {
	c.mqtt.Stop()
	c.httpClient.CloseIdleConnections()
}

// https://protect-us.ismartlife.me/
func getAudioCodecName(audioSkill *AudioSkill) string {
	switch audioSkill.CodecType {
	// case 100:
	// 	return "ADPCM"
	case 101:
		return core.CodecPCML
	case 102, 103, 104:
		return core.CodecAAC
	case 105:
		return core.CodecPCMU
	case 106:
		return core.CodecPCMA
	// case 107:
	// 	return "G726-32"
	// case 108:
	// 	return "SPEEX"
	case 109:
		return core.CodecMP3
	default:
		return core.CodecPCML
	}
}
