package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"clash-rules-srs/internal/cli"
)

func TestHelpIsSuccessful(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"--help"}, &out, &errOut, cli.Commands{})
	if code != 0 || out.String() == "" || errOut.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
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
