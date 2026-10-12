package main

import (
	"slices"

	"github.com/AlexxIT/go2rtc/internal/alsa"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/bubble"
	"github.com/AlexxIT/go2rtc/internal/debug"
	"github.com/AlexxIT/go2rtc/internal/doorbird"
	"github.com/AlexxIT/go2rtc/internal/dvrip"
	"github.com/AlexxIT/go2rtc/internal/echo"
	"github.com/AlexxIT/go2rtc/internal/eseecloud"
	"github.com/AlexxIT/go2rtc/internal/exec"
	"github.com/AlexxIT/go2rtc/internal/expr"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/flussonic"
	"github.com/AlexxIT/go2rtc/internal/gopro"
	"github.com/AlexxIT/go2rtc/internal/hass"
	"github.com/AlexxIT/go2rtc/internal/hls"
	"github.com/AlexxIT/go2rtc/internal/homekit"
	"github.com/AlexxIT/go2rtc/internal/http"
	"github.com/AlexxIT/go2rtc/internal/isapi"
	"github.com/AlexxIT/go2rtc/internal/ivideon"
	"github.com/AlexxIT/go2rtc/internal/kasa"
	"github.com/AlexxIT/go2rtc/internal/mjpeg"
	"github.com/AlexxIT/go2rtc/internal/mp4"
	"github.com/AlexxIT/go2rtc/internal/mpeg"
	"github.com/AlexxIT/go2rtc/internal/multitrans"
	"github.com/AlexxIT/go2rtc/internal/nest"
	"github.com/AlexxIT/go2rtc/internal/ngrok"
	"github.com/AlexxIT/go2rtc/internal/onvif"
	"github.com/AlexxIT/go2rtc/internal/pinggy"
	"github.com/AlexxIT/go2rtc/internal/ring"
	"github.com/AlexxIT/go2rtc/internal/roborock"
	"github.com/AlexxIT/go2rtc/internal/rtmp"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/tapo"
	"github.com/AlexxIT/go2rtc/internal/tuya"
	"github.com/AlexxIT/go2rtc/internal/v4l2"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/internal/webtorrent"
	"github.com/AlexxIT/go2rtc/internal/wyoming"
	"github.com/AlexxIT/go2rtc/internal/wyze"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
	"github.com/AlexxIT/go2rtc/internal/yandex"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func main() {
	// 版本将稍后从 -buildvcs 信息读取，此处仅作为回退值
	app.Version = "1.9.14"

	type module struct {
		name string
		init func()
	}

	modules := []module{
		{"", app.Init},    // init config and logs
		{"api", api.Init}, // init API before all others
		{"ws", ws.Init},   // init WS API endpoint
		{"", streams.Init},
		// 主要输入源和服务器
		{"http", http.Init},     // rtsp source, HTTP server
		{"rtsp", rtsp.Init},     // rtsp source, RTSP server
		{"webrtc", webrtc.Init}, // webrtc source, WebRTC server
		// 主要接口
		{"mp4", mp4.Init},     // MP4 API
		{"hls", hls.Init},     // HLS API
		{"mjpeg", mjpeg.Init}, // MJPEG API
		// 其他输入源和服务器
		{"hass", hass.Init},             // hass source, Hass API server
		{"homekit", homekit.Init},       // homekit source, HomeKit server
		{"onvif", onvif.Init},           // onvif source, ONVIF API server
		{"rtmp", rtmp.Init},             // rtmp source, RTMP server
		{"webtorrent", webtorrent.Init}, // webtorrent source, WebTorrent module
		{"wyoming", wyoming.Init},
		// 外部命令和脚本源
		{"echo", echo.Init},
		{"exec", exec.Init},
		{"expr", expr.Init},
		{"ffmpeg", ffmpeg.Init},
		// 硬件输入源
		{"alsa", alsa.Init},
		{"v4l2", v4l2.Init},
		// 其他输入源
		{"bubble", bubble.Init},
		{"doorbird", doorbird.Init},
		{"dvrip", dvrip.Init},
		{"eseecloud", eseecloud.Init},
		{"flussonic", flussonic.Init},
		{"gopro", gopro.Init},
		{"isapi", isapi.Init},
		{"ivideon", ivideon.Init},
		{"kasa", kasa.Init},
		{"mpegts", mpeg.Init},
		{"multitrans", multitrans.Init},
		{"nest", nest.Init},
		{"ring", ring.Init},
		{"roborock", roborock.Init},
		{"tapo", tapo.Init},
		{"tuya", tuya.Init},
		{"wyze", wyze.Init},
		{"xiaomi", xiaomi.Init},
		{"yandex", yandex.Init},
		// 辅助模块
		{"debug", debug.Init},
		{"ngrok", ngrok.Init},
		{"pinggy", pinggy.Init},
		{"srtp", srtp.Init},
	}

	for _, m := range modules {
		if app.Modules == nil || m.name == "" || slices.Contains(app.Modules, m.name) {
			m.init()
		}
	}

	shell.RunUntilSignal()
}
