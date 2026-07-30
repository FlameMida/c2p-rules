package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ToolPins struct {
	DomainListCustom string
	GeoIP            string
	GeoView          string
}

var Pins = ToolPins{
	DomainListCustom: "efacb51b8950ae673ebb6dcb9e7ecdd1decb1b6d",
	GeoIP:            "85084dfbe282e4e9cb460b07196e6eecfd126d19",
	GeoView:          "3c91926d360b8f49d47520639e574608318baf12",
}

type Executor interface {
	Run(context.Context, string, string, ...string) error
}

type OSExecutor struct {
	Timeout     time.Duration
	MaxLogBytes int64
}

func (e OSExecutor) Run(ctx context.Context, cwd, program string, args ...string) error {
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, program, args...)
	command.Dir = cwd
	command.WaitDelay = time.Second
	stdout := &cappedBuffer{limit: defaultLogLimit(e.MaxLogBytes)}
	stderr := &cappedBuffer{limit: defaultLogLimit(e.MaxLogBytes)}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if commandContext.Err() != nil {
			return fmt.Errorf("%s timed out after %s: %w", program, timeout, commandContext.Err())
		}
		return fmt.Errorf("%s failed: %w; stderr=%s", program, err, stderr.String())
	}
	return nil
}

func Bootstrap(ctx context.Context, cacheRoot string, executor Executor) error {
	if cacheRoot == "" {
		return fmt.Errorf("cache root is empty")
	}
	if executor == nil {
		return fmt.Errorf("bootstrap executor is nil")
	}
	cacheAbsolute, err := filepath.Abs(cacheRoot)
	if err != nil {
		return fmt.Errorf("resolve cache root %s: %w", cacheRoot, err)
	}
	upstreamRoot := filepath.Join(cacheAbsolute, "upstream")
	binRoot := filepath.Join(cacheAbsolute, "bin")
	if err := os.MkdirAll(upstreamRoot, 0o755); err != nil {
		return fmt.Errorf("create upstream cache: %w", err)
	}
	if err := os.MkdirAll(binRoot, 0o755); err != nil {
		return fmt.Errorf("create binary cache: %w", err)
	}

	community := filepath.Join(upstreamRoot, "domain-list-community")
	if err := syncRolling(ctx, executor, "https://github.com/v2fly/domain-list-community.git", community); err != nil {
		return fmt.Errorf("bootstrap domain-list-community: %w", err)
	}
	tools := []struct {
		name       string
		repository string
		commit     string
	}{
		{"domain-list-custom", "https://github.com/Loyalsoldier/domain-list-custom.git", Pins.DomainListCustom},
		{"geoip", "https://github.com/Loyalsoldier/geoip.git", Pins.GeoIP},
		{"geoview", "https://github.com/snowie2000/geoview.git", Pins.GeoView},
	}
	for _, tool := range tools {
		destination := filepath.Join(upstreamRoot, tool.name)
		if err := checkoutPinned(ctx, executor, tool.repository, destination, tool.commit); err != nil {
			return fmt.Errorf("bootstrap %s: %w", tool.name, err)
		}
		if err := executor.Run(ctx, destination, "go", "build", "-trimpath", "-o", filepath.Join(binRoot, tool.name), "."); err != nil {
			return fmt.Errorf("build %s: %w", tool.name, err)
		}
	}
	return nil
}

func syncRolling(ctx context.Context, executor Executor, repository, destination string) error {
	if _, err := os.Stat(filepath.Join(destination, ".git")); os.IsNotExist(err) {
		if err := executor.Run(ctx, "", "git", "clone", "--depth", "1", "--filter=blob:none", repository, destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "remote", "set-url", "origin", repository); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "fetch", "--depth", "1", "origin", "HEAD"); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "clean", "-ffdx"); err != nil {
		return err
	}
	return executor.Run(ctx, "", "git", "-C", destination, "rev-parse", "--verify", "HEAD")
}

func checkoutPinned(ctx context.Context, executor Executor, repository, destination, commit string) error {
	if _, err := os.Stat(filepath.Join(destination, ".git")); os.IsNotExist(err) {
		if err := executor.Run(ctx, "", "git", "clone", "--filter=blob:none", "--no-checkout", repository, destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "remote", "set-url", "origin", repository); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "fetch", "--depth", "1", "origin", commit); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "checkout", "--detach", commit); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "reset", "--hard", commit); err != nil {
		return err
	}
	if err := executor.Run(ctx, "", "git", "-C", destination, "clean", "-ffdx"); err != nil {
		return err
	}
	return nil
}

func defaultLogLimit(value int64) int64 {
	if value <= 0 {
		return 64 * 1024
	}
	return value
}
