package app

import (
	"fmt"
	"io"

	"clash-rules-srs/internal/releasecmp"
)

type ReleaseDecisionOptions struct {
	CandidateDir string
	CandidateTag string
	BaselineDir  string
	BaselineTag  string
	Mode         releasecmp.Mode
}

func ReleaseDecision(options ReleaseDecisionOptions, out io.Writer) error {
	decision, err := releasecmp.Decide(releasecmp.Input{
		CandidateDir: options.CandidateDir,
		CandidateTag: options.CandidateTag,
		BaselineDir:  options.BaselineDir,
		BaselineTag:  options.BaselineTag,
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
