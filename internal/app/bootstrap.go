package app

import (
	"context"
	"fmt"

	"clash-rules-srs/internal/tools"
)

type BootstrapOptions struct {
	CacheRoot string
}

func Bootstrap(ctx context.Context, options BootstrapOptions, executor tools.Executor) error {
	if options.CacheRoot == "" {
		return fmt.Errorf("cache root is required")
	}
	return tools.Bootstrap(ctx, options.CacheRoot, executor)
}
