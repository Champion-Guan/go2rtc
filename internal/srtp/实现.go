package srtp

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen string `yaml:"listen"`
		} `yaml:"srtp"`
	}

	// 默认配置
	cfg.Mod.Listen = ":8443"

	// 从 YAML 加载配置
	app.LoadConfig(&cfg)

	if cfg.Mod.Listen == "" {
		return
	}

	// 创建 SRTP 端点，接收 HomeKit 摄像头视频
	Server = srtp.NewServer(cfg.Mod.Listen)
}

var Server *srtp.Server
