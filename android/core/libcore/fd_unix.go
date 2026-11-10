//go:build unix

package libcore

import "syscall"

// dupFd gives mihomo its own copy of the tun fd; the app keeps (and closes) the original.
func dupFd(fd int) (int, error) { return syscall.Dup(fd) }

func closeFd(fd int) { _ = syscall.Close(fd) }
