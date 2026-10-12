package hardware

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
)

const (
	EngineSoftware     = "software"
	EngineVAAPI        = "vaapi"        // Intel iGPU and AMD GPU
	EngineV4L2M2M      = "v4l2m2m"      // Raspberry Pi 3 and 4
	EngineCUDA         = "cuda"         // NVidia on Windows and Linux
	EngineDXVA2        = "dxva2"        // Intel on Windows
	EngineVideoToolbox = "videotoolbox" // macOS
	EngineRKMPP        = "rkmpp"        // Rockchip
)

func Init(bin string) {
	api.HandleFunc("api/ffmpeg/hardware", func(w http.ResponseWriter, r *http.Request) {
		api.ResponseSources(w, ProbeAll(bin))
	})
}

// MakeHardware 将软件 FFmpeg 参数转换为硬件参数
// 引擎为空时自动选择
func MakeHardware(args *ffmpeg.Args, engine string, defaults map[string]string) {
	for i, codec := range args.Codecs {
		if len(codec) < 10 {
			continue // skip short line (-c:v mjpeg...)
		}

		// 获取当前编码名称
		name := cut(codec, ' ', 1)
		switch name {
		case "libx264":
			name = "h264"
		case "libx265":
			name = "h265"
		case "mjpeg":
		default:
			continue // skip unsupported codec
		}

		if engine == "" {
			if engine = cache[name]; engine == "" {
				engine = probeHardware(args.Bin, name)
				cache[name] = engine
			}
		}

		switch engine {
		case EngineVAAPI:
			args.Codecs[i] = defaults[name+"/"+engine]

			if !args.HasFilters("drawtext=") {
				args.Input = "-hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch " + args.Input

				if name == "h264" {
					fixPixelFormat(args)
				}

				for i, filter := range args.Filters {
					if strings.HasPrefix(filter, "scale=") {
						args.Filters[i] = "scale_vaapi=" + filter[6:]
					}
					if strings.HasPrefix(filter, "transpose=") {
						if filter == "transpose=1,transpose=1" { // 180 degrees half-turn
							args.Filters[i] = "transpose_vaapi=4" // reversal
						} else {
							args.Filters[i] = "transpose_vaapi=" + filter[10:]
						}
					}
				}

				// 输入不支持硬件加速时修正，支持时保持原样
				// 在硬件缩放和旋转之前插入第一个滤镜
				args.InsertFilter("format=vaapi|nv12,hwupload")
			} else {
				// 为文字、缩放和旋转滤镜启用软件像素格式
				args.Input = "-hwaccel vaapi -hwaccel_output_format nv12 -hwaccel_flags allow_profile_mismatch " + args.Input

				args.AddFilter("hwupload")
			}

		case EngineCUDA:
			args.Codecs[i] = defaults[name+"/"+engine]

			// CUDA 不支持硬件旋转
			// https://github.com/AlexxIT/go2rtc/issues/389
			if !args.HasFilters("drawtext=", "transpose=") {
				args.Input = "-hwaccel cuda -hwaccel_output_format cuda " + args.Input

				for i, filter := range args.Filters {
					if strings.HasPrefix(filter, "scale=") {
						args.Filters[i] = "scale_cuda=" + filter[6:]
					}
				}
			} else {
				args.Input = "-hwaccel cuda -hwaccel_output_format nv12 " + args.Input

				args.AddFilter("hwupload")
			}

		case EngineDXVA2:
			args.Input = "-hwaccel dxva2 -hwaccel_output_format dxva2_vld " + args.Input
			args.Codecs[i] = defaults[name+"/"+engine]

			for i, filter := range args.Filters {
				if strings.HasPrefix(filter, "scale=") {
					args.Filters[i] = "scale_qsv=" + filter[6:]
				}
			}

			args.InsertFilter("hwmap=derive_device=qsv,format=qsv")

		case EngineVideoToolbox:
			hwFilters := true

			for i, filter := range args.Filters {
				switch {
				case strings.HasPrefix(filter, "scale="):
					args.Filters[i] = "scale_vt=" + filter[6:]
				case strings.HasPrefix(filter, "transpose="):
					switch filter {
					case "transpose=1":
						args.Filters[i] = "transpose_vt=clock"
					case "transpose=2":
						args.Filters[i] = "transpose_vt=cclock"
					case "transpose=1,transpose=1":
						args.Filters[i] = "transpose_vt=reversal"
					default:
						hwFilters = false
					}
				default:
					hwFilters = false
				}
			}

			if hwFilters {
				args.Input = "-hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld " + args.Input
			} else {
				// 普通软件滤镜无法直接处理 VideoToolbox 硬件帧，
				// 例如 drawtext，需要先显式转换像素格式。
				args.Input = "-hwaccel videotoolbox -hwaccel_output_format nv12 " + args.Input
			}
			args.Codecs[i] = defaults[name+"/"+engine]

		case EngineV4L2M2M:
			args.Codecs[i] = defaults[name+"/"+engine]

		case EngineRKMPP:
			args.Codecs[i] = defaults[name+"/"+engine]

			if !args.HasFilters("drawtext=") {
				args.Input = "-hwaccel rkmpp -hwaccel_output_format drm_prime -afbc rga " + args.Input

				for i, filter := range args.Filters {
					if strings.HasPrefix(filter, "scale=") {
						args.Filters[i] = "scale_rkrga=" + filter[6:] + ":force_original_aspect_ratio=0"
					}
					if strings.HasPrefix(filter, "transpose=") {
						if filter == "transpose=1,transpose=1" { // 180 degrees half-turn
							args.Filters[i] = "vpp_rkrga=transpose=4" // reversal
						} else {
							args.Filters[i] = "vpp_rkrga=transpose=" + filter[10:]
						}
					}
				}

				if len(args.Filters) > 0 {
					// 输入不支持硬件加速时修正，支持时保持原样
					// 在硬件缩放和旋转之前插入第一个滤镜
					args.InsertFilter("format=drm_prime|nv12,hwupload")
				}
			} else {
				// 为文字、缩放和旋转滤镜启用软件像素格式
				args.Input = "-hwaccel rkmpp -hwaccel_output_format nv12 -afbc rga " + args.Input

				args.AddFilter("hwupload")
			}
		}
	}
}

