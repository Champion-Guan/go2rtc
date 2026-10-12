package app

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
)

var (
	Version    string
	Modules    []string
	UserAgent  string
	ConfigPath string
	Info       = make(map[string]any)
)

const usage = `go2rtc 使用方法：

  -c, --config   配置文件路径或 YAML/JSON 配置字符串，可重复指定
  -d, --daemon   在后台运行
  -v, --version  输出版本并退出
`

func Init() {
	var config flagConfig
	var daemon bool
	var version bool

	flag.Var(&config, "config", "")
	flag.Var(&config, "c", "")
	flag.BoolVar(&daemon, "daemon", false, "")
	flag.BoolVar(&daemon, "d", false, "")
	flag.BoolVar(&version, "version", false, "")
	flag.BoolVar(&version, "v", false, "")

	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	revision, vcsTime := readRevisionTime()

	if version {
		fmt.Printf("go2rtc 版本 %s (%s) %s/%s\n", Version, revision, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if daemon && os.Getppid() != 1 {
		if runtime.GOOS == "windows" {
			fmt.Println("Windows 不支持后台进程模式")
			os.Exit(1)
		}

		// 在后台重新运行程序并退出当前进程
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		if err := cmd.Start(); err != nil {
			fmt.Println("启动后台进程失败：", err)
			os.Exit(1)
		}
		fmt.Println("已在后台运行，进程标识：", cmd.Process.Pid)
		os.Exit(0)
	}

	UserAgent = "go2rtc/" + Version

	Info["version"] = Version
	Info["revision"] = revision

	initConfig(config)
	initLogger()

	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	Logger.Info().Str("version", Version).Str("platform", platform).Str("revision", revision).Msg("go2rtc")
	Logger.Debug().Str("version", runtime.Version()).Str("vcs.time", vcsTime).Msg("build")

	if ConfigPath != "" {
		Logger.Info().Str("path", ConfigPath).Msg("config")
	}

	var cfg struct {
		Mod struct {
			Modules []string `yaml:"modules"`
		} `yaml:"app"`
	}

	LoadConfig(&cfg)

	Modules = cfg.Mod.Modules
}

func readRevisionTime() (revision, vcsTime string) {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if len(setting.Value) > 7 {
					revision = setting.Value[:7]
				} else {
					revision = setting.Value
				}
			case "vcs.time":
				vcsTime = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					revision += ".dirty"
				}
			}
		}

		// 从 -buildvcs 信息读取版本
		// 带标签版本格式：v1.9.13
		// 修改后代码的版本格式：v1.9.14-0.20251215184105-753d6617ab58+dirty
		if info.Main.Version != "v"+Version {
			// Format: 1.9.13+dev.753d661[.dirty]
			// 兼容 Python 的 awesomeversion 和 packaging.version。
			// 版本应大于上一个发行版，小于下一个发行版。
			Version += "+dev." + revision
		}
	}
	return
}
