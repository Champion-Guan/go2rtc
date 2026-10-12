package opus

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// 此特殊处理的说明：
// - Apple 未采用 RFC 7587，而使用 RFC 3550 的 RTP 时间戳
// - Apple 在局域网中可能请求 20ms 数据包，在 LTE 中请求 60ms 数据包
// - FFmpeg 默认生成 20ms 数据包，每包仅有一帧
// - FFmpeg 应使用 -min_comp 0，让每个包保持相同时长
// - Apple 不关注轨道的实际采样率
// - Apple 只关注基于请求采样率的正确时间戳

// RepackToHAP 将标准 OPUS RTP 包转换为 HAP 包
// 预期条件：
// - 输入数据包时长为 20ms，每包仅有一帧
// - 输出数据包时长为 20ms 或 60ms
// - 输入采样率可以不同；输出 60ms 数据包时不能过大
// - 输出采样率为 16000
// https://github.com/AlexxIT/go2rtc/issues/667
func RepackToHAP(rtpTime byte, handler core.HandlerFunc) core.HandlerFunc {
	switch rtpTime {
	case 20:
		return repackToHAP20(handler)
	case 60:
		return repackToHAP60(handler)
	}
	return handler
}

// pkg/hap/camera/accessory.go 中仅使用一个采样率
const (
	timestamp20 = 16000 * 0.020
	timestamp60 = 16000 * 0.060
)

// repackToHAP20 将 RTP 时间戳从 RFC 7587 修正为 RFC 3550
func repackToHAP20(handler core.HandlerFunc) core.HandlerFunc {
	var timestamp uint32

	return func(pkt *rtp.Packet) {
		timestamp += timestamp20

		clone := *pkt
		clone.Timestamp = timestamp
		handler(&clone)
	}
}

// repackToHAP60 将 20 ms 帧合并为单个 60 ms 数据包
// 感谢 @civita 的思路：https://github.com/AlexxIT/go2rtc/pull/843
func repackToHAP60(handler core.HandlerFunc) core.HandlerFunc {
	var sequence uint16
	var timestamp uint32

	var framesCount byte
	var framesSize []byte
	var framesData []byte

	return func(pkt *rtp.Packet) {
		framesData = append(framesData, pkt.Payload[1:]...)

		if framesCount++; framesCount < 3 {
			if frameSize := len(pkt.Payload) - 1; frameSize >= 252 {
				b0 := 252 + byte(frameSize)&0b11
				framesSize = append(framesSize, b0, byte(frameSize/4)-b0)
			} else {
				framesSize = append(framesSize, byte(frameSize))
			}
			return
		}

		toc := pkt.Payload[0]

		payload := make([]byte, 2, 2+len(framesSize)+len(framesData))
		payload[0] = toc | 0b11  // code 3 (multiple frames per packet)
		payload[1] = 0b1000_0011 // VBR, no padding, 3 frames
		payload = append(payload, framesSize...)
		payload = append(payload, framesData...)

		sequence++
		timestamp += timestamp60

		clone := *pkt
		clone.Payload = payload
		clone.SequenceNumber = sequence
		clone.Timestamp = timestamp
		handler(&clone)

		framesCount = 0
		framesSize = framesSize[:0]
		framesData = framesData[:0]
	}
}
