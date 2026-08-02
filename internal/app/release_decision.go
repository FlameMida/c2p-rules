package app

import (
	"fmt"
	"io"

	"clash-rules-srs/internal/releasecmp"
)

type ReleaseDecisionOptions struct {
	CandidateDir string
	BaselineDir  string
	Mode         releasecmp.Mode
}

func ReleaseDecision(options ReleaseDecisionOptions, out io.Writer) error {
	decision, err := releasecmp.Decide(releasecmp.Input{
		CandidateDir: options.CandidateDir,
		BaselineDir:  options.BaselineDir,
		Mode:         options.Mode,
	})
	if err != nil {
		return err
	}
	baseline := decision.BaselineFingerprint
	if baseline == "" {
		baseline = "none"
	}
	_, err = fmt.Fprintf(
		out,
		"should_publish=%t\nreason=%s\nbaseline_fingerprint=%s\n",
		decision.ShouldPublish,
		decision.Reason,
		baseline,
	)
	return err
}
