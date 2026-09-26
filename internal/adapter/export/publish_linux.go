//go:build linux

package export

import (
	"errors"
	"os"
	"syscall"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"golang.org/x/sys/unix"
)

func fileID(info os.FileInfo) (uint64, uint64) {
	stat := info.Sys().(*syscall.Stat_t)
	return uint64(stat.Dev), stat.Ino
}

func exchangeChecked(root *os.Root, staging, target string, want application.OverwriteIdentity) error {
	current, err := rootIdentity(root, target)
	if err != nil || !sameIdentity(current, &want) {
		return &shared.Error{Code: "export_source_changed", Message: "上書き対象が変更されました", Retryable: true}
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	fd := int(directory.Fd())
	if err := unix.Renameat2(fd, staging, fd, target, unix.RENAME_EXCHANGE); err != nil {
		return err
	}
	old, err := rootIdentity(root, staging)
	if err != nil || !sameIdentity(old, &want) {
		swapBackErr := unix.Renameat2(fd, staging, fd, target, unix.RENAME_EXCHANGE)
		return errors.Join(
			&shared.Error{Code: "export_source_changed", Message: "上書き対象が変更されました", Retryable: true},
			swapBackErr,
		)
	}
	return nil
}
