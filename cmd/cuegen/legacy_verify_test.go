package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sha256Hex is a local helper (the internal/hashing test helper of the same
// name lives in a different package).
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// withEmptyHashMap swaps legacyBinaryHashes for the duration of the test so
// unknown-platform behavior can be exercised without depending on the host's
// actual GOOS/GOARCH being absent from the map.
func withEmptyHashMap(t *testing.T) {
	t.Helper()
	saved := legacyBinaryHashes
	legacyBinaryHashes = map[string]string{}
	t.Cleanup(func() { legacyBinaryHashes = saved })
}

// writeBinary writes a small deterministic file to use as a stand-in for the
// legacy binary; its SHA256 is computed by the test for expected/mismatch
// cases.
func writeBinary(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cuegen_v0.16.8")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyLegacyIntegrityMatchEmbedded(t *testing.T) {
	// Use a content whose sha256 we inject into the map for the host platform.
	const content = "canonical legacy binary bytes\n"
	path := writeBinary(t, content)
	hash := sha256Hex([]byte(content))

	saved := legacyBinaryHashes
	legacyBinaryHashes = map[string]string{platformKey(): hash}
	t.Cleanup(func() { legacyBinaryHashes = saved })
	t.Setenv(legacyShaEnv, "")

	if err := verifyLegacyIntegrity(path); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestVerifyLegacyIntegrityMismatch(t *testing.T) {
	path := writeBinary(t, "some bytes")
	saved := legacyBinaryHashes
	legacyBinaryHashes = map[string]string{platformKey(): "deadbeef" + strings.Repeat("0", 56)}
	t.Cleanup(func() { legacyBinaryHashes = saved })
	t.Setenv(legacyShaEnv, "")

	err := verifyLegacyIntegrity(path)
	mm, ok := err.(mismatchError)
	if !ok {
		t.Fatalf("expected mismatchError, got %T %v", err, err)
	}
	if mm.expected == mm.got {
		t.Fatalf("expected and got should differ: %+v", mm)
	}
}

func TestVerifyLegacyIntegrityUnknownPlatform(t *testing.T) {
	withEmptyHashMap(t)
	t.Setenv(legacyShaEnv, "")
	path := writeBinary(t, "whatever")

	if err := verifyLegacyIntegrity(path); !errors.Is(err, errUnknownPlatform) {
		t.Fatalf("expected errUnknownPlatform, got %v", err)
	}
}

func TestVerifyLegacyIntegrityEnvOverrideMatch(t *testing.T) {
	const content = "self-built legacy binary\n"
	path := writeBinary(t, content)
	hash := sha256Hex([]byte(content))

	// Host platform intentionally absent from the map; override must win.
	withEmptyHashMap(t)
	t.Setenv(legacyShaEnv, "sha256:"+hash)

	if err := verifyLegacyIntegrity(path); err != nil {
		t.Fatalf("expected nil with matching override, got %v", err)
	}
}

func TestVerifyLegacyIntegrityEnvOverrideBareHex(t *testing.T) {
	const content = "self-built legacy binary\n"
	path := writeBinary(t, content)
	hash := sha256Hex([]byte(content))

	withEmptyHashMap(t)
	t.Setenv(legacyShaEnv, hash) // bare hex, no sha256: prefix

	if err := verifyLegacyIntegrity(path); err != nil {
		t.Fatalf("expected nil with bare-hex override, got %v", err)
	}
}

func TestVerifyLegacyIntegrityEnvSkip(t *testing.T) {
	path := writeBinary(t, "irrelevant")
	t.Setenv(legacyShaEnv, "skip")
	if err := verifyLegacyIntegrity(path); err != errSkipRequested {
		t.Fatalf("expected errSkipRequested, got %v", err)
	}
}

func TestVerifyLegacyIntegrityEnvSkipCaseInsensitive(t *testing.T) {
	path := writeBinary(t, "irrelevant")
	t.Setenv(legacyShaEnv, "SKIP")
	if err := verifyLegacyIntegrity(path); err != errSkipRequested {
		t.Fatalf("expected errSkipRequested for SKIP, got %v", err)
	}
}

func TestNormalizeDigest(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", ""},
		{"sha256:" + strings.Repeat("a", 64), strings.Repeat("a", 64)},
		{"SHA256:" + strings.Repeat("a", 64), strings.Repeat("a", 64)},
		{strings.Repeat("A", 64), strings.Repeat("a", 64)}, // uppercased bare hex normalized
		{strings.Repeat("z", 64), strings.Repeat("z", 64)}, // length ok; validity checked downstream
	}
	for _, c := range cases {
		if got := normalizeDigest(c.in); got != c.want {
			t.Errorf("normalizeDigest(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestVerifyLegacyIntegrityUnreadableFile(t *testing.T) {
	// A directory is not readable as a file via os.Open in the same way; use a
	// path that does not exist to surface the read error distinctly.
	t.Setenv(legacyShaEnv, "")
	if err := verifyLegacyIntegrity(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
