package h264

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAVC = 0

const PSMaxSize = 128 // the biggest SPS I've seen is 48 (EZVIZ CS-CV210)

func RTPDepay(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	depack := &codecs.H264Packet{IsAVC: true}

	sps, pps := GetParameterSet(codec.FmtpLine)
	ps := JoinNALU(sps, pps)

	buf := make([]byte, 0, 512*1024) // 512K

	return func(packet *rtp.Packet) {
		//log.Printf("[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, %v", codec.Name, packet.Payload[0]&0x1F, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

		payload, err := depack.Unmarshal(packet.Payload)
		if len(payload) == 0 || err != nil {
			return
		}

		// 内存溢出保护，丢失大量带结束标记的包时可能发生
		// https://github.com/AlexxIT/go2rtc/issues/675
		if len(buf) > 5*1024*1024 {
			buf = buf[: 0 : 512*1024]
		}

		// 修正 TP-Link Tapo TC70：发送 SPS 和 PPS 时 packet.Marker 为 true
		// Reolink Duo 2 的 SPS 带结束标记，PPS 不带
		if packet.Marker && len(payload) < PSMaxSize {
			switch NALUType(payload) {
			case NALUTypeSPS, NALUTypePPS:
				buf = append(buf, payload...)
				return
			case NALUTypeSEI:
				// RTSP 服务器：https://github.com/AlexxIT/go2rtc/issues/244
				// 依次发送带结束标记的 SPS、PPS、SEI 和关键帧
				return
			}
		}

		if len(buf) == 0 {
			for {
				// Amcrest IP4M-1051: 9, 7, 8, 6, 28...
				// Amcrest IP4M-1051: 9, 6, 1
				switch NALUType(payload) {
				case NALUTypeIFrame:
					// 修正缺少 SPS/PPS 的关键帧
					buf = append(buf, ps...)
				case NALUTypeSEI, NALUTypeAUD:
					// 修正 FFmpeg 转码首帧
					i := int(4 + binary.BigEndian.Uint32(payload))

					// 检查是否仅有一个 NAL，修正 Reolink RLC-510A 的 FFmpeg 转码
					if i == len(payload) {
						return
					}

					payload = payload[i:]
					continue
				case NALUTypePFrame, NALUTypeSPS, NALUTypePPS: // pass
				default:
					return // skip any unknown NAL unit type
				}
				break
			}
		}

		// 收集访问单元的全部 NAL
		if !packet.Marker {
			buf = append(buf, payload...)
			return
		}

		if len(buf) > 0 {
			payload = append(buf, payload...)
			buf = buf[:0]
		}

		// SPS 不应过大
		if NALUType(payload) == NALUTypeSPS && binary.BigEndian.Uint32(payload) >= PSMaxSize {
			// 部分异常摄像头将 SPS、PPS 和关键帧放在单包中，用 00 00 00 01 分隔
			// https://github.com/AlexxIT/WebRTC/issues/391
			// https://github.com/AlexxIT/WebRTC/issues/392
			payload = annexb.FixAnnexBInAVCC(payload)
		}

		//log.Printf("[AVC] %v, len: %d, ts: %10d, seq: %d", NALUTypes(payload), len(payload), packet.Timestamp, packet.SequenceNumber)

		clone := *packet
		clone.Version = RTPPacketVersionAVC
		clone.Payload = payload
		handler(&clone)
	}
}

func RTPPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	if mtu == 0 {
		mtu = 1472
	}

	payloader := &Payloader{IsAVC: true}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != RTPPacketVersionAVC {
			handler(packet)
			return
		}

		payloads := payloader.Payload(mtu, packet.Payload)
		last := len(payloads) - 1
		for i, payload := range payloads {
			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         i == last,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: payload,
			}
			handler(&clone)
		}
	}
}
