package hashing

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestParseValid(t *testing.T) {
	full := sha256Hex([]byte("hello"))
	tests := []struct {
		name       string
		in         string
		wantHex    string
		wantPrefix bool
	}{
		{"full lowercase", "sha256:" + full, full, false},
		{"full uppercase normalizes", "SHA256:" + strings.ToUpper(full), full, false},
		{"12-char prefix (minimum)", "sha256:" + full[:12], full[:12], true},
		{"64-char is full, not prefix", "sha256:" + full, full, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if d.Algo != "sha256" {
				t.Errorf("Algo = %q, want sha256", d.Algo)
			}
			if d.Hex != tc.wantHex {
				t.Errorf("Hex = %q, want %q", d.Hex, tc.wantHex)
			}
			if d.Prefix != tc.wantPrefix {
				t.Errorf("Prefix = %v, want %v", d.Prefix, tc.wantPrefix)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	full := sha256Hex([]byte("hello"))
	tests := []struct {
		name string
		in   string
	}{
		{"no prefix (bare hex)", full},
		{"unknown algorithm", "md5:" + full},
		{"empty hex part", "sha256:"},
		{"non-hex characters", "sha256:" + strings.Repeat("z", 12)},
		{"11-char prefix (below minimum)", "sha256:" + full[:11]},
		{"too long", "sha256:" + full + "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.in); err == nil {
				t.Fatalf("Parse(%q): expected error, got nil", tc.in)
			}
		})
	}
}

func TestParseUnknownAlgorithmListsSupported(t *testing.T) {
	_, err := Parse("md5:deadbeef")
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("error = %v, want it to list supported algorithms", err)
	}
}

func TestParseMissingPrefixNamesExpectedForm(t *testing.T) {
	_, err := Parse("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil || !strings.Contains(err.Error(), "sha256:<hex>") {
		t.Fatalf("error = %v, want it to name the expected form", err)
	}
}

func TestMatchesFull(t *testing.T) {
	data := []byte("hello world")
	d, err := Parse("sha256:" + sha256Hex(data))
	if err != nil {
		t.Fatal(err)
	}
	ok, full, err := d.Matches(data)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected match")
	}
	if full != sha256Hex(data) {
		t.Errorf("full = %q, want %q", full, sha256Hex(data))
	}

	ok, _, err = d.Matches([]byte("different"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected mismatch for different data")
	}
}

func TestMatchesPrefix(t *testing.T) {
	data := []byte("hello world")
	full := sha256Hex(data)
	d, err := Parse("sha256:" + full[:12])
	if err != nil {
		t.Fatal(err)
	}
	ok, _, err := d.Matches(data)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected prefix match")
	}

	ok, _, err = d.Matches([]byte("something else entirely"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected prefix mismatch for different data")
	}
}

func TestComputeUnsupportedAlgo(t *testing.T) {
	if _, err := Compute("md5", []byte("x")); err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
}

func TestDigestString(t *testing.T) {
	d := Digest{Algo: "sha256", Hex: "abcd"}
	if got, want := d.String(), "sha256:abcd"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
