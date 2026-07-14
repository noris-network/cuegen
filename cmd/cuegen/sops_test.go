package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

// --- test helpers ----------------------------------------------------------

// genAgeIdentity returns a fresh age identity and its public recipient string.
func genAgeIdentity(t *testing.T) (priv, pub string) {
	t.Helper()
	ident, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate age identity: %v", err)
	}
	return ident.String(), ident.Recipient().String()
}

// sopsEncrypt encrypts plaintext using the sops CLI with the given age
// recipient and input/output type. The SOPS_AGE_KEY env must NOT be set
// during encryption (only the public recipient is needed). The filenameOverride
// sets the file extension sops uses to infer the output format. Returns the
// encrypted bytes.
func sopsEncrypt(t *testing.T, plaintext []byte, recipient, inputType, outputType, filenameOverride string) []byte {
	t.Helper()
	cmd := exec.Command("sops", "encrypt", "--age", recipient,
		"--input-type", inputType, "--output-type", outputType,
		"--filename-override", filenameOverride)
	cmd.Stdin = bytes.NewReader(plaintext)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sops encrypt: %v\n%s", err, stderr.String())
	}
	return out
}

// withAgeKey runs fn with SOPS_AGE_KEY=key, so the sops package loads only
// the test identity for decryption. SOPS_AGE_KEY_FILE is unset to avoid the
// sops package trying to open an empty path. t.Setenv cannot manage
// SOPS_AGE_KEY_FILE here (sops opens the path even on an empty string), so
// the prior value is saved and restored via t.Cleanup instead of leaking the
// deletion to the rest of the test process.
func withAgeKey(t *testing.T, key string, fn func()) {
	t.Helper()
	t.Setenv("SOPS_AGE_KEY", key)
	if old, ok := os.LookupEnv("SOPS_AGE_KEY_FILE"); ok {
		t.Cleanup(func() { os.Setenv("SOPS_AGE_KEY_FILE", old) })
	} else {
		t.Cleanup(func() { os.Unsetenv("SOPS_AGE_KEY_FILE") })
	}
	os.Unsetenv("SOPS_AGE_KEY_FILE")
	fn()
}

// --- looksLikeSops ---------------------------------------------------------

