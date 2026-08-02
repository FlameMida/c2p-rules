package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"clash-rules-srs/internal/fileutil"
)

var releaseAssetNames = [...]string{
	"geoip.dat",
	"geoip.dat.sha256sum",
	"geosite.dat",
	"geosite.dat.sha256sum",
	"install_passwall2_rules.sh",
	"install_passwall2_rules.sh.sha256sum",
}

func ReleaseAssets() []string {
	assets := make([]string, len(releaseAssetNames))
	copy(assets, releaseAssetNames[:])
	return assets
}

func WriteSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open checksum target %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("stat checksum target %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return "", fmt.Errorf("checksum target is not a regular file: %s", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close checksum target %s: %w", path, err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	content := []byte(digest + "  " + filepath.Base(path) + "\n")
	if err := fileutil.AtomicWrite(path+".sha256sum", content, 0o644); err != nil {
		return "", fmt.Errorf("write checksum for %s: %w", path, err)
	}
	return digest, nil
}

func Assets(directory string, expected []string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read asset directory %s: %w", directory, err)
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, name := range expected {
		if filepath.Base(name) != name || name == "." || name == "" {
			return fmt.Errorf("unsafe expected asset name %q", name)
		}
		if _, duplicate := expectedSet[name]; duplicate {
			return fmt.Errorf("duplicate expected asset %q", name)
		}
		expectedSet[name] = struct{}{}
	}
	actualSet := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("asset is not a regular file: %s", entry.Name())
		}
		actualSet[entry.Name()] = struct{}{}
	}
	var missing, unexpected []string
	for name := range expectedSet {
		if _, exists := actualSet[name]; !exists {
			missing = append(missing, name)
		}
	}
	for name := range actualSet {
		if _, exists := expectedSet[name]; !exists {
			unexpected = append(unexpected, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	if len(missing) != 0 || len(unexpected) != 0 {
		return fmt.Errorf("asset set mismatch: missing=%v unexpected=%v", missing, unexpected)
	}
	for name := range expectedSet {
		if !strings.HasSuffix(name, ".sha256sum") {
			continue
		}
		if err := verifyChecksum(filepath.Join(directory, name)); err != nil {
			return err
		}
	}
	return nil
}

func verifyChecksum(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read checksum %s: %w", path, err)
	}
	if len(content) != 64+2+len(filepath.Base(strings.TrimSuffix(path, ".sha256sum")))+1 || content[len(content)-1] != '\n' {
		return fmt.Errorf("checksum format mismatch: %s", path)
	}
	line := strings.TrimSuffix(string(content), "\n")
	digest, targetName, found := strings.Cut(line, "  ")
	if !found || targetName != filepath.Base(strings.TrimSuffix(path, ".sha256sum")) || len(digest) != 64 {
		return fmt.Errorf("checksum format mismatch: %s", path)
	}
	if _, err := hex.DecodeString(digest); err != nil || strings.ToLower(digest) != digest {
		return fmt.Errorf("checksum digest is invalid: %s", path)
	}
	target, err := os.Open(filepath.Join(filepath.Dir(path), targetName))
	if err != nil {
		return fmt.Errorf("open checksum target %s: %w", targetName, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, target)
	closeErr := target.Close()
	if copyErr != nil {
		return fmt.Errorf("hash checksum target %s: %w", targetName, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close checksum target %s: %w", targetName, closeErr)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != digest {
		return fmt.Errorf("checksum mismatch for %s", targetName)
	}
	return nil
}
