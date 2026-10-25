//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package protection

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockState(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
