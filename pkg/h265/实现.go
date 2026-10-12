package h265

import "github.com/AlexxIT/go2rtc/pkg/h264"

const forbiddenZeroBit = 0x80
const nalUnitType = 0x3F

// Deprecated: DecodeStream 查找并返回 AVC 格式的第一个访问单元
// 用于处理分隔符大小未知的实时流
func DecodeStream(annexb []byte) ([]byte, int) {
	startPos := -1

	i := 0
	for {
		// 查找下一个分隔符
		if i = h264.IndexFrom(annexb, []byte{0, 0, 1}, i); i < 0 {
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

		nalType := (octet >> 1) & nalUnitType
		if startPos >= 0 {
			switch nalType {
			case NALUTypeVPS, NALUTypePFrame:
				if annexb[i-4] == 0 {
					return h264.DecodeAnnexB(annexb[startPos : i-4]), i - 4
				} else {
					return h264.DecodeAnnexB(annexb[startPos : i-3]), i - 3
				}
			}
		} else {
			switch nalType {
			case NALUTypeVPS, NALUTypePFrame:
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
