package xiaomi

import (
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/legacy"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

func Dial(rawURL string) (core.Producer, error) {
	// 格式：xiaomi/miss
	if strings.Contains(rawURL, "vendor") {
		return miss.Dial(rawURL)
	}

	// 格式：xiaomi/legacy
	return legacy.Dial(rawURL)
}

func IsLegacy(model string) bool {
	return legacy.Supported(model)
}
