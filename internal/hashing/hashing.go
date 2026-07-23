// Package hashing implements algorithm-prefixed digests ("sha256:<hex>")
// for cuegen's drift-detection flags (-hash, -cmp-hash). These digests are
// not a security feature: they exist for cache invalidation and drift
// detection (has the rendered output changed since last time?), not to
// resist an adversary constructing a collision. The algorithm is prefixed
// in the digest string, not baked into a flag name, so a future algorithm
// change is a registry entry, not a CLI break.
package hashing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strings"
)

// minPrefixLen is the shortest hex prefix Parse accepts for a shortened
// digest, mirroring the same tradeoff git and docker make for short
// hashes/digests: short enough to be usable in a CI config, long enough
// that nobody accidentally compares on 2 characters.
const minPrefixLen = 12

// DefaultAlgo is the algorithm -hash computes and prints.
const DefaultAlgo = "sha256"

var algos = map[string]struct {
	new    func() hash.Hash
	hexLen int
}{
	"sha256": {sha256.New, 64},
}

// supported lists the registered algorithm names, sorted, for error
// messages.
func supported() string {
	names := make([]string, 0, len(algos))
	for name := range algos {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Digest is a parsed comparison value from -cmp-hash.
type Digest struct {
	Algo   string
	Hex    string // normalized (lowercase), possibly shortened
	Prefix bool   // true = prefix comparison rather than full match
}

// Parse reads a digest of the form "algo:hex". The algorithm prefix is
// mandatory so the input is self-describing and stays unambiguous as more
// algorithms are registered - there is deliberately no bare-hex form and no
// algorithm inference from hex length.
func Parse(s string) (Digest, error) {
	s = strings.TrimSpace(strings.ToLower(s))

	algo, hexPart, hasPrefix := strings.Cut(s, ":")
	if !hasPrefix {
		return Digest{}, fmt.Errorf(
			"digest %q is missing an algorithm prefix, expected form %s:<hex>", s, DefaultAlgo)
	}

	spec, ok := algos[algo]
	if !ok {
		return Digest{}, fmt.Errorf("unsupported hash algorithm %q (supported: %s)", algo, supported())
	}
	if hexPart == "" {
		return Digest{}, fmt.Errorf("digest %q has an empty hex part", s)
	}
	if _, err := hex.DecodeString(padOdd(hexPart)); err != nil {
		return Digest{}, fmt.Errorf("digest %q contains non-hex characters", hexPart)
	}
	if len(hexPart) > spec.hexLen {
		return Digest{}, fmt.Errorf("%s digest too long: %d characters, max %d", algo, len(hexPart), spec.hexLen)
	}
	if len(hexPart) < spec.hexLen && len(hexPart) < minPrefixLen {
		return Digest{}, fmt.Errorf(
			"digest prefix too short: %d characters, need at least %d", len(hexPart), minPrefixLen)
	}

	return Digest{Algo: algo, Hex: hexPart, Prefix: len(hexPart) < spec.hexLen}, nil
}

// Compute hashes data with the named algorithm.
func Compute(algo string, data []byte) (string, error) {
	spec, ok := algos[algo]
	if !ok {
		return "", fmt.Errorf("unsupported hash algorithm %q", algo)
	}
	h := spec.new()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Matches computes the digest of data using d's algorithm and compares it
// against d - a full comparison, or a prefix comparison if d was parsed
// from a shortened hex string. It also returns the full computed digest so
// callers can report it on a mismatch.
func (d Digest) Matches(data []byte) (bool, string, error) {
	full, err := Compute(d.Algo, data)
	if err != nil {
		return false, "", err
	}
	if d.Prefix {
		return strings.HasPrefix(full, d.Hex), full, nil
	}
	return full == d.Hex, full, nil
}

// String returns the digest in canonical "algo:hex" form.
func (d Digest) String() string { return d.Algo + ":" + d.Hex }

// padOdd appends a trailing "0" to odd-length input so hex.DecodeString can
// be used purely as a character-set check. Without this, an odd-length
// non-hex string (e.g. "zzz") is rejected by hex.DecodeString with "odd
// length hex string" instead of the intended "non-hex characters" - the
// wrong diagnostic for what's actually wrong with it.
func padOdd(s string) string {
	if len(s)%2 == 1 {
		return s + "0"
	}
	return s
}
