package config

import (
	"io"

	"clash-rules-srs/internal/yamlutil"
)

func DecodeStrict(r io.Reader, out any) error {
	return yamlutil.DecodeStrict(r, out, yamlutil.StrictOptions{})
}
