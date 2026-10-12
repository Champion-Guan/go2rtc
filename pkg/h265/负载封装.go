package h265

import (
	"encoding/binary"
	"math"

	"github.com/AlexxIT/go2rtc/pkg/h264"
)

//
// 网络抽象层单元头实现
//

const (
	// sizeof(uint16)
	h265NaluHeaderSize = 2
	// https://datatracker.ietf.org/doc/html/rfc7798#section-4.4.2
	h265NaluAggregationPacketType = 48
	// https://datatracker.ietf.org/doc/html/rfc7798#section-4.4.3
	h265NaluFragmentationUnitType = 49
	// https://datatracker.ietf.org/doc/html/rfc7798#section-4.4.4
	h265NaluPACIPacketType = 50
)

// H265NALUHeader 是 H265 NAL 单元头
// https://datatracker.ietf.org/doc/html/rfc7798#section-1.1.4
// +---------------+---------------+
//
//	|0|1|2|3|4|5|6|7|0|1|2|3|4|5|6|7|
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|F|   Type    |  LayerID  | TID |
//	+-------------+-----------------+
type H265NALUHeader uint16

func newH265NALUHeader(highByte, lowByte uint8) H265NALUHeader {
	return H265NALUHeader((uint16(highByte) << 8) | uint16(lowByte))
}

// F 是禁止位，应始终为 0。
func (h H265NALUHeader) F() bool {
	return (uint16(h) >> 15) != 0
}

// NAL 单元类型。
func (h H265NALUHeader) Type() uint8 {
	// 01111110 00000000
	const mask = 0b01111110 << 8
	return uint8((uint16(h) & mask) >> (8 + 1))
}

// IsTypeVCLUnit 判断 NAL 类型是否为 VCL 单元。
func (h H265NALUHeader) IsTypeVCLUnit() bool {
	// 类型编码为 6 位
	const msbMask = 0b00100000
	return (h.Type() & msbMask) == 0
}

// 非三维 HEVC 中，LayerID 应始终为 0。
func (h H265NALUHeader) LayerID() uint8 {
	// 00000001 11111000
	const mask = (0b00000001 << 8) | 0b11111000
	return uint8((uint16(h) & mask) >> 3)
}

// TID 为 NAL 单元时间层标识加 1。
func (h H265NALUHeader) TID() uint8 {
	const mask = 0b00000111
	return uint8(uint16(h) & mask)
}

// IsAggregationPacket 判断是否为聚合包。
func (h H265NALUHeader) IsAggregationPacket() bool {
	return h.Type() == h265NaluAggregationPacketType
}

// IsFragmentationUnit 判断是否为分片单元包。
func (h H265NALUHeader) IsFragmentationUnit() bool {
	return h.Type() == h265NaluFragmentationUnitType
}

// IsPACIPacket 判断是否为 PACI 包。
func (h H265NALUHeader) IsPACIPacket() bool {
	return h.Type() == h265NaluPACIPacketType
}

//
// 分片单元实现
//

const (
	// sizeof(uint8)
	h265FragmentationUnitHeaderSize = 1
)

// H265FragmentationUnitHeader 表示 H265 分片单元头
// +---------------+
// |0|1|2|3|4|5|6|7|
// +-+-+-+-+-+-+-+-+
// |S|E|  FuType   |
// +---------------+
type H265FragmentationUnitHeader uint8

// S 表示分片 NAL 单元开始。
func (h H265FragmentationUnitHeader) S() bool {
	const mask = 0b10000000
	return ((h & mask) >> 7) != 0
}

// E 表示分片 NAL 单元结束。
func (h H265FragmentationUnitHeader) E() bool {
	const mask = 0b01000000
	return ((h & mask) >> 6) != 0
}

// FuType 必须等于原始 NAL 单元的 Type。
func (h H265FragmentationUnitHeader) FuType() uint8 {
	const mask = 0b00111111
	return uint8(h) & mask
}

// Payloader 封装 H265 数据包
type Payloader struct {
	AddDONL         bool
	SkipAggregation bool
	donl            uint16
}

