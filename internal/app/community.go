package app

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func regularFileEqualFold(root, name string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() && strings.EqualFold(entry.Name(), name) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}
