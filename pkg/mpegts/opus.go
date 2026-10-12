package mpegts

import (
	"github.com/AlexxIT/go2rtc/pkg/bits"
)

// opusDT：FFmpeg 的每个音频单元包含 5 个 OPUS 包，每包在 48000 时钟下长 960
const opusDT = 960 * ClockRate / 48000

// https://opus-codec.org/docs/
var opusInfo = []byte{ // registration_descriptor
	0x05,               // descriptor_tag
	0x04,               // descriptor_length
	'O', 'p', 'u', 's', // format_identifier
}

//goland:noinspection GoSnakeCaseUsage
func CutOPUSPacket(b []byte) (packet []byte, left []byte) {
	r := bits.NewReader(b)

	size := opus_control_header(r)
	if size == 0 {
		return nil, nil
	}

	packet = r.ReadBytes(size)
	left = r.Left()
	return
}

//goland:noinspection GoSnakeCaseUsage
func opus_control_header(r *bits.Reader) int {
	control_header_prefix := r.ReadBits(11)
	if control_header_prefix != 0x3FF {
		return 0
	}

	start_trim_flag := r.ReadBit()
	end_trim_flag := r.ReadBit()
	control_extension_flag := r.ReadBit()
	_ = r.ReadBits(2) // reserved

	var payload_size int
	for {
		i := r.ReadByte()
		payload_size += int(i)
		if i < 255 {
			break
		}
	}

	if start_trim_flag != 0 {
		_ = r.ReadBits(3)
		_ = r.ReadBits(13)
	}
	if end_trim_flag != 0 {
		_ = r.ReadBits(3)
		_ = r.ReadBits(13)
	}
	if control_extension_flag != 0 {
		control_extension_length := r.ReadByte()
		_ = r.ReadBytes(int(control_extension_length)) // reserved
	}

	return payload_size
}
