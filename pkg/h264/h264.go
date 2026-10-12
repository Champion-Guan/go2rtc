package h264

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const (
	NALUTypePFrame = 1 // Coded slice of a non-IDR picture
	NALUTypeIFrame = 5 // Coded slice of an IDR picture
	NALUTypeSEI    = 6 // Supplemental enhancement information (SEI)
	NALUTypeSPS    = 7 // Sequence parameter set
	NALUTypePPS    = 8 // Picture parameter set
	NALUTypeAUD    = 9 // Access unit delimiter
)

func NALUType(b []byte) byte {
	return b[4] & 0x1F
}

// IsKeyframe 检查访问单元中是否有关键帧 NALU
func IsKeyframe(b []byte) bool {
	for {
		switch NALUType(b) {
		case NALUTypePFrame:
			return false
		case NALUTypeIFrame:
			return true
		}

		size := int(binary.BigEndian.Uint32(b)) + 4
		if size < len(b) {
			b = b[size:]
			continue
		} else {
			return false
		}
	}
}

func Join(ps, iframe []byte) []byte {
	b := make([]byte, len(ps)+len(iframe))
	i := copy(b, ps)
	copy(b[i:], iframe)
	return b
}

// https://developers.google.com/cast/docs/media
const (
	ProfileBaseline    = 0x42
	ProfileMain        = 0x4D
	ProfileHigh        = 0x64
	CapabilityBaseline = 0xE0
	CapabilityMain     = 0x40
)

// GetProfileLevelID 从 fmtp 行获取配置档
// 部分设备无法播放高级别视频，因此限制最大配置档和级别，
// 即使 fmtp 为空，也返回一个配置档
func GetProfileLevelID(fmtp string) string {
	// avc1.640029 表示 H.264 High 4.1，适用于第一、二代 Chromecast
	profile := byte(ProfileHigh)
	capab := byte(0)
	level := byte(41)

	if fmtp != "" {
		var conf []byte
		// 部分摄像头提供错误的 profile-level-id
		// https://github.com/AlexxIT/go2rtc/issues/155
		if s := core.Between(fmtp, "sprop-parameter-sets=", ","); s != "" {
			if sps, _ := base64.StdEncoding.DecodeString(s); len(sps) >= 4 {
				conf = sps[1:4]
			}
		} else if s = core.Between(fmtp, "profile-level-id=", ";"); s != "" {
			conf, _ = hex.DecodeString(s)
		}

		if len(conf) == 3 {
			// 将配置档、能力和级别限制为受支持值
			switch conf[0] {
			case ProfileBaseline, ProfileMain:
				profile = conf[0]
			}
			switch conf[1] {
			case CapabilityBaseline, CapabilityMain:
				capab = conf[1]
			}
			switch conf[2] {
			case 30, 31, 40:
				level = conf[2]
			}
		}
	}

	return fmt.Sprintf("%02X%02X%02X", profile, capab, level)
}

func GetParameterSet(fmtp string) (sps, pps []byte) {
	if fmtp == "" {
		return
	}

	s := core.Between(fmtp, "sprop-parameter-sets=", ";")
	if s == "" {
		return
	}

	i := strings.IndexByte(s, ',')
	if i < 0 {
		return
	}

	sps, _ = base64.StdEncoding.DecodeString(s[:i])
	pps, _ = base64.StdEncoding.DecodeString(s[i+1:])

	return
}

// GetFmtpLine 从 AVC 格式的 SPS、PPS 和关键帧获取 fmtp 行
func GetFmtpLine(avc []byte) string {
	s := "packetization-mode=1"

	for {
		size := 4 + int(binary.BigEndian.Uint32(avc))

		switch NALUType(avc) {
		case NALUTypeSPS:
			s += ";profile-level-id=" + hex.EncodeToString(avc[5:8])
			s += ";sprop-parameter-sets=" + base64.StdEncoding.EncodeToString(avc[4:size])
		case NALUTypePPS:
			s += "," + base64.StdEncoding.EncodeToString(avc[4:size])
		}

		if size < len(avc) {
			avc = avc[size:]
		} else {
			return s
		}
	}
}
