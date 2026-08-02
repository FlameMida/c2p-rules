package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	"clash-rules-srs/internal/cli"
)

func TestHelpIsSuccessful(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"--help"}, &out, &errOut, cli.Commands{})
	if code != 0 || out.String() == "" || errOut.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("bootstrap|build|verify|release-decision")) {
		t.Fatalf("help=%q", out.String())
	}
}

func TestReleaseDecisionCommandIsDispatched(t *testing.T) {
	want := []string{"--candidate", "publish", "--candidate-tag", "candidate", "--force"}
	called := false
	command := func(_ context.Context, args []string, _, _ io.Writer) error {
		called = true
		if !slices.Equal(args, want) {
			t.Fatalf("args=%v", args)
		}
		return nil
	}
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), append([]string{"release-decision"}, want...), &out, &errOut, cli.Commands{
		ReleaseDecision: command,
	})
	if code != 0 || !called || out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("code=%d called=%t stdout=%q stderr=%q", code, called, out.String(), errOut.String())
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"unknown"}, &out, &errOut, cli.Commands{})
	if code != 2 || errOut.String() != "ERROR: unknown command: unknown\n" {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}

func TestCommandUsageErrorReturnsExitTwo(t *testing.T) {
	command := func(context.Context, []string, io.Writer, io.Writer) error {
		return &cli.UsageError{Err: errors.New("bad flag")}
	}
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"build"}, &out, &errOut, cli.Commands{Build: command})
	if code != 2 || errOut.String() != "ERROR: bad flag\n" {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}
