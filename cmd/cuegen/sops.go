package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/decrypt"
)

// sopsFilter is the engine.FileFilter that transparently decrypts
// sops-encrypted files using the official sops Go package. Only age
// recipients are supported (the sops package loads identities from
// SOPS_AGE_KEY, SOPS_AGE_KEY_FILE, or the default config path).
//
// Detection is byte-level: a file must contain "sops" and "unencrypted_suffix"
// (or the combined token "sops_unencrypted_suffix") to be considered. On a
// hit, we attempt to decrypt. Two outcomes are distinguished:
//
//   - Detection miss: the heuristic fired on a non-sops file (parse error or
//     no sops metadata). We warn to stderr and pass the raw bytes through
//     unchanged so CUE compiles the file as-is.
//   - Genuine decrypt failure: a real sops file that could not be decrypted
//     (missing/rotated key, corrupt ciphertext, MAC mismatch, …). This fails
//     HARD - returning the error aborts the render so ciphertext never
//     reaches the CUE compiler or a downstream deployment.
func sopsFilter(path string, raw []byte) ([]byte, error) {
	if !looksLikeSops(raw) {
		return raw, nil
	}
	plain, err := decrypt.Data(raw, sopsFormat(path))
	if err != nil {
		// MetadataNotFound or a parse error means the file tripped the
		// heuristic but isn't a genuine sops file - pass it through.
		if errors.Is(err, sops.MetadataNotFound) ||
			strings.Contains(err.Error(), "unmarshalling") {
			fmt.Fprintf(os.Stderr, "cuegen: sops decrypt %s: %v; passing through\n", path, err)
			return raw, nil
		}
		return nil, fmt.Errorf("sops decrypt %s: %w", path, err)
	}
	return plain, nil
}

// sopsFormat maps a file path to the format string the sops decrypt package
// expects: "yaml" for .yaml/.yml, "json" for .json, "binary" for everything
// else (including .cue, which sops encrypts as a binary store).
func sopsFormat(path string) string {
	switch {
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		return "yaml"
	case strings.HasSuffix(path, ".json"):
		return "json"
	default:
		return "binary"
	}
}

// looksLikeSops is a cheap pre-filter that decides whether a file is worth
// running through the decrypt path. Sops files come in two flavors:
//
//   - JSON: quoted keys "sops" and "unencrypted_suffix" (or the combined
//     "sops_unencrypted_suffix").
//   - YAML: unquoted keys sops: and unencrypted_suffix: (or
//     sops_unencrypted_suffix:).
//
// Checking for the colon-terminated form (sops:) avoids false positives from
// CUE source code that merely mentions sops in a comment, since a comment
// would say "sops" without a trailing colon.
func looksLikeSops(raw []byte) bool {
	// Combined token - quoted (JSON) or colon-terminated (YAML).
	if bytes.Contains(raw, []byte(`"sops_unencrypted_suffix"`)) ||
		bytes.Contains(raw, []byte("sops_unencrypted_suffix:")) {
		return true
	}
	// Separate keys - quoted (JSON) or colon-terminated (YAML).
	hasSops := bytes.Contains(raw, []byte(`"sops"`)) ||
		bytes.Contains(raw, []byte("sops:"))
	hasSuffix := bytes.Contains(raw, []byte(`"unencrypted_suffix"`)) ||
		bytes.Contains(raw, []byte("unencrypted_suffix:"))
	return hasSops && hasSuffix
}
