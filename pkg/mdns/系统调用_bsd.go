//go:build darwin || ios || freebsd || openbsd || netbsd || dragonfly

package mdns

import (
	"syscall"
)

func SetsockoptInt(fd uintptr, level, opt int, value int) (err error) {
	// 为 BSD 类系统同时修改 SO_REUSEADDR 和 REUSEPORT 标志
	// https://github.com/AlexxIT/go2rtc/issues/626
	// https://stackoverflow.com/questions/14388706/how-do-so-reuseaddr-and-so-reuseport-differ/14388707
	if opt == syscall.SO_REUSEADDR {
		if err = syscall.SetsockoptInt(int(fd), level, opt, value); err != nil {
			return
		}

		opt = syscall.SO_REUSEPORT
	}

	return syscall.SetsockoptInt(int(fd), level, opt, value)
}

func SetsockoptIPMreq(fd uintptr, level, opt int, mreq *syscall.IPMreq) (err error) {
	return syscall.SetsockoptIPMreq(int(fd), level, opt, mreq)
}
