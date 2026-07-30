package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-rules-srs/internal/tools"
)

func TestRunnerUsesOnlyConfiguredBinRoot(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "geoview"), "#!/bin/sh\nprintf '%s' \"$0\" > \"$1\"\n")
	out := filepath.Join(t.TempDir(), "used-path")
	runner := tools.Runner{BinRoot: bin, Timeout: time.Second, MaxLogBytes: 4096}
	if err := runner.Run(context.Background(), "geoview", "", out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != filepath.Join(bin, "geoview") {
		t.Fatalf("path=%q", got)
	}
}

func TestRunnerRejectsTraversalMissingToolsAndTimeouts(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "slow"), "#!/bin/sh\nsleep 1\n")
	runner := tools.Runner{BinRoot: bin, Timeout: 20 * time.Millisecond, MaxLogBytes: 128}
	for _, name := range []string{"../geoview", "missing", "slow"} {
		err := runner.Run(context.Background(), name, "")
		if err == nil {
			t.Fatalf("name=%q", name)
		}
	}
}

func TestRunnerBoundsStderr(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "noisy"), "#!/bin/sh\nprintf '1234567890' >&2\nexit 7\n")
	runner := tools.Runner{BinRoot: bin, Timeout: time.Second, MaxLogBytes: 4}
	err := runner.Run(context.Background(), "noisy", "")
	if err == nil || strings.Contains(err.Error(), "567890") || !strings.Contains(err.Error(), "1234") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunnerOutputReturnsBoundedStdout(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "output"), "#!/bin/sh\nprintf '1234567890'\n")
	runner := tools.Runner{BinRoot: bin, Timeout: time.Second, MaxLogBytes: 4}
	output, err := runner.Output(context.Background(), "output", "")
	if err != nil || output != "1234" {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
