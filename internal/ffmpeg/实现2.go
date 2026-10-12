package ffmpeg

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/device"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/hardware"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/virtual"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]string `yaml:"ffmpeg"`
		Log struct {
			Level string `yaml:"ffmpeg"`
		} `yaml:"log"`
	}

	cfg.Mod = defaults // will be overriden from yaml
	cfg.Log.Level = "error"

	app.LoadConfig(&cfg)

	log = app.GetLogger("ffmpeg")

	// zerolog 日志级别：trace debug info warn error fatal panic disabled
	// FFmpeg 日志级别：trace debug verbose info warning error fatal panic quiet
	if cfg.Log.Level == "warn" {
		cfg.Log.Level = "warning"
	}
	defaults["global"] += " -v " + cfg.Log.Level

	streams.RedirectFunc("ffmpeg", func(url string) (string, error) {
		if _, err := Version(); err != nil {
			return "", err
		}
		args := parseArgs(url[7:])
		if core.Contains(args.Codecs, "auto") {
			return "", nil // force call streams.HandleFunc("ffmpeg")
		}
		return "exec:" + args.String(), nil
	})

	streams.HandleFunc("ffmpeg", NewProducer)

	api.HandleFunc("api/ffmpeg", apiFFmpeg)

	device.Init(defaults["bin"])
	hardware.Init(defaults["bin"])
}

var defaults = map[string]string{
	"bin":     "ffmpeg",
	"global":  "-hide_banner",
	"timeout": "5",

	// inputs
	"file":     "-re -i {input}",
	"http":     "-fflags nobuffer -flags low_delay -i {input}",
	"rtsp":     "-fflags nobuffer -flags low_delay -timeout {timeout} -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i {input}",
	"rtsp/udp": "-fflags nobuffer -flags low_delay -timeout {timeout} -user_agent go2rtc/ffmpeg -i {input}",

	// output
	"output":       "-user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}",
	"output/mjpeg": "-f mjpeg -",
	"output/raw":   "-f yuv4mpegpipe -",
	"output/aac":   "-f adts -",
	"output/wav":   "-f wav -",

	// `-preset superfast`：不能使用 ultrafast，因为它不支持 `-profile main -level 4.1`
	// `-tune zerolatency`：尽量降低延迟
	// `-profile high -level 4.1`：常用的流媒体编码配置
	// `-pix_fmt:v yuv420p`：Telegram 所需的像素格式
	"h264":  "-c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p",
	"h265":  "-c:v libx265 -g 50 -profile:v main -x265-params level=5.1:high-tier=0 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p",
	"mjpeg": "-c:v mjpeg",
	//"mjpeg": "-c:v mjpeg -force_duplicated_matrix:v 1 -huffman:v 0 -pix_fmt:v yuvj420p",

	"raw":         "-c:v rawvideo",
	"raw/gray8":   "-c:v rawvideo -pix_fmt:v gray8",
	"raw/yuv420p": "-c:v rawvideo -pix_fmt:v yuv420p",
	"raw/yuv422p": "-c:v rawvideo -pix_fmt:v yuv422p",
	"raw/yuv444p": "-c:v rawvideo -pix_fmt:v yuv444p",

	// https://ffmpeg.org/ffmpeg-codecs.html#libopus-1
	// https://github.com/pion/webrtc/issues/1514
	// https://ffmpeg.org/ffmpeg-resampler.html
	// `-async 1` 或 `-min_comp 0`：强制重采样以保持时间戳递增，改善 WebRTC 音质
	"opus":       "-c:a libopus -application:a lowdelay -min_comp 0",
	"opus/16000": "-c:a libopus -application:a lowdelay -min_comp 0 -ar:a 16000 -ac:a 1",
	"pcmu":       "-c:a pcm_mulaw -ar:a 8000 -ac:a 1",
	"pcmu/8000":  "-c:a pcm_mulaw -ar:a 8000 -ac:a 1",
	"pcmu/16000": "-c:a pcm_mulaw -ar:a 16000 -ac:a 1",
	"pcmu/48000": "-c:a pcm_mulaw -ar:a 48000 -ac:a 1",
	"pcma":       "-c:a pcm_alaw -ar:a 8000 -ac:a 1",
	"pcma/8000":  "-c:a pcm_alaw -ar:a 8000 -ac:a 1",
	"pcma/16000": "-c:a pcm_alaw -ar:a 16000 -ac:a 1",
	"pcma/48000": "-c:a pcm_alaw -ar:a 48000 -ac:a 1",
	"aac":        "-c:a aac", // keep sample rate and channels
	"aac/16000":  "-c:a aac -ar:a 16000 -ac:a 1",
	"mp3":        "-c:a libmp3lame -q:a 8",
	"pcm":        "-c:a pcm_s16be -ar:a 8000 -ac:a 1",
	"pcm/8000":   "-c:a pcm_s16be -ar:a 8000 -ac:a 1",
	"pcm/16000":  "-c:a pcm_s16be -ar:a 16000 -ac:a 1",
	"pcm/48000":  "-c:a pcm_s16be -ar:a 48000 -ac:a 1",
	"pcml":       "-c:a pcm_s16le -ar:a 8000 -ac:a 1",
	"pcml/8000":  "-c:a pcm_s16le -ar:a 8000 -ac:a 1",
	"pcml/16000": "-c:a pcm_s16le -ar:a 16000 -ac:a 1",
	"pcml/44100": "-c:a pcm_s16le -ar:a 44100 -ac:a 1",

	// Linux 上的 Intel 和 AMD 硬件加速
	// 不建议像 QSV 一样设置 `-async_depth:v 1`，否则可能掉帧
	// `-bf 0`：务必禁用 B 帧
	"h264/vaapi":  "-c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0",
	"h265/vaapi":  "-c:v hevc_vaapi -g 50 -bf 0 -profile:v main -level:v 5.1 -sei:v 0",
	"mjpeg/vaapi": "-c:v mjpeg_vaapi",

	// 树莓派硬件加速
	"h264/v4l2m2m": "-c:v h264_v4l2m2m -g 50 -bf 0",
	"h265/v4l2m2m": "-c:v hevc_v4l2m2m -g 50 -bf 0",

	// 瑞芯微硬件加速
	// 必须使用自定义 FFmpeg：https://github.com/AlexxIT/go2rtc/issues/768
	// HEVC 不包含 profile 设置
	"h264/rkmpp":  "-c:v h264_rkmpp -g 50 -bf 0 -profile:v high -level:v 4.1",
	"h265/rkmpp":  "-c:v hevc_rkmpp -g 50 -bf 0 -profile:v main -level:v 5.1",
	"mjpeg/rkmpp": "-c:v mjpeg_rkmpp",

	// Linux 和 Windows 上的 NVIDIA 硬件加速
	// preset=p2 表示更快，tune=ll 表示低延迟
	"h264/cuda": "-c:v h264_nvenc -g 50 -bf 0 -profile:v high -level:v auto -preset:v p2 -tune:v ll",
	"h265/cuda": "-c:v hevc_nvenc -g 50 -bf 0 -profile:v main -level:v auto",

	// Windows 上的 Intel 硬件加速
	"h264/dxva2":  "-c:v h264_qsv -g 50 -bf 0 -profile:v high -level:v 4.1 -async_depth:v 1",
	"h265/dxva2":  "-c:v hevc_qsv -g 50 -bf 0 -profile:v main -level:v 5.1 -async_depth:v 1",
	"mjpeg/dxva2": "-c:v mjpeg_qsv",

	// macOS 硬件加速
	"h264/videotoolbox": "-c:v h264_videotoolbox -g 50 -bf 0 -profile:v high",
	"h265/videotoolbox": "-c:v hevc_videotoolbox -g 50 -bf 0 -profile:v main -level:v 5.1",
}

