package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
)

type Command func(context.Context, []string, io.Writer, io.Writer) error

type Commands struct {
	Bootstrap       Command
	Build           Command
	Verify          Command
	ReleaseDecision Command
}

type UsageError struct {
	Err error
}

func (e *UsageError) Error() string {
	return e.Err.Error()
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

func Run(ctx context.Context, args []string, out, errOut io.Writer, commands Commands) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: geodata-build <bootstrap|build|verify|release-decision> [options]")
		return 0
	}

	var command Command
	switch args[0] {
	case "bootstrap":
		command = commands.Bootstrap
	case "build":
		command = commands.Build
	case "verify":
		command = commands.Verify
	case "release-decision":
		command = commands.ReleaseDecision
	default:
		fmt.Fprintf(errOut, "ERROR: unknown command: %s\n", args[0])
		return 2
	}

	if command == nil {
		fmt.Fprintf(errOut, "ERROR: command not wired: %s\n", args[0])
		return 1
	}
	if err := command(ctx, args[1:], out, errOut); err != nil {
		fmt.Fprintf(errOut, "ERROR: %v\n", err)
		var usage *UsageError
		if errors.As(err, &usage) {
			return 2
		}
		return 1
	}
	return 0
}
