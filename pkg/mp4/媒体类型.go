package mp4

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
)

const (
	MimeH264 = "avc1.640029"
	MimeH265 = "hvc1.1.6.L153.B0"
	MimeAAC  = "mp4a.40.2"
	MimeFlac = "flac"
	MimeOpus = "opus"
)

func MimeCodecs(codecs []*core.Codec) string {
	var s string

	for i, codec := range codecs {
		if i > 0 {
			s += ","
		}

		switch codec.Name {
		case core.CodecH264:
			s += "avc1." + h264.GetProfileLevelID(codec.FmtpLine)
		case core.CodecH265:
			// H.265 配置档为 main，级别为 5.1
			// hvc1 受 Safari 支持，hev1 不受支持；Chrome 两者都支持
			s += MimeH265
		case core.CodecAAC:
			s += MimeAAC
		case core.CodecOpus:
			s += MimeOpus
		case core.CodecFLAC:
			s += MimeFlac
		}
	}

	return s
}

func ContentType(codecs []*core.Codec) string {
	return `video/mp4; codecs="` + MimeCodecs(codecs) + `"`
}
