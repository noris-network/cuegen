package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// errNotAgeSops marks a detection miss: the bytes tripped the [looksLikeSops]
// heuristic but turned out not to be an age-encrypted sops file we handle -
// either a heuristic false positive (the quoted markers appeared by chance) or
// a non-age envelope (KMS/PGP). Such files are passed through unchanged. A
// decrypt failure on a *genuine* age-encrypted sops file is NOT wrapped with
// this sentinel and therefore fails hard (see [sopsFilter]).
var errNotAgeSops = errors.New("not an age-encrypted sops file")

// notAgeSops wraps reason (and optionally err) in errNotAgeSops so callers can
// distinguish a detection miss from a real decrypt failure via errors.Is.
func notAgeSops(reason string, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errNotAgeSops, reason, err)
	}
	return fmt.Errorf("%w: %s", errNotAgeSops, reason)
}

// sopsFilter is the engine.FileFilter that transparently decrypts
// sops-encrypted files using age recipients only.
//
// Detection is byte-level: a file must contain "sops" and "unencrypted_suffix"
// (or the combined token "sops_unencrypted_suffix") to be considered. On a
// hit, we attempt to decrypt. Two outcomes are distinguished:
//
//   - Detection miss ([errNotAgeSops]): the heuristic fired on a non-sops
//     file or a non-age envelope (KMS/PGP). We warn to stderr and pass the raw
//     bytes through unchanged so CUE compiles the file as-is.
//   - Genuine decrypt failure: a real age-encrypted sops file that could not
//     be decrypted (missing/rotated key, corrupt ciphertext, …). This fails
//     HARD - returning the error aborts the render so ciphertext never
//     reaches the CUE compiler or a downstream deployment.
//
// Only age recipients are supported.
//
// Supported file shapes:
//   - sops binary store: a JSON envelope with top-level "data" + "sops" keys.
//     The clear-text "data" is returned verbatim (raw bytes).
//   - sops native JSON: any other JSON document with a top-level "sops"
//     metadata block. Every "ENC[AES256_GCM,...]" leaf is decrypted in place
//     using its tree-path AAD; the "sops" block is stripped from the output.
//
// Native YAML files are intentionally not supported: cuegen's inputs are CUE
// sources and binary blobs, both of which sops encrypts as the binary store.
// Add YAML support if a real need shows up.
func sopsFilter(path string, raw []byte) ([]byte, error) {
	if !looksLikeSops(raw) {
		return raw, nil
	}
	plain, err := decryptSops(raw)
	if err != nil {
		if errors.Is(err, errNotAgeSops) {
			fmt.Fprintf(os.Stderr, "cuegen: sops decrypt %s: %v; passing through\n", path, err)
			return raw, nil
		}
		return nil, fmt.Errorf("sops decrypt %s: %w", path, err)
	}
	return plain, nil
}

// looksLikeSops is a cheap pre-filter that decides whether a file is worth
// running through the JSON-based decrypt path. Sops files we accept are
// always JSON, so we require the canonical *quoted* keys "sops" and either
// "unencrypted_suffix" or the combined token "sops_unencrypted_suffix". This
// avoids false positives when CUE source code merely mentions sops in a
// comment.
func looksLikeSops(raw []byte) bool {
	if bytes.Contains(raw, []byte(`"sops_unencrypted_suffix"`)) {
		return true
	}
	return bytes.Contains(raw, []byte(`"sops"`)) &&
		bytes.Contains(raw, []byte(`"unencrypted_suffix"`))
}

// sopsEnvelope captures only the fields we need from a sops file.
type sopsEnvelope struct {
	Sops *sopsMetadata `json:"sops"`
}

type sopsMetadata struct {
	Age []sopsAgeRecipient `json:"age"`
}

type sopsAgeRecipient struct {
	Recipient string `json:"recipient"`
	Enc       string `json:"enc"`
}

