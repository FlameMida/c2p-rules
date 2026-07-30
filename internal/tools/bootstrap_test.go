package tools_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"clash-rules-srs/internal/tools"
)

type recordedCall struct {
	cwd     string
	program string
	args    []string
}

type recordingExecutor struct {
	calls  []recordedCall
	failAt int
}

func (e *recordingExecutor) Run(_ context.Context, cwd, program string, args ...string) error {
	e.calls = append(e.calls, recordedCall{cwd: cwd, program: program, args: append([]string(nil), args...)})
	if e.failAt > 0 && len(e.calls) == e.failAt {
		return errors.New("injected failure")
	}
	return nil
}

func TestBootstrapUsesExactPinsAndBuildsAllTools(t *testing.T) {
	cache := t.TempDir()
	executor := &recordingExecutor{}
	if err := tools.Bootstrap(context.Background(), cache, executor); err != nil {
		t.Fatal(err)
	}
	text := callsText(executor.calls)
	for name, commit := range map[string]string{
		"domain-list-custom": "efacb51b8950ae673ebb6dcb9e7ecdd1decb1b6d",
		"geoip":              "85084dfbe282e4e9cb460b07196e6eecfd126d19",
		"geoview":            "3c91926d360b8f49d47520639e574608318baf12",
	} {
		if !strings.Contains(text, "fetch --depth 1 origin "+commit) || !strings.Contains(text, "checkout --detach "+commit) {
			t.Fatalf("%s pin missing from calls:\n%s", name, text)
		}
		wantOutput := filepath.Join(cache, "bin", name)
		if !strings.Contains(text, "go build -trimpath -o "+wantOutput+" .") {
			t.Fatalf("%s build missing from calls:\n%s", name, text)
		}
	}
	if !strings.Contains(text, "domain-list-community.git") || !strings.Contains(text, "fetch --depth 1 origin HEAD") {
		t.Fatalf("rolling community missing:\n%s", text)
	}
	for _, revision := range []string{"FETCH_HEAD", tools.Pins.DomainListCustom, tools.Pins.GeoIP, tools.Pins.GeoView} {
		if !strings.Contains(text, "reset --hard "+revision) {
			t.Fatalf("checkout %s is not reset to a clean revision:\n%s", revision, text)
		}
	}
	if got := strings.Count(text, "clean -ffdx"); got != 4 {
		t.Fatalf("clean calls=%d, want 4:\n%s", got, text)
	}
}

func TestBootstrapStopsAtFirstExecutorFailure(t *testing.T) {
	executor := &recordingExecutor{failAt: 2}
	err := tools.Bootstrap(context.Background(), t.TempDir(), executor)
	if err == nil || !strings.Contains(err.Error(), "injected failure") || len(executor.calls) != 2 {
		t.Fatalf("calls=%v err=%v", executor.calls, err)
	}
}

func TestPinsAreStable(t *testing.T) {
	want := tools.ToolPins{
		DomainListCustom: "efacb51b8950ae673ebb6dcb9e7ecdd1decb1b6d",
		GeoIP:            "85084dfbe282e4e9cb460b07196e6eecfd126d19",
		GeoView:          "3c91926d360b8f49d47520639e574608318baf12",
	}
	if !reflect.DeepEqual(tools.Pins, want) {
		t.Fatalf("pins=%#v", tools.Pins)
	}
}

func callsText(calls []recordedCall) string {
	var lines []string
	for _, call := range calls {
		lines = append(lines, strings.TrimSpace(call.cwd+" "+call.program+" "+strings.Join(call.args, " ")))
	}
	return strings.Join(lines, "\n")
}
