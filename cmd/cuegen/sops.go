package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
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
//   - Detection miss: the heuristic fired on a non-sops file. We warn to
//     stderr and pass the raw bytes through unchanged so CUE compiles the
//     file as-is. This covers a missing sops metadata block (the typed
//     sops.MetadataNotFound sentinel) and unparseable input (the sops stores
//     return these as untyped fmt.Errorf values mentioning "unmarshal", so a
//     substring match is the only available signal - typed errors are
//     preferred wherever sops exposes them).
//   - Genuine decrypt failure: a real sops file that could not be decrypted
//     (missing/rotated key, corrupt ciphertext, MAC mismatch via
//     sops.MacMismatch, …). This fails HARD - returning the error aborts the
//     render so ciphertext never reaches the CUE compiler or a downstream
//     deployment.
func sopsFilter(path string, raw []byte) ([]byte, error) {
	if !looksLikeSops(raw) {
		return raw, nil
	}
	plain, err := decrypt.Data(raw, sopsFormat(path))
	if err != nil {
		// A false positive whose error indicates the input isn't a genuine
		// sops file is passed through: the typed MetadataNotFound sentinel,
		// or an untyped parse error from a sops store (matched on the
		// "unmarshal" stem so both "unmarshalling" and the YAML store's
		// "unmarshaling" spelling are covered). Everything else - including
		// sops.MacMismatch and a missing data key - is a real sops file that
		// could not be decrypted, and must fail hard.
		if errors.Is(err, sops.MetadataNotFound) ||
			isUnmarshalError(err) {
			log.Printf("sops decrypt %s: %v; passing through", path, err)
			return raw, nil
		}
		return nil, fmt.Errorf("sops decrypt %s: %w", path, err)
	}
	return plain, nil
}

// isUnmarshalError reports whether err is a sops store parse failure. The
// sops stores return these as untyped fmt.Errorf values whose message stems
// from the encoding library ("Error unmarshalling input yaml/json", "Could
// not unmarshal input data") - there is no typed sentinel to compare
// against, so a case-insensitive match on "unmarshal" is the most robust
// signal. It tolerates both the British ("unmarshalling") and American
// ("unmarshaling") spellings the YAML store emits across its code paths.
func isUnmarshalError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unmarshal")
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
// Checking for the colon-terminated form (sops:) avoids the most common
// false positive - CUE source code that merely mentions sops in a comment,
// which would say "sops" without a trailing colon. The match is still purely
// lexical: a string value containing "sops:" (e.g. {"note": "sops: bar"})
// trips it too. That is harmless: a false positive that is not a genuine
// sops file is passed through unchanged by sopsFilter, so the only cost is a
// wasted decrypt attempt.
//
// unencrypted_suffix is NOT reliable on its own as the "this is really a
// sops file" signal: it is one of six mutually exclusive crypt rules
// (encrypted_suffix, {en,un}encrypted_regex, {en,un}encrypted_comment_regex
// also qualify) and the sops CLI only defaults to it when the file was
// encrypted with none of the others configured. A file encrypted with
// e.g. `--encrypted-regex '^data$'` - the standard way to encrypt only the
// data/stringData fields of a Kubernetes Secret - carries no
// unencrypted_suffix key at all, so requiring it here would miss the file
// entirely: it would be loaded as-is, ciphertext and all, straight into the
// rendered manifest with no error. mac, by contrast, is not part of that
// omitempty group; it is unconditionally written to every sops file
// regardless of which crypt rule was used, so pairing it with sops: closes
// that gap.
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
	hasMac := bytes.Contains(raw, []byte(`"mac"`)) ||
		bytes.Contains(raw, []byte("mac:"))
	return hasSops && (hasSuffix || hasMac)
}