func decryptSops(raw []byte) ([]byte, error) {
	// Parse twice: once to grab the sops metadata in a typed shape, once as
	// generic JSON so we can walk every leaf without losing fields.
	var meta sopsEnvelope
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, notAgeSops("parse sops envelope", err)
	}
	if meta.Sops == nil {
		return nil, notAgeSops("no sops metadata block", nil)
	}
	if len(meta.Sops.Age) == 0 {
		return nil, notAgeSops("no age recipients in sops metadata", nil)
	}

	dek, err := extractDEK(meta.Sops.Age)
	if err != nil {
		return nil, err
	}

	// Re-parse as generic JSON for the tree walk.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse sops body: %w", err)
	}

	// Binary store: top-level "data" is the encrypted blob, AAD is "data:",
	// the decrypted bytes are returned verbatim.
	if dataRaw, ok := root["data"]; ok && len(root) <= 2 /* data + sops */ {
		var encStr string
		if err := json.Unmarshal(dataRaw, &encStr); err == nil &&
			strings.HasPrefix(encStr, "ENC[") {
			plain, _, err := decryptValue(encStr, dek, "data:")
			if err != nil {
				return nil, fmt.Errorf("decrypt binary data: %w", err)
			}
			return []byte(plain), nil
		}
	}

	// Native JSON: walk the tree, decrypt every ENC[...] leaf, strip "sops".
	delete(root, "sops")
	walked := make(map[string]any, len(root))
	for k, v := range root {
		var node any
		if err := json.Unmarshal(v, &node); err != nil {
			return nil, fmt.Errorf("unmarshal %q: %w", k, err)
		}
		out, err := walkAndDecrypt(node, dek, []string{k})
		if err != nil {
			return nil, err
		}
		walked[k] = out
	}
	return json.MarshalIndent(walked, "", "  ")
}

// extractDEK takes the sops age recipients and returns the 32-byte data
// encryption key recovered with the first identity that succeeds.
func extractDEK(recipients []sopsAgeRecipient) ([]byte, error) {
	identities, err := loadAgeIdentities()
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf("no age identities found (set SOPS_AGE_KEY or SOPS_AGE_KEY_FILE)")
	}
	var lastErr error
	for _, r := range recipients {
		out, err := age.Decrypt(armor.NewReader(strings.NewReader(r.Enc)), identities...)
		if err != nil {
			lastErr = err
			continue
		}
		dek, err := io.ReadAll(out)
		if err != nil {
			lastErr = err
			continue
		}
		// The sops DEK is always an AES-256 key; anything else would only
		// surface later as an opaque aes.NewCipher error.
		if len(dek) != 32 {
			lastErr = fmt.Errorf("decrypted DEK has %d bytes, want 32", len(dek))
			continue
		}
		return dek, nil
	}
	return nil, fmt.Errorf("no age identity could decrypt any recipient: %w", lastErr)
}

// loadAgeIdentities reads identities from, in order:
//   - SOPS_AGE_KEY (literal, possibly newline-separated keys)
//   - SOPS_AGE_KEY_FILE (path to a keys.txt)
//   - $XDG_CONFIG_HOME/sops/age/keys.txt (or $HOME/.config/sops/age/keys.txt)
func loadAgeIdentities() ([]age.Identity, error) {
	var all []age.Identity
	parse := func(r io.Reader) error {
		ids, err := age.ParseIdentities(r)
		if err != nil {
			return fmt.Errorf("parse age identities: %w", err)
		}
		all = append(all, ids...)
		return nil
	}

	if k := os.Getenv("SOPS_AGE_KEY"); k != "" {
		if err := parse(strings.NewReader(k)); err != nil {
			return nil, err
		}
	}
	if f := os.Getenv("SOPS_AGE_KEY_FILE"); f != "" {
		if err := parseFile(f, parse); err != nil {
			return nil, fmt.Errorf("SOPS_AGE_KEY_FILE %s: %w", f, err)
		}
	}
	if len(all) == 0 {
		// os.UserConfigDir resolves $XDG_CONFIG_HOME with a $HOME/.config
		// fallback - exactly sops' default keys.txt location. Best-effort:
		// an unresolvable config dir or missing file is not an error.
		if cfg, err := os.UserConfigDir(); err == nil {
			p := filepath.Join(cfg, "sops", "age", "keys.txt")
			if _, err := os.Stat(p); err == nil {
				if err := parseFile(p, parse); err != nil {
					return nil, fmt.Errorf("%s: %w", p, err)
				}
			}
		}
	}
	return all, nil
}

