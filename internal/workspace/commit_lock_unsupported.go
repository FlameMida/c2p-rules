//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package workspace

import "fmt"

type rootCommitLock struct{}

func acquireCommitLock(string) (*rootCommitLock, error) {
	return nil, fmt.Errorf("cross-process workspace commits are unsupported on this platform")
}

func (*rootCommitLock) release() error { return nil }