var log zerolog.Logger

// configTemplate 返回配置中的模板；不存在时返回原始模板
func configTemplate(template string) string {
	if s := defaults[template]; s != "" {
		return s
	}
	return template
}

// inputTemplate 按名称选择 YAML 中的输入模板
// 查询包含 input 参数时，按该名称选择其他模板
// 没有对应模板时，直接使用 input 参数作为模板
func inputTemplate(name, s string, query url.Values) string {
	var template string
	if input := query.Get("input"); input != "" {
		template = configTemplate(input)
	} else {
		template = defaults[name]
	}
	if strings.Contains(template, "{timeout}") {
		timeout := query.Get("timeout")
		if timeout == "" {
			timeout = defaults["timeout"]
		}
		template = strings.Replace(template, "{timeout}", timeout+"000000", 1)
	}
	return strings.Replace(template, "{input}", s, 1)
}

func parseArgs(s string) *ffmpeg.Args {
	// 初始化 FFmpeg 参数
	args := &ffmpeg.Args{
		Bin:     defaults["bin"],
		Global:  defaults["global"],
		Output:  defaults["output"],
		Version: verAV,
	}

	var source = s
	var query url.Values
	if i := strings.IndexByte(s, '#'); i >= 0 {
		query = streams.ParseQuery(s[i+1:])
		args.Video = len(query["video"])
		args.Audio = len(query["audio"])
		s = s[:i]
	}

	// 解析输入：
	//   1. 输入 xxxx:// 链接（HTTP、RTSP 或其他协议）
	//   2. 输入流名称
	//   3. 输入 FFmpeg 设备（本地 USB 摄像头）
	if i := strings.Index(s, "://"); i > 0 {
		switch s[:i] {
		case "http", "https", "rtmp":
			args.Input = inputTemplate("http", s, query)
		case "rtsp", "rtsps":
			// https://ffmpeg.org/ffmpeg-protocols.html#rtsp
			// 跳过不需要的输入轨道
			switch {
			case (args.Video > 0 && args.Audio > 0) || (args.Video == 0 && args.Audio == 0):
				args.Input = "-allowed_media_types video+audio "
			case args.Video > 0:
				args.Input = "-allowed_media_types video "
			case args.Audio > 0:
				args.Input = "-allowed_media_types audio "
			}

			args.Input += inputTemplate("rtsp", s, query)
		default:
			args.Input = "-i " + s
		}
	} else if streams.Get(s) != nil {
		s = "rtsp://127.0.0.1:" + rtsp.Port + "/" + s
		switch {
		case args.Video > 0 && args.Audio == 0:
			s += "?video"
		case args.Audio > 0 && args.Video == 0:
			s += "?audio"
		default:
			s += "?video&audio"
		}
		s += "&source=ffmpeg:" + url.QueryEscape(source)
		for _, v := range query["query"] {
			s += "&" + v
		}
		args.Input = inputTemplate("rtsp", s, query)
	} else if i = strings.Index(s, "?"); i > 0 {
		switch s[:i] {
		case "device":
			args.Input = device.GetInput(s[i+1:])
		case "virtual":
			args.Input = virtual.GetInput(s[i+1:])
		case "tts":
			args.Input = virtual.GetInputTTS(s[i+1:])
		}
	} else {
		args.Input = inputTemplate("file", s, query)
	}

	if query["async"] != nil {
		args.Input = "-use_wallclock_as_timestamps 1 -async 1 " + args.Input
	}

	// 解析查询参数：
	//   1. `width`/`height` params
	//   2. `rotate` param
	//   3. `video` 参数（支持多个）
	//   4. `audio` 参数（支持多个）
	//   5. `hardware` param
	if query != nil {
		// 1. 处理 FFmpeg 原始参数
		for _, raw := range query["raw"] {
			// 支持模板：https://github.com/AlexxIT/go2rtc/issues/487
			raw = configTemplate(raw)
			args.AddCodec(raw)
		}

		// 2. 处理视频滤镜（缩放和旋转）
		if query["width"] != nil || query["height"] != nil {
			filter := "scale="
			if query["width"] != nil {
				filter += query["width"][0]
			} else {
				filter += "-1"
			}
			filter += ":"
			if query["height"] != nil {
				filter += query["height"][0]
			} else {
				filter += "-1"
			}
			args.AddFilter(filter)
		}

		if query["rotate"] != nil {
			var filter string
			switch query["rotate"][0] {
			case "90":
				filter = "transpose=1" // 90 degrees clockwise
			case "180":
				filter = "transpose=1,transpose=1"
			case "-90", "270":
				filter = "transpose=2" // 90 degrees counterclockwise
			}
			if filter != "" {
				args.AddFilter(filter)
			}
		}

		for _, drawtext := range query["drawtext"] {
			// 支持模板：https://github.com/AlexxIT/go2rtc/issues/487
			drawtext = configTemplate(drawtext)

			// 支持默认时间戳格式
			if !strings.Contains(drawtext, "text=") {
				drawtext += `:text='%{localtime\:%Y-%m-%d %X}'`
			}

			args.AddFilter("drawtext=" + drawtext)
		}

		// 3. 处理视频编码
		if args.Video > 0 {
			for _, video := range query["video"] {
				if video != "copy" {
					if codec := defaults[video]; codec != "" {
						args.AddCodec(codec)
					} else {
						args.AddCodec(video)
					}
				} else {
					args.AddCodec("-c:v copy")
				}
			}
		}

		if query["bitrate"] != nil {
			// https://trac.ffmpeg.org/wiki/Limiting%20the%20output%20bitrate
			b := query["bitrate"][0]
			args.AddCodec("-b:v " + b + " -maxrate " + b + " -bufsize " + b)
		}

		// 4. 处理音频编码
		if args.Audio > 0 {
			for _, audio := range query["audio"] {
				if audio != "copy" {
					if codec := defaults[audio]; codec != "" {
						args.AddCodec(codec)
					} else {
						args.AddCodec(audio)
					}
				} else {
					args.AddCodec("-c:a copy")
				}
			}
		}

		if query["hardware"] != nil {
			hardware.MakeHardware(args, query["hardware"][0], defaults)
		}
	}

	switch {
	case args.Video == 0 && args.Audio == 0:
		args.AddCodec("-c copy")
	case args.Video == 0:
		args.AddCodec("-vn")
	case args.Audio == 0:
		args.AddCodec("-an")
	}

	// 将输出从 RTSP 改为其他管道格式
	switch {
	case args.Video == 0 && args.Audio == 0:
		// MJPEG 输入无需转码，适用于能输出原始 MJPEG 的 FFmpeg 设备
		if strings.Contains(args.Input, " mjpeg ") {
			args.Output = defaults["output/mjpeg"]
		}
	case args.Video == 1 && args.Audio == 0:
		switch core.Before(query.Get("video"), "/") {
		case "mjpeg":
			args.Output = defaults["output/mjpeg"]
		case "raw":
			args.Output = defaults["output/raw"]
		}
	case args.Video == 0 && args.Audio == 1:
		switch core.Before(query.Get("audio"), "/") {
		case "aac":
			args.Output = defaults["output/aac"]
		case "pcma", "pcmu", "pcml":
			args.Output = defaults["output/wav"]
		}
	}

	return args
}