// parseFile opens path, hands the *os.File to fn, and closes it on return.
// Streaming is enough: age.ParseIdentities reads line-oriented input.
func parseFile(path string, fn func(io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

// decryptValue parses a sops ENC[AES256_GCM,...] string and returns the
// plaintext together with the recorded type tag ("str", "int", "bool", …).
// Values with a different algorithm tag (e.g. AES256_CBC) fail up front
// with a clear error instead of misparsing into an AEAD failure.
func decryptValue(encStr string, dek []byte, aad string) (string, string, error) {
	inner, ok := strings.CutPrefix(encStr, "ENC[AES256_GCM,")
	if !ok {
		return "", "", fmt.Errorf("unsupported sops value %.40q: want ENC[AES256_GCM,...]", encStr)
	}
	inner, ok = strings.CutSuffix(inner, "]")
	if !ok {
		return "", "", fmt.Errorf("malformed sops value %.40q: missing closing bracket", encStr)
	}
	fields := map[string]string{}
	for kv := range strings.SplitSeq(inner, ",") {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			continue
		}
		fields[k] = v
	}
	data, err := base64.StdEncoding.DecodeString(fields["data"])
	if err != nil {
		return "", "", fmt.Errorf("decode data: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(fields["iv"])
	if err != nil {
		return "", "", fmt.Errorf("decode iv: %w", err)
	}
	tag, err := base64.StdEncoding.DecodeString(fields["tag"])
	if err != nil {
		return "", "", fmt.Errorf("decode tag: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", "", err
	}
	aesgcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return "", "", err
	}
	plain, err := aesgcm.Open(nil, iv, slices.Concat(data, tag), []byte(aad))
	if err != nil {
		return "", "", fmt.Errorf("aes-gcm open: %w", err)
	}
	return string(plain), fields["type"], nil
}

// walkAndDecrypt walks a parsed JSON tree (map/slice/scalar), decrypting
// every ENC[...] string leaf with its tree-path AAD. Non-encrypted values
// pass through unchanged. List indices do not contribute to the AAD -
// upstream sops uses the path *up to* the list, matching its own
// `walkSlice` which does not extend `path` per element.
func walkAndDecrypt(node any, dek []byte, path []string) (any, error) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			out, err := walkAndDecrypt(val, dek, append(slices.Clone(path), key))
			if err != nil {
				return nil, err
			}
			v[key] = out
		}
		return v, nil
	case []any:
		for i, val := range v {
			out, err := walkAndDecrypt(val, dek, path)
			if err != nil {
				return nil, err
			}
			v[i] = out
		}
		return v, nil
	case string:
		if !strings.HasPrefix(v, "ENC[") {
			return v, nil
		}
		aad := strings.Join(path, ":") + ":"
		plain, typ, err := decryptValue(v, dek, aad)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s: %w", aad, err)
		}
		return typedValue(plain, typ), nil
	default:
		return v, nil
	}
}

// typedValue converts a decrypted string back to its original Go type so the
// re-serialized JSON keeps numbers as numbers, bools as bools, etc. A parse
// failure (e.g. a "str"-tagged value that happens to contain digits) is
// intentional: we swallow the error and fall through to returning the raw
// string, preserving the value verbatim rather than dropping it.
func typedValue(plain, typ string) any {
	switch typ {
	case "int":
		if n, err := strconv.ParseInt(plain, 10, 64); err == nil {
			return n
		}
	case "float":
		if f, err := strconv.ParseFloat(plain, 64); err == nil {
			return f
		}
	case "bool":
		return plain == "True" || plain == "true"
	}
	return plain
}
