package core

import "encoding/json"

const (
	DirectionRecvonly = "recvonly"
	DirectionSendonly = "sendonly"
	DirectionSendRecv = "sendrecv"
)

const (
	KindVideo = "video"
	KindAudio = "audio"
)

const (
	CodecH264 = "H264" // payloadType: 96
	CodecH265 = "H265"
	CodecVP8  = "VP8"
	CodecVP9  = "VP9"
	CodecAV1  = "AV1"
	CodecJPEG = "JPEG" // payloadType: 26
	CodecRAW  = "RAW"

	CodecPCMU = "PCMU" // payloadType: 0
	CodecPCMA = "PCMA" // payloadType: 8
	CodecAAC  = "MPEG4-GENERIC"
	CodecOpus = "OPUS" // payloadType: 111
	CodecG722 = "G722"
	CodecMP3  = "MPA" // payload: 14, aka MPEG-1 Layer III
	CodecPCM  = "L16" // Linear PCM (big endian)

	CodecPCML = "PCML" // Linear PCM (little endian)

	CodecELD  = "ELD" // AAC-ELD
	CodecFLAC = "FLAC"

	CodecAll = "ALL"
	CodecAny = "ANY"
)

const PayloadTypeRAW byte = 255

type Producer interface {
	// GetMedias 返回使用本地 Media.Direction 的媒体列表：
	// - 生产者视频/音频使用 recvonly
	// - 生产者反向通道使用 sendonly
	GetMedias() []*Media

	// GetTrack 返回只能产生 rtp.Packet 的接收者
	GetTrack(media *Media, codec *Codec) (*Receiver, error)

	// Deprecated: 应重命名为 Run()
	Start() error

	// Deprecated: 应重命名为 Close()
	Stop() error
}

type Consumer interface {
	// GetMedias 返回使用本地 Media.Direction 的媒体列表：
	// - 消费者视频/音频使用 sendonly
	// - 消费者反向通道使用 recvonly
	GetMedias() []*Media

	AddTrack(media *Media, codec *Codec, track *Receiver) error

	// Deprecated: 应重命名为 Close()
	Stop() error
}

type Mode byte

const (
	ModeActiveProducer Mode = iota + 1 // typical source (client)
	ModePassiveConsumer
	ModePassiveProducer
	ModeActiveConsumer
)

func (m Mode) String() string {
	switch m {
	case ModeActiveProducer:
		return "active producer"
	case ModePassiveConsumer:
		return "passive consumer"
	case ModePassiveProducer:
		return "passive producer"
	case ModeActiveConsumer:
		return "active consumer"
	}
	return "unknown"
}

func (m Mode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}
