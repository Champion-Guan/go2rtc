package www

import (
	"embed"
	"io/fs"
)

//go:embed *.html
//go:embed *.js
//go:embed *.json
var embedded embed.FS

// 资源文件使用中文名称，旧地址继续可用，兼容书签及第三方播放器。
var aliases = map[string]string{
	"add.html":         "添加设备.html",
	"config.html":      "配置编辑.html",
	"links.html":       "播放链接.html",
	"log.html":         "日志查看.html",
	"net.html":         "连接拓扑.html",
	"stream.html":      "视频预览.html",
	"webrtc.html":      "WebRTC预览.html",
	"webrtc-sync.html": "WebRTC同步预览.html",
	"hls.html":         "HLS预览.html",
	"main.js":          "公共界面.js",
	"video-rtc.js":     "实时播放器.js",
	"video-stream.js":  "视频流播放器.js",
	"schema.json":      "配置规范.json",
}

type staticFiles struct{ embed.FS }

func (s staticFiles) Open(name string) (fs.File, error) {
	if translated, ok := aliases[name]; ok {
		name = translated
	}
	return s.FS.Open(name)
}

func (s staticFiles) ReadFile(name string) ([]byte, error) {
	if translated, ok := aliases[name]; ok {
		name = translated
	}
	return s.FS.ReadFile(name)
}

var Static = staticFiles{embedded}
