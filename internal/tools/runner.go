package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Runner struct {
	BinRoot     string
	Timeout     time.Duration
	MaxLogBytes int64
}

func (r *Runner) Run(ctx context.Context, name, cwd string, args ...string) error {
	_, err := r.Output(ctx, name, cwd, args...)
	return err
}

func (r *Runner) Output(ctx context.Context, name, cwd string, args ...string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("tool runner is nil")
	}
	if filepath.Base(name) != name || name == "." || name == "" || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("unsafe tool name %q", name)
	}
	tool := filepath.Join(r.BinRoot, name)
	info, err := os.Stat(tool)
	if err != nil {
		return "", fmt.Errorf("tool %s missing at %s: %w", name, tool, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("tool %s is not executable at %s", name, tool)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	maxLogBytes := r.MaxLogBytes
	if maxLogBytes <= 0 {
		maxLogBytes = 64 * 1024
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, tool, args...)
	if cwd != "" {
		command.Dir = cwd
	}
	command.WaitDelay = time.Second
	stdout := &cappedBuffer{limit: maxLogBytes}
	stderr := &cappedBuffer{limit: maxLogBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if commandContext.Err() != nil {
			return stdout.String(), fmt.Errorf("tool %s timed out after %s: %w", name, timeout, commandContext.Err())
		}
		return stdout.String(), fmt.Errorf("tool %s failed: %w; stderr=%s", name, err, stderr.String())
	}
	return stdout.String(), nil
}

type cappedBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int64
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - int64(len(b.data))
	if remaining > 0 {
		keep := int64(len(data))
		if keep > remaining {
			keep = remaining
		}
		b.data = append(b.data, data[:keep]...)
	}
	return len(data), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
