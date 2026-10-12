//go:build !(linux && (386 || amd64 || arm || arm64 || mipsle))

package alsa

func Init() {
	// 不支持
}
