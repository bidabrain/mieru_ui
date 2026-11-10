//go:build !unix

package libcore

func dupFd(fd int) (int, error) { return fd, nil }

func closeFd(int) {}
