package util

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileLock 文件锁，用于防止多个进程同时运行
type FileLock struct {
	file *os.File
	path string
}

// NewFileLock 创建文件锁
func NewFileLock(lockPath string) (*FileLock, error) {
	if lockPath == "" {
		// 使用默认路径
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取用户主目录失败: %v", err)
		}
		lockPath = filepath.Join(home, ".notify", "notify.lock")
	}

	// 确保目录存在
	lockDir := filepath.Dir(lockPath)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("创建锁文件目录失败: %v", err)
	}

	return &FileLock{
		path: lockPath,
	}, nil
}

// Lock 获取文件锁（阻塞式）
func (fl *FileLock) Lock() error {
	file, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("打开锁文件失败: %v", err)
	}

	// 尝试获取排他锁
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		return fmt.Errorf("已有其他实例正在运行（无法获取文件锁）")
	}

	fl.file = file

	// 写入当前进程 PID
	pid := fmt.Sprintf("%d\n", os.Getpid())
	file.Truncate(0)
	file.Seek(0, 0)
	file.WriteString(pid)
	file.Sync()

	return nil
}

// TryLock 尝试获取文件锁（非阻塞）
func (fl *FileLock) TryLock() (bool, error) {
	file, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return false, fmt.Errorf("打开锁文件失败: %v", err)
	}

	// 尝试获取排他锁（非阻塞）
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK {
			return false, nil // 锁已被占用
		}
		return false, fmt.Errorf("获取锁失败: %v", err)
	}

	fl.file = file

	// 写入当前进程 PID
	pid := fmt.Sprintf("%d\n", os.Getpid())
	file.Truncate(0)
	file.Seek(0, 0)
	file.WriteString(pid)
	file.Sync()

	return true, nil
}

// Unlock 释放文件锁
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}

	// 释放锁
	err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN)
	if err != nil {
		fl.file.Close()
		return fmt.Errorf("释放锁失败: %v", err)
	}

	// 关闭文件
	err = fl.file.Close()
	fl.file = nil

	return err
}

// IsLocked 检查是否已被锁定
func (fl *FileLock) IsLocked() bool {
	return fl.file != nil
}
