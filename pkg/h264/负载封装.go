package h264

import "encoding/binary"

// Payloader 封装 H264 数据包
type Payloader struct {
	IsAVC     bool
	stapANalu []byte
}

const (
	stapaNALUType  = 24
	fuaNALUType    = 28
	fubNALUType    = 29
	spsNALUType    = 7
	ppsNALUType    = 8
	audNALUType    = 9
	fillerNALUType = 12

	fuaHeaderSize = 2
	//stapaHeaderSize     = 1
	//stapaNALULengthSize = 2

	naluTypeBitmask   = 0x1F
	naluRefIdcBitmask = 0x60
	//fuStartBitmask    = 0x80
	//fuEndBitmask      = 0x40

	outputStapAHeader = 0x78
)

//func annexbNALUStartCode() []byte { return []byte{0x00, 0x00, 0x00, 0x01} }

func EmitNalus(nals []byte, isAVC bool, emit func([]byte)) {
	if !isAVC {
		nextInd := func(nalu []byte, start int) (indStart int, indLen int) {
			zeroCount := 0

			for i, b := range nalu[start:] {
				if b == 0 {
					zeroCount++
					continue
				} else if b == 1 {
					if zeroCount >= 2 {
						return start + i - zeroCount, zeroCount + 1
					}
				}
				zeroCount = 0
			}
			return -1, -1
		}

		nextIndStart, nextIndLen := nextInd(nals, 0)
		if nextIndStart == -1 {
			emit(nals)
		} else {
			for nextIndStart != -1 {
				prevStart := nextIndStart + nextIndLen
				nextIndStart, nextIndLen = nextInd(nals, prevStart)
				if nextIndStart != -1 {
					emit(nals[prevStart:nextIndStart])
				} else {
					// 未找到结束标记，输出到流结束
					emit(nals[prevStart:])
				}
			}
		}
	} else {
		for {
			n := uint32(len(nals))
			if n < 4 {
				break
			}
			end := 4 + binary.BigEndian.Uint32(nals)
			if n < end {
				break
			}
			emit(nals[4:end])
			nals = nals[end:]
		}
	}
}

// Payload 将 H264 数据包分片为一个或多个字节数组
func (p *Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	var payloads [][]byte
	if len(payload) == 0 {
		return payloads
	}

	EmitNalus(payload, p.IsAVC, func(nalu []byte) {
		if len(nalu) == 0 {
			return
		}

		naluType := nalu[0] & naluTypeBitmask
		naluRefIdc := nalu[0] & naluRefIdcBitmask

		switch naluType {
		case audNALUType, fillerNALUType:
			return
		case spsNALUType, ppsNALUType:
			if p.stapANalu == nil {
				p.stapANalu = []byte{outputStapAHeader}
			}
			p.stapANalu = append(p.stapANalu, byte(len(nalu)>>8), byte(len(nalu)))
			p.stapANalu = append(p.stapANalu, nalu...)
			return
		}

		if p.stapANalu != nil {
			// 将当前 NALU 与 SPS、PPS 封装为 STAP-A
			// 支持连续多个 PPS
			if len(p.stapANalu) <= int(mtu) {
				payloads = append(payloads, p.stapANalu)
			}
			p.stapANalu = nil
		}

		// 单个 NALU
		if len(nalu) <= int(mtu) {
			out := make([]byte, len(nalu))
			copy(out, nalu)
			payloads = append(payloads, out)
			return
		}

		// FU-A
		maxFragmentSize := int(mtu) - fuaHeaderSize

		// FU 负载由原始 NAL 单元负载的分片组成；
		// 按顺序拼接连续 FU 的负载后，
		// 即可重建原始 NAL 单元的负载。
		// 原始 NAL 单元类型字节不直接包含在
		// 分片负载中，其信息改由
		// 分片单元 FU 指示字节中的
		// F 和 NRI 字段，
		// 以及 FU 头的类型字段传递。
		// FU 负载可包含任意数量字节，
		// 也可以为空。

		naluData := nalu
		// 按 RFC 规定，首字节的信息冗余，因此跳过
		naluDataIndex := 1
		naluDataLength := len(nalu) - naluDataIndex
		naluDataRemaining := naluDataLength

		if min(maxFragmentSize, naluDataRemaining) <= 0 {
			return
		}

		for naluDataRemaining > 0 {
			currentFragmentSize := min(maxFragmentSize, naluDataRemaining)
			out := make([]byte, fuaHeaderSize+currentFragmentSize)

			// +---------------+
			// |0|1|2|3|4|5|6|7|
			// +-+-+-+-+-+-+-+-+
			// |F|NRI|  Type   |
			// +---------------+
			out[0] = fuaNALUType
			out[0] |= naluRefIdc

			// +---------------+
			// |0|1|2|3|4|5|6|7|
			// +-+-+-+-+-+-+-+-+
			// |S|E|R|  Type   |
			// +---------------+

			out[1] = naluType
			if naluDataRemaining == naluDataLength {
				// 设置起始位
				out[1] |= 1 << 7
			} else if naluDataRemaining-currentFragmentSize == 0 {
				// 设置结束位
				out[1] |= 1 << 6
			}

			copy(out[fuaHeaderSize:], naluData[naluDataIndex:naluDataIndex+currentFragmentSize])
			payloads = append(payloads, out)

			naluDataRemaining -= currentFragmentSize
			naluDataIndex += currentFragmentSize
		}
	})

	return payloads
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
