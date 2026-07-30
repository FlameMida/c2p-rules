package main

import (
	"context"
	"os"

	"clash-rules-srs/internal/app"
	"clash-rules-srs/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, app.Commands()))
}
