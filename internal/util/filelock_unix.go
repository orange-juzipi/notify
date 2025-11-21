//go:build unix

package util

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// lockFile 对文件加排他锁（Unix 平台）
func (fl *FileLock) lockFile(file *os.File, nonBlocking bool) error {
	flags := unix.LOCK_EX
	if nonBlocking {
		flags |= unix.LOCK_NB
	}

	err := unix.Flock(int(file.Fd()), flags)
	if err != nil {
		if err == unix.EWOULDBLOCK && nonBlocking {
			return errWouldBlock
		}
		return fmt.Errorf("加锁失败: %v", err)
	}

	return nil
}

// unlockFile 释放文件锁（Unix 平台）
func (fl *FileLock) unlockFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	if err != nil {
		return fmt.Errorf("释放锁失败: %v", err)
	}
	return nil
}
