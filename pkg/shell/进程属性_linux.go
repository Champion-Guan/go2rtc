package shell

import "syscall"

// 父进程退出时停止子进程，包括 SIGKILL
var procAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
