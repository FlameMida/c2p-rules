package releasecmp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"clash-rules-srs/internal/passwall"
	"clash-rules-srs/internal/verify"
)

type Mode uint8

const (
	Compare Mode = iota + 1
	FirstRelease
	Force
)

type Input struct {
	CandidateDir string
	CandidateTag string
	BaselineDir  string
	BaselineTag  string
	Mode         Mode
}

type Decision struct {
	ShouldPublish       bool
	Reason              string
	BaselineFingerprint string
}

var (
	releaseTagField      = regexp.MustCompile(`(?m)^[\t ]*RELEASE_TAG[\t ]*=.*$`)
	releaseTagAssignment = regexp.MustCompile(`^RELEASE_TAG='([^'\r\n]*)'$`)
)

const normalizedTagAssignment = "RELEASE_TAG='__CLASH_RULES_SRS_RELEASE_TAG__'"

func NormalizeInstaller(data []byte) ([]byte, error) {
	normalized, _, err := normalizeInstaller(data)
	return normalized, err
}

func normalizeInstaller(data []byte) ([]byte, string, error) {
	fields := releaseTagField.FindAllIndex(data, -1)
	if len(fields) != 1 {
		return nil, "", fmt.Errorf("installer must contain exactly one RELEASE_TAG field: found %d", len(fields))
	}
	start, end := fields[0][0], fields[0][1]
	assignment := releaseTagAssignment.FindSubmatchIndex(data[start:end])
	if assignment == nil {
		return nil, "", fmt.Errorf("installer RELEASE_TAG field does not use generated syntax")
	}
	value := string(data[start+assignment[2] : start+assignment[3]])
	if err := passwall.ValidateReleaseTag(value); err != nil {
		return nil, "", fmt.Errorf("invalid RELEASE_TAG assignment: %w", err)
	}
	result := make([]byte, 0, len(data)-(end-start)+len(normalizedTagAssignment))
	result = append(result, data[:start]...)
	result = append(result, normalizedTagAssignment...)
	result = append(result, data[end:]...)
	return result, value, nil
}

func Decide(input Input) (Decision, error) {
	candidateFingerprint, err := fingerprint(input.CandidateDir, input.CandidateTag)
	if err != nil {
		return Decision{}, fmt.Errorf("candidate payload: %w", err)
	}
	switch input.Mode {
	case FirstRelease:
		return Decision{ShouldPublish: true, Reason: "first-release"}, nil
	case Force:
		return Decision{ShouldPublish: true, Reason: "forced"}, nil
	case Compare:
		baselineFingerprint, err := fingerprint(input.BaselineDir, input.BaselineTag)
		if err != nil {
			return Decision{}, fmt.Errorf("baseline payload: %w", err)
		}
		changed := candidateFingerprint != baselineFingerprint
		reason := "unchanged"
		if changed {
			reason = "changed"
		}
		return Decision{
			ShouldPublish:       changed,
			Reason:              reason,
			BaselineFingerprint: baselineFingerprint,
		}, nil
	default:
		return Decision{}, fmt.Errorf("invalid release decision mode %d", input.Mode)
	}
}

func fingerprint(directory, expectedTag string) (string, error) {
	if err := verify.Assets(directory, verify.ReleaseAssets()); err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, name := range []string{"geosite.dat", "geoip.dat"} {
		if err := hashFileFrame(digest, directory, name); err != nil {
			return "", err
		}
	}
	installer, err := os.ReadFile(filepath.Join(directory, "install_passwall2_rules.sh"))
	if err != nil {
		return "", fmt.Errorf("read installer: %w", err)
	}
	normalized, actualTag, err := normalizeInstaller(installer)
	if err != nil {
		return "", err
	}
	if err := passwall.ValidateReleaseTag(expectedTag); err != nil {
		return "", fmt.Errorf("invalid expected release tag: %w", err)
	}
	if actualTag != expectedTag {
		return "", fmt.Errorf("release tag mismatch: installer=%q expected=%q", actualTag, expectedTag)
	}
	if err := hashFrame(digest, "install_passwall2_rules.sh", int64(len(normalized)), bytes.NewReader(normalized)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func hashFileFrame(digest hash.Hash, directory, name string) error {
	file, err := os.Open(filepath.Join(directory, name))
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat %s: %w", name, err)
	}
	hashErr := hashFrame(digest, name, info.Size(), file)
	closeErr := file.Close()
	if hashErr != nil {
		return hashErr
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", name, closeErr)
	}
	return nil
}

func hashFrame(digest hash.Hash, name string, size int64, source io.Reader) error {
	if size < 0 {
		return fmt.Errorf("negative content length for %s", name)
	}
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(len(name)))
	if _, err := digest.Write(encoded[:]); err != nil {
		return fmt.Errorf("frame name length for %s: %w", name, err)
	}
	if _, err := io.WriteString(digest, name); err != nil {
		return fmt.Errorf("frame name %s: %w", name, err)
	}
	binary.BigEndian.PutUint64(encoded[:], uint64(size))
	if _, err := digest.Write(encoded[:]); err != nil {
		return fmt.Errorf("frame content length for %s: %w", name, err)
	}
	written, err := io.Copy(digest, source)
	if err != nil {
		return fmt.Errorf("hash content %s: %w", name, err)
	}
	if written != size {
		return fmt.Errorf("content length changed for %s: expected %d, read %d", name, size, written)
	}
	return nil
}
