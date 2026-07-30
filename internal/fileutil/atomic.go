package fileutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func AtomicWrite(path string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".atomic-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary mode for %s: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	remove = false
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory %s for sync: %w", directory, err)
	}
	syncErr := directoryHandle.Sync()
	closeErr := directoryHandle.Close()
	if syncErr != nil {
		return fmt.Errorf("sync directory %s: %w", directory, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close directory %s: %w", directory, closeErr)
	}
	return nil
}