func TestLooksLikeSops(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"combined token", `{"sops_unencrypted_suffix":"_"}`, true},
		{"both quoted keys", `{"sops":{},"unencrypted_suffix":"_"}`, true},
		{"yaml combined token", "sops_unencrypted_suffix: _\n", true},
		{"yaml both keys", "sops:\n    age: []\nunencrypted_suffix: _\n", true},
		{"comment mention only", "// sops manages this\npackage x", false},
		{"plain text", "hello world", false},
		{"sops without suffix", `{"sops":{}}`, false},
		{"suffix without sops", `{"unencrypted_suffix":"_"}`, false},
		{"yaml sops without suffix", "sops:\n    age: []\n", false},
		{"yaml suffix without sops", "unencrypted_suffix: _\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeSops([]byte(tc.raw)); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// --- sopsFilter end-to-end: binary store (CUE source) ----------------------

// TestSopsFilterBinaryStore encrypts a CUE source snippet as a sops binary
// store and verifies sopsFilter decrypts it back to the original plaintext.
func TestSopsFilterBinaryStore(t *testing.T) {
	priv, pub := genAgeIdentity(t)
	plaintext := []byte("package main\n\nvalue: \"hello\"\n")
	encrypted := sopsEncrypt(t, plaintext, pub, "binary", "binary", "secret.enc.cue")

	withAgeKey(t, priv, func() {
		out, err := sopsFilter("secret.enc.cue", encrypted)
		if err != nil {
			t.Fatalf("sopsFilter: %v", err)
		}
		if !bytes.Equal(out, plaintext) {
			t.Errorf("output = %q, want %q", out, plaintext)
		}
	})
}

// --- sopsFilter end-to-end: native YAML ------------------------------------

// TestSopsFilterNativeYAML encrypts a YAML file with sops and verifies
// sopsFilter decrypts it, stripping the sops metadata block.
func TestSopsFilterNativeYAML(t *testing.T) {
	priv, pub := genAgeIdentity(t)
	plaintext := []byte("tokens:\n    USER: svc-foo\n    PASS: s3cret\n")
	encrypted := sopsEncrypt(t, plaintext, pub, "yaml", "yaml", "config.enc.yaml")

	withAgeKey(t, priv, func() {
		out, err := sopsFilter("config.enc.yaml", encrypted)
		if err != nil {
			t.Fatalf("sopsFilter: %v", err)
		}
		if bytes.Contains(out, []byte("sops:")) {
			t.Errorf("sops block not stripped:\n%s", out)
		}
		if !bytes.Contains(out, []byte("svc-foo")) || !bytes.Contains(out, []byte("s3cret")) {
			t.Errorf("decrypted values missing:\n%s", out)
		}
	})
}

// TestSopsFilterNativeYMLExtension mirrors TestSopsFilterNativeYAML but uses
// the .yml extension, exercising the .yml branch of sopsFormat end-to-end
// through sopsFilter (TestSopsFormat covers the mapping in isolation only).
func TestSopsFilterNativeYMLExtension(t *testing.T) {
	priv, pub := genAgeIdentity(t)
	plaintext := []byte("tokens:\n    USER: svc-foo\n")
	encrypted := sopsEncrypt(t, plaintext, pub, "yaml", "yaml", "config.enc.yml")

	withAgeKey(t, priv, func() {
		out, err := sopsFilter("config.enc.yml", encrypted)
		if err != nil {
			t.Fatalf("sopsFilter: %v", err)
		}
		if bytes.Contains(out, []byte("sops:")) {
			t.Errorf("sops block not stripped:\n%s", out)
		}
		if !bytes.Contains(out, []byte("svc-foo")) {
			t.Errorf("decrypted value missing:\n%s", out)
		}
	})
}

// --- sopsFilter end-to-end: native JSON ------------------------------------

// TestSopsFilterNativeJSON encrypts a JSON file with sops and verifies
// sopsFilter decrypts it, stripping the sops metadata block.
func TestSopsFilterNativeJSON(t *testing.T) {
	priv, pub := genAgeIdentity(t)
	plaintext := []byte(`{"tokens":{"USER":"svc-foo","PASS":"s3cret"}}`)
	encrypted := sopsEncrypt(t, plaintext, pub, "json", "json", "config.enc.json")

	withAgeKey(t, priv, func() {
		out, err := sopsFilter("config.enc.json", encrypted)
		if err != nil {
			t.Fatalf("sopsFilter: %v", err)
		}
		if bytes.Contains(out, []byte(`"sops"`)) {
			t.Errorf("sops block not stripped:\n%s", out)
		}
		if !bytes.Contains(out, []byte("svc-foo")) || !bytes.Contains(out, []byte("s3cret")) {
			t.Errorf("decrypted values missing:\n%s", out)
		}
	})
}

// --- sopsFilter: hard vs soft failure -------------------------------------

// A genuine age-encrypted sops file with the wrong key configured must fail
// hard - no ciphertext ever reaches the CUE compiler or a deployment.
func TestSopsFilterHardFailureOnWrongKey(t *testing.T) {
	_, pub := genAgeIdentity(t)
	plaintext := []byte("tokens:\n    TOKEN: secret\n")
	encrypted := sopsEncrypt(t, plaintext, pub, "yaml", "yaml", "secrets.enc.yaml")

	otherPriv, _ := genAgeIdentity(t) // a DIFFERENT identity configured
	withAgeKey(t, otherPriv, func() {
		_, err := sopsFilter("secrets.enc.yaml", encrypted)
		if err == nil {
			t.Fatal("expected hard failure for real sops file with wrong key, got nil")
		}
		if !strings.Contains(err.Error(), "sops decrypt") {
			t.Errorf("error = %q, want it to mention sops decrypt", err)
		}
	})
}

// A heuristic false positive (markers present, not valid sops) passes through.
func TestSopsFilterSoftPassthroughOnFalsePositive(t *testing.T) {
	raw := []byte(`{"sops":"mentioned","unencrypted_suffix":"_","note":"not sops"}`)
	out, err := sopsFilter("notes.json", raw)
	if err != nil {
		t.Fatalf("expected soft passthrough, got error: %v", err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("passthrough altered bytes")
	}
}

// --- sopsFormat ------------------------------------------------------------

func TestSopsFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"secret.enc.cue", "binary"},
		{"config.enc.yaml", "yaml"},
		{"config.enc.yml", "yaml"},
		{"config.enc.json", "json"},
		{"data.txt", "binary"},
		{"plain", "binary"},
	}
	for _, tc := range tests {
		if got := sopsFormat(tc.path); got != tc.want {
			t.Errorf("sopsFormat(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// --- sopsFilter: real example files ----------------------------------------

// TestSopsFilterExampleFiles verifies the example sops files under examples/sops
// decrypt correctly with the demo age key.
func TestSopsFilterExampleFiles(t *testing.T) {
	demoKey := "AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ"

	files := []struct {
		name   string
		path   string
		expect string // substring that must appear in the decrypted output
	}{
		{"secret.enc.cue", "../../examples/sops/secret.enc.cue", "password"},
		{"config.enc.yaml", "../../examples/sops/config.enc.yaml", "db.example.com"},
	}
	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.FromSlash(f.path))
			if err != nil {
				t.Skipf("example file not found: %v", err)
			}
			withAgeKey(t, demoKey, func() {
				out, err := sopsFilter(f.path, raw)
				if err != nil {
					t.Fatalf("sopsFilter %s: %v", f.name, err)
				}
				if !bytes.Contains(out, []byte(f.expect)) {
					t.Errorf("decrypted output missing %q:\n%s", f.expect, out)
				}
			})
		})
	}
}
