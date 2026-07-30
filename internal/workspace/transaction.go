package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type FS interface {
	MkdirTemp(dir, pattern string) (string, error)
	MkdirAll(path string, perm fs.FileMode) error
	Rename(oldPath, newPath string) error
	RemoveAll(path string) error
	Stat(path string) (fs.FileInfo, error)
}

type OSFS struct{}

func (OSFS) MkdirTemp(dir, pattern string) (string, error) { return os.MkdirTemp(dir, pattern) }
func (OSFS) MkdirAll(path string, perm fs.FileMode) error  { return os.MkdirAll(path, perm) }
func (OSFS) Rename(oldPath, newPath string) error          { return os.Rename(oldPath, newPath) }
func (OSFS) RemoveAll(path string) error                   { return os.RemoveAll(path) }
func (OSFS) Stat(path string) (fs.FileInfo, error)         { return os.Stat(path) }

type transactionState uint8

type commitLock interface {
	release() error
}

var acquireCommitLock = acquireOSCommitLock

const (
	stateActive transactionState = iota
	stateFailed
	stateCommitted
	stateAborted
)

type switchOperation struct {
	name      string
	staged    string
	final     string
	backup    string
	hadFinal  bool
	backedUp  bool
	installed bool
}

type Transaction struct {
	mu      sync.Mutex
	fs      FS
	root    string
	layout  Layout
	state   transactionState
	pending []switchOperation
}

func Begin(root string) (*Transaction, error) {
	return BeginWithFS(root, OSFS{})
}

func BeginWithFS(root string, filesystem FS) (*Transaction, error) {
	if filesystem == nil {
		return nil, fmt.Errorf("workspace filesystem is nil")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root %s: %w", root, err)
	}
	if err := filesystem.MkdirAll(absolute, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace root %s: %w", absolute, err)
	}
	staging, err := filesystem.MkdirTemp(absolute, ".staging-")
	if err != nil {
		return nil, fmt.Errorf("create staging in %s: %w", absolute, err)
	}
	layout := newLayout(staging)
	for _, directory := range []string{layout.Build, layout.Publish, layout.DataMerged, layout.IP} {
		if err := filesystem.MkdirAll(directory, 0o755); err != nil {
			_ = filesystem.RemoveAll(staging)
			return nil, fmt.Errorf("create staging directory %s: %w", directory, err)
		}
	}
	return &Transaction{fs: filesystem, root: absolute, layout: layout, state: stateActive}, nil
}

func (t *Transaction) Layout() Layout {
	if t == nil {
		return Layout{}
	}
	return t.layout
}

func (t *Transaction) Commit() (err error) {
	if t == nil {
		return fmt.Errorf("workspace transaction is nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != stateActive {
		return fmt.Errorf("workspace transaction cannot commit in state %d", t.state)
	}
	commitLock, err := acquireCommitLock(t.root)
	if err != nil {
		return fmt.Errorf("lock workspace commit: %w", err)
	}
	defer func() {
		if unlockErr := commitLock.release(); unlockErr != nil {
			if t.state != stateCommitted {
				err = errors.Join(err, fmt.Errorf("unlock workspace commit: %w", unlockErr))
			}
		}
	}()
	operations := []switchOperation{
		{name: "build", staged: t.layout.Build, final: filepath.Join(t.root, "build"), backup: filepath.Join(t.layout.Staging, ".backup-build")},
		{name: "publish", staged: t.layout.Publish, final: filepath.Join(t.root, "publish"), backup: filepath.Join(t.layout.Staging, ".backup-publish")},
	}
	for index := range operations {
		info, err := t.fs.Stat(operations[index].final)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("final %s path is not a directory: %s", operations[index].name, operations[index].final)
			}
			operations[index].hadFinal = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat final %s: %w", operations[index].name, err)
		}
	}

	for index := range operations {
		operation := &operations[index]
		if operation.hadFinal {
			if err := t.fs.Rename(operation.final, operation.backup); err != nil {
				return t.failAndRollback(operations, fmt.Errorf("backup final %s: %w", operation.name, err))
			}
			operation.backedUp = true
		}
		if err := t.fs.Rename(operation.staged, operation.final); err != nil {
			return t.failAndRollback(operations, fmt.Errorf("install staged %s: %w", operation.name, err))
		}
		operation.installed = true
	}

	t.state = stateCommitted
	t.pending = nil
	_ = t.fs.RemoveAll(t.layout.Staging)
	return nil
}

func (t *Transaction) failAndRollback(operations []switchOperation, cause error) error {
	rollbackErr := t.rollback(operations)
	if rollbackErr != nil {
		t.state = stateFailed
		t.pending = operations
		return errors.Join(cause, fmt.Errorf("rollback workspace switch: %w", rollbackErr))
	}
	t.state = stateActive
	return cause
}

func (t *Transaction) rollback(operations []switchOperation) error {
	var rollbackErr error
	for index := len(operations) - 1; index >= 0; index-- {
		operation := &operations[index]
		if operation.installed {
			if err := t.fs.Rename(operation.final, operation.staged); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove installed %s: %w", operation.name, err))
				continue
			}
			operation.installed = false
		}
		if operation.backedUp {
			if err := t.fs.Rename(operation.backup, operation.final); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore final %s: %w", operation.name, err))
				continue
			}
			operation.backedUp = false
		}
	}
	return rollbackErr
}

func (t *Transaction) Abort() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch t.state {
	case stateCommitted, stateAborted:
		return nil
	case stateFailed:
		if err := t.rollback(t.pending); err != nil {
			return fmt.Errorf("recover failed workspace transaction: %w", err)
		}
		t.pending = nil
		t.state = stateActive
	}
	if err := t.fs.RemoveAll(t.layout.Staging); err != nil {
		return fmt.Errorf("remove staging %s: %w", t.layout.Staging, err)
	}
	t.state = stateAborted
	return nil
}
