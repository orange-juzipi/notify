//go:build windows

package util

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile 对文件加排他锁（Windows 平台）
func (fl *FileLock) lockFile(file *os.File, nonBlocking bool) error {
	// Windows 使用 LockFileEx API
	handle := windows.Handle(file.Fd())

	// 定义锁的标志
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonBlocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}

	// 锁定整个文件（从偏移 0 开始，长度为 0xFFFFFFFF）
	var ol windows.Overlapped
	err := windows.LockFileEx(handle, flags, 0, 1, 0, &ol)
	if err != nil {
		if err == windows.ERROR_LOCK_VIOLATION && nonBlocking {
			return errWouldBlock
		}
		return fmt.Errorf("加锁失败: %v", err)
	}

	return nil
}

// unlockFile 释放文件锁（Windows 平台）
func (fl *FileLock) unlockFile(file *os.File) error {
	handle := windows.Handle(file.Fd())

	var ol windows.Overlapped
	err := windows.UnlockFileEx(handle, 0, 1, 0, &ol)
	if err != nil {
		return fmt.Errorf("释放锁失败: %v", err)
	}

	return nil
}