var cache = map[string]string{}
var probeHardware = ProbeHardware

func run(bin string, args string) bool {
	err := exec.Command(bin, strings.Split(args, " ")...).Run()
	return err == nil
}

func runToString(bin string, args string) string {
	if run(bin, args) {
		return "OK"
	} else {
		return "ERROR"
	}
}

func cut(s string, sep byte, pos int) string {
	for n := 0; n < pos; n++ {
		if i := strings.IndexByte(s, sep); i > 0 {
			s = s[i+1:]
		} else {
			return ""
		}
	}
	if i := strings.IndexByte(s, sep); i > 0 {
		return s[:i]
	}
	return s
}

// fixPixelFormat:
//   - good h264 pixel: yuv420p(tv, bt709) == yuv420p (mpeg/limited/tv)
//   - bad h264 pixel: yuvj420p(pc, bt709) == yuvj420p (jpeg/full/pc)
//   - 错误的 JPEG 像素格式：yuvj422p(pc, bt470bg)
func fixPixelFormat(args *ffmpeg.Args) {
	// 原作者测试中，以下滤镜具有相同 CPU/GPU 负载：
	//   - "hwupload"
	//   - "hwupload,scale_vaapi=out_color_matrix=bt709:out_range=tv"
	//   - "hwupload,scale_vaapi=out_color_matrix=bt709:out_range=tv:format=nv12"
	const fixPixFmt = "out_color_matrix=bt709:out_range=tv:format=nv12"

	for i, filter := range args.Filters {
		if strings.HasPrefix(filter, "scale=") {
			args.Filters[i] = filter + ":" + fixPixFmt
			return
		}
	}

	args.Filters = append(args.Filters, "scale="+fixPixFmt)
}
