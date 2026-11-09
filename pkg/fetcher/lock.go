package fetcher

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// FileLock provides file-based locking using flock() syscall.
// This works correctly across containers sharing the same filesystem (PVC).
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a new file lock
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// Acquire attempts to acquire an exclusive lock with a timeout.
// Uses flock() which is automatically released if the process dies.
func (l *FileLock) Acquire(timeout time.Duration) error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	l.file = f

	// Try non-blocking lock first
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil // Got the lock immediately
	}

	// Lock is held by another process, wait with timeout
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			_ = l.file.Close()
			l.file = nil
			return fmt.Errorf("timeout waiting for lock %s", l.path)
		}

		time.Sleep(100 * time.Millisecond)

		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil // Got the lock
		}
	}
}

// Release releases the lock
func (l *FileLock) Release() error {
	if l.file == nil {
		return nil
	}

	// Unlock and close
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err := l.file.Close()
	l.file = nil
	return err
}