// Payload 将 H265 数据包分片为一个或多个字节数组
func (p *Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	var payloads [][]byte
	if len(payload) == 0 {
		return payloads
	}

	bufferedNALUs := make([][]byte, 0)
	aggregationBufferSize := 0

	flushBufferedNals := func() {
		if len(bufferedNALUs) == 0 {
			return
		}
		if len(bufferedNALUs) == 1 {
			// 作为单个 NALU 数据包输出
			nalu := bufferedNALUs[0]

			if p.AddDONL {
				buf := make([]byte, len(nalu)+2)

				// 将 NALU 头复制到负载头
				copy(buf[0:h265NaluHeaderSize], nalu[0:h265NaluHeaderSize])

				// 将 DONL 复制到头部
				binary.BigEndian.PutUint16(buf[h265NaluHeaderSize:h265NaluHeaderSize+2], p.donl)

				// 写入负载
				copy(buf[h265NaluHeaderSize+2:], nalu[h265NaluHeaderSize:])

				p.donl++

				payloads = append(payloads, buf)
			} else {
				// 直接将 NALU 写入负载
				payloads = append(payloads, nalu)
			}
		} else {
			// 构造聚合包
			aggregationPacketSize := aggregationBufferSize + 2
			buf := make([]byte, aggregationPacketSize)

			layerID := uint8(math.MaxUint8)
			tid := uint8(math.MaxUint8)
			for _, nalu := range bufferedNALUs {
				header := newH265NALUHeader(nalu[0], nalu[1])
				headerLayerID := header.LayerID()
				headerTID := header.TID()
				if headerLayerID < layerID {
					layerID = headerLayerID
				}
				if headerTID < tid {
					tid = headerTID
				}
			}

			binary.BigEndian.PutUint16(buf[0:2], (uint16(h265NaluAggregationPacketType)<<9)|(uint16(layerID)<<3)|uint16(tid))

			index := 2
			for i, nalu := range bufferedNALUs {
				if p.AddDONL {
					if i == 0 {
						binary.BigEndian.PutUint16(buf[index:index+2], p.donl)
						index += 2
					} else {
						buf[index] = byte(i - 1)
						index++
					}
				}
				binary.BigEndian.PutUint16(buf[index:index+2], uint16(len(nalu)))
				index += 2
				index += copy(buf[index:], nalu)
			}
			payloads = append(payloads, buf)
		}
		// 清空已缓冲的 NALU
		bufferedNALUs = make([][]byte, 0)
		aggregationBufferSize = 0
	}

	h264.EmitNalus(payload, true, func(nalu []byte) {
		if len(nalu) == 0 {
			return
		}

		if len(nalu) <= int(mtu) {
			// 此 NALU 可放入单个包，既可单独输出，
			// 也可追加到前一个聚合包

			marginalAggregationSize := len(nalu) + 2
			if p.AddDONL {
				marginalAggregationSize += 1
			}

			if aggregationBufferSize+marginalAggregationSize > int(mtu) {
				flushBufferedNals()
			}
			bufferedNALUs = append(bufferedNALUs, nalu)
			aggregationBufferSize += marginalAggregationSize
			if p.SkipAggregation {
				// 立即输出
				flushBufferedNals()
			}
		} else {
			// 此 NALU 超出当前 MTU，需要分片
			fuPacketHeaderSize := h265FragmentationUnitHeaderSize + 2 /* payload header size */
			if p.AddDONL {
				fuPacketHeaderSize += 2
			}

			// 对 NALU 进行分片
			maxFUPayloadSize := int(mtu) - fuPacketHeaderSize

			naluHeader := newH265NALUHeader(nalu[0], nalu[1])

			// 分片包负载中省略 NALU 头
			nalu = nalu[h265NaluHeaderSize:]

			if maxFUPayloadSize == 0 || len(nalu) == 0 {
				return
			}

			// 输出全部已缓冲聚合包
			flushBufferedNals()

			fullNALUSize := len(nalu)
			for len(nalu) > 0 {
				curentFUPayloadSize := len(nalu)
				if curentFUPayloadSize > maxFUPayloadSize {
					curentFUPayloadSize = maxFUPayloadSize
				}

				out := make([]byte, fuPacketHeaderSize+curentFUPayloadSize)

				// 写入负载 header
				binary.BigEndian.PutUint16(out[0:2], uint16(naluHeader))
				out[0] = (out[0] & 0b10000001) | h265NaluFragmentationUnitType<<1

				// 写入分片头
				out[2] = byte(H265FragmentationUnitHeader(naluHeader.Type()))
				if len(nalu) == fullNALUSize {
					// 设置起始位
					out[2] |= 1 << 7
				} else if len(nalu)-curentFUPayloadSize == 0 {
					// 设置结束位
					out[2] |= 1 << 6
				}

				if p.AddDONL {
					// 写入 DONL 头
					binary.BigEndian.PutUint16(out[3:5], p.donl)

					p.donl++

					// 复制分片负载
					copy(out[5:], nalu[0:curentFUPayloadSize])
				} else {
					// 复制分片负载
					copy(out[3:], nalu[0:curentFUPayloadSize])
				}

				// 将分片追加到负载
				payloads = append(payloads, out)

				// 推进 NALU 数据指针
				nalu = nalu[curentFUPayloadSize:]
			}
		}
	})

	flushBufferedNals()

	return payloads
}
