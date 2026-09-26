//go:build darwin || linux

package export

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openIdentityNoFollow(root *os.Root, name string) (*os.File, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return nil, invalidDestination()
	}

	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()

	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(fd), name), nil
}
