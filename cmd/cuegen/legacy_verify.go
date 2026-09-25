package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Sentinel and typed errors for legacy-binary integrity verification. runLegacy
// branches on these to tailor its diagnostics.
var errUnknownPlatform = errors.New("no expected SHA256 for legacy binary on this platform")

// errSkipRequested signals that the operator opted out of verification via
// CUEGEN_LEGACY_SHA256=skip; runLegacy warns loudly and proceeds.
var errSkipRequested = errors.New("integrity check skipped by operator")

// mismatchError carries both digests so the fatal diagnostic can show what was
// expected versus what was found.
type mismatchError struct{ expected, got string }

func (e mismatchError) Error() string {
	return fmt.Sprintf("SHA256 mismatch: expected %s, got %s", e.expected, e.got)
}

// legacyShaEnv is the escape hatch / test-injection bridge. Because the legacy
// fallback tests run cuegen as a compiled subprocess, a package-level var
// mutated in the test process cannot reach it; the env var can. Values:
//   - unset               -> use legacyBinaryHashes[platformKey()]
//   - "skip"              -> bypass with a loud warning (operator opt-out)
//   - "sha256:<64-hex>"   -> that digest is the sole expected hash (self-built)
//   - "<64-hex>"          -> same, without the algo prefix
const legacyShaEnv = "CUEGEN_LEGACY_SHA256"

// verifyLegacyIntegrity reads the binary at path, computes its SHA256, and
// compares it against the expected digest. The expected digest comes from
// legacyBinaryHashes keyed by platformKey(), unless CUEGEN_LEGACY_SHA256
// overrides it. Returns nil on a match, errSkipRequested when the operator
// bypassed the check, errUnknownPlatform when no hash is known and no override
// is set, or a mismatchError on a failed comparison.
func verifyLegacyIntegrity(path string) error {
	override := strings.TrimSpace(os.Getenv(legacyShaEnv))
	if strings.EqualFold(override, "skip") {
		return errSkipRequested
	}

	expected, err := expectedHash(override)
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read legacy binary %q: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash legacy binary %q: %w", path, err)
	}
	got := hex.EncodeToString(h.Sum(nil))

	if got != expected {
		return mismatchError{expected: expected, got: got}
	}
	return nil
}

// expectedHash resolves the expected SHA256 from an explicit override or the
// embedded platform map. A bare or prefixed 64-char hex digest is accepted as
// an override; anything else set but not "skip"-like falls through to the map
// (so a garbage value does not silently disable verification — it just fails
// to parse and the platform map is consulted instead).
func expectedHash(override string) (string, error) {
	if d := normalizeDigest(override); d != "" {
		return d, nil
	}
	if h, ok := legacyBinaryHashes[platformKey()]; ok {
		return h, nil
	}
	return "", errUnknownPlatform
}

// normalizeDigest accepts "sha256:<hex>" or a bare 64-char lowercase hex
// string and returns the bare hex digest, or "" if the input is not a usable
// SHA256 digest.
func normalizeDigest(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "sha256:") {
		s = s[len("sha256:"):]
	}
	s = strings.Trim(s, " ")
	if len(s) != 64 {
		return ""
	}
	return strings.ToLower(s)
}
