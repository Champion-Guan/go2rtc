package mp4

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// ParseQuery 类似常规解析，但额外处理 mp4 参数
func ParseQuery(query map[string][]string) []*core.Media {
	if v := query["mp4"]; len(v) != 0 {
		medias := []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecH264},
					{Name: core.CodecH265},
				},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecAAC},
				},
			},
		}

		if v[0] == "" {
			return medias // legacy
		}

		medias[1].Codecs = append(medias[1].Codecs,
			&core.Codec{Name: core.CodecPCMA},
			&core.Codec{Name: core.CodecPCMU},
			&core.Codec{Name: core.CodecPCM},
			&core.Codec{Name: core.CodecPCML},
		)

		if v[0] == "flac" {
			return medias // modern browsers
		}

		medias[1].Codecs = append(medias[1].Codecs,
			&core.Codec{Name: core.CodecOpus},
			&core.Codec{Name: core.CodecMP3},
		)

		return medias // Chrome, FFmpeg, VLC
	}

	return core.ParseQuery(query)
}

func ParseCodecs(codecs string, parseAudio bool) (medias []*core.Media) {
	var videos []*core.Codec
	var audios []*core.Codec

	for _, name := range strings.Split(codecs, ",") {
		switch name {
		case MimeH264:
			codec := &core.Codec{Name: core.CodecH264}
			videos = append(videos, codec)
		case MimeH265:
			codec := &core.Codec{Name: core.CodecH265}
			videos = append(videos, codec)
		case MimeAAC:
			codec := &core.Codec{Name: core.CodecAAC}
			audios = append(audios, codec)
		case MimeFlac:
			audios = append(audios,
				&core.Codec{Name: core.CodecPCMA},
				&core.Codec{Name: core.CodecPCMU},
				&core.Codec{Name: core.CodecPCM},
				&core.Codec{Name: core.CodecPCML},
			)
		case MimeOpus:
			codec := &core.Codec{Name: core.CodecOpus}
			audios = append(audios, codec)
		}
	}

	if videos != nil {
		media := &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs:    videos,
		}
		medias = append(medias, media)
	}

	if audios != nil && parseAudio {
		media := &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    audios,
		}
		medias = append(medias, media)
	}

	return
}

// PatchVideoRotate 更新视频轨道变换矩阵。
// 多数播放器和浏览器支持旋转，Safari 除外。
// 缩放兼容性有限，建议谨慎使用。
// 仅支持 0、90、180、270 度。
func PatchVideoRotate(init []byte, degrees int) bool {
	// 查找视频原子
	i := bytes.Index(init, []byte("vide"))
	if i < 0 {
		return false
	}

	// 定位视频矩阵位置
	i -= 4 + 3 + 1 + 8 + 32 + 8 + 4 + 4 + 4*9

	// 旋转矩阵：
	// [   cos   sin     0]
	// [  -sin   cos     0]
	// [     0     0 16384]
	var cos, sin uint16

	switch degrees {
	case 0:
		cos = 1
		sin = 0
	case 90:
		cos = 0
		sin = 1
	case 180:
		cos = 0xFFFF // -1
		sin = 0
	case 270:
		cos = 0
		sin = 0xFFFF // -1
	default:
		return false
	}

	binary.BigEndian.PutUint16(init[i:], cos)
	binary.BigEndian.PutUint16(init[i+4:], sin)
	binary.BigEndian.PutUint16(init[i+12:], -sin)
	binary.BigEndian.PutUint16(init[i+16:], cos)

	return true
}

// PatchVideoScale 更新像素宽高比原子。
// 多数播放器和浏览器支持，Firefox 除外。
// 仅支持正整数。
func PatchVideoScale(init []byte, scaleX, scaleY int) bool {
	// 查找视频原子
	i := bytes.Index(init, []byte("pasp"))
	if i < 0 {
		return false
	}

	binary.BigEndian.PutUint32(init[i+4:], uint32(scaleX))
	binary.BigEndian.PutUint32(init[i+8:], uint32(scaleY))

	return true
}
