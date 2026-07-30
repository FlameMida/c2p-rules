//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type rootCommitLock struct {
	file *os.File
}

func acquireOSCommitLock(root string) (commitLock, error) {
	path := filepath.Join(root, ".commit.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	return &rootCommitLock{file: file}, nil
}

func (l *rootCommitLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
