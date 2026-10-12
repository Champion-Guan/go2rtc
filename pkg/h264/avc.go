package h264

import (
	"bytes"
	"encoding/binary"
)

const forbiddenZeroBit = 0x80
const nalUnitType = 0x1F

// Deprecated: DecodeStream 查找并返回 AVC 格式的第一个访问单元
// 用于处理分隔符大小未知的实时流
func DecodeStream(annexb []byte) ([]byte, int) {
	startPos := -1

	i := 0
	for {
		// 查找下一个分隔符
		if i = IndexFrom(annexb, []byte{0, 0, 1}, i); i < 0 {
			break
		}

		// 将 i 移到下一个访问单元
		if i += 3; i >= len(annexb) {
			break
		}

		// 检查访问单元类型是否有效
		octet := annexb[i]
		if octet&forbiddenZeroBit != 0 {
			continue
		}

		// 0 => AUD => SPS/IF/PF => AUD
		// 0 => SPS/PF => SPS/PF
		nalType := octet & nalUnitType
		if startPos >= 0 {
			switch nalType {
			case NALUTypeAUD, NALUTypeSPS, NALUTypePFrame:
				if annexb[i-4] == 0 {
					return DecodeAnnexB(annexb[startPos : i-4]), i - 4
				} else {
					return DecodeAnnexB(annexb[startPos : i-3]), i - 3
				}
			}
		} else {
			switch nalType {
			case NALUTypeSPS, NALUTypePFrame:
				if i >= 4 && annexb[i-4] == 0 {
					startPos = i - 4
				} else {
					startPos = i - 3
				}
			}
		}
	}

	return nil, 0
}

// DecodeAnnexB 将 Annex-B 转换为 AVC
// 支持未知大小的分隔符
func DecodeAnnexB(b []byte) []byte {
	if b[2] == 1 {
		// convert: 0 0 1 => 0 0 0 1
		b = append([]byte{0}, b...)
	}

	startPos := 0

	i := 4
	for {
		// 查找下一个分隔符
		if i = IndexFrom(b, []byte{0, 0, 1}, i); i < 0 {
			break
		}

		// 将 i 移到下一个访问单元
		if i += 3; i >= len(b) {
			break
		}

		// 检查访问单元类型是否有效
		octet := b[i]
		if octet&forbiddenZeroBit != 0 {
			continue
		}

		switch octet & nalUnitType {
		case NALUTypePFrame, NALUTypeIFrame, NALUTypeSPS, NALUTypePPS:
			if b[i-4] != 0 {
				// prefix: 0 0 1
				binary.BigEndian.PutUint32(b[startPos:], uint32(i-startPos-7))
				tmp := make([]byte, 0, len(b)+1)
				tmp = append(tmp, b[:i]...)
				tmp = append(tmp, 0)
				b = append(tmp, b[i:]...)
				startPos = i - 3
			} else {
				// prefix: 0 0 0 1
				binary.BigEndian.PutUint32(b[startPos:], uint32(i-startPos-8))
				startPos = i - 4
			}
		}
	}

	binary.BigEndian.PutUint32(b[startPos:], uint32(len(b)-startPos-4))
	return b
}

func IndexFrom(b []byte, sep []byte, from int) int {
	if from > 0 {
		if from < len(b) {
			if i := bytes.Index(b[from:], sep); i >= 0 {
				return from + i
			}
		}
		return -1
	}

	return bytes.Index(b, sep)
}
