package mjpeg

import (
	"bytes"
	"image/jpeg"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
	"github.com/pion/rtp"
)

func FixJPEG(b []byte) []byte {
	// 跳过非 JPEG 数据
	if len(b) < 10 || b[0] != 0xFF || b[1] != markerSOI {
		return b
	}

	// 跳过没有应用标记的 JPEG
	if b[2] == 0xFF && b[3] == markerDQT {
		return b
	}

	switch string(b[6:10]) {
	case "JFIF", "Exif":
		// 图像头可被 imghdr 识别时跳过修正
		// - https://docs.python.org/3/library/imghdr.html
		return b
	case "AVI1":
		// 在 JPEG 的 SOS 标记前添加 DHT 表
		// 适用于从 MJPEG 流保存 JPEG 帧
		// - https://github.com/image-rs/jpeg-decoder/issues/76
		// - https://github.com/pion/mediadevices/pull/493
		// - https://bugzilla.mozilla.org/show_bug.cgi?id=963907#c18
		return InjectDHT(b)
	}

	// 图像头错误时重新编码 JPEG
	//
	// 例如此应用产生不兼容图像，
	// https://github.com/jacksonliam/mjpg-streamer
	//
	// 无法上传到 Telegram 服务器：
	// {"ok":false,"error_code":400,"description":"Bad Request: IMAGE_PROCESS_FAILED"}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, img, nil); err != nil {
		return b
	}
	return buf.Bytes()
}

// Encoder 将 YUV 帧转换为图像。
// 支持跳过空帧，例如 USB 摄像头尚未完成启动时。
func Encoder(codec *core.Codec, skipEmpty int, handler core.HandlerFunc) core.HandlerFunc {
	newImage := y4m.NewImage(codec.FmtpLine)

	return func(packet *rtp.Packet) {
		img := newImage(packet.Payload)

		if skipEmpty != 0 && y4m.HasSameColor(img) {
			skipEmpty--
			return
		}

		buf := bytes.NewBuffer(nil)
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return
		}

		clone := *packet
		clone.Payload = buf.Bytes()
		handler(&clone)
	}
}

const dhtSize = 432 // known size for 4 default tables

func InjectDHT(b []byte) []byte {
	if bytes.Index(b, []byte{0xFF, markerDHT}) > 0 {
		return b // already exist
	}

	i := bytes.Index(b, []byte{0xFF, markerSOS})
	if i < 0 {
		return b
	}

	dht := make([]byte, 0, dhtSize)
	dht = MakeHuffmanHeaders(dht)

	tmp := make([]byte, len(b)+dhtSize)
	copy(tmp, b[:i])
	copy(tmp[i:], dht)
	copy(tmp[i+dhtSize:], b[i:])

	return tmp
}
