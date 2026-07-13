package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// --- test helpers ----------------------------------------------------------

// genAge returns a fresh age identity and a random 32-byte data encryption key.
func genAge(t *testing.T) (*age.X25519Identity, []byte) {
	t.Helper()
	ident, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate age identity: %v", err)
	}
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		t.Fatalf("rand dek: %v", err)
	}
	return ident, dek
}

// armorDEK encrypts dek to ident's recipient and returns the armored blob
// exactly as sops stores it in sops.age[].enc.
func armorDEK(t *testing.T, ident *age.X25519Identity, dek []byte) string {
	t.Helper()
	buf := &bytes.Buffer{}
	aw := armor.NewWriter(buf)
	enc, err := age.Encrypt(aw, ident.Recipient())
	if err != nil {
		t.Fatalf("age encrypt: %v", err)
	}
	if _, err := enc.Write(dek); err != nil {
		t.Fatalf("write dek: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close age writer: %v", err)
	}
	if err := aw.Close(); err != nil {
		t.Fatalf("close armor writer: %v", err)
	}
	return buf.String()
}

// sopsMeta builds the sops metadata block carrying the age-encrypted DEK.
func sopsMeta(t *testing.T, ident *age.X25519Identity, dek []byte) map[string]any {
	return map[string]any{
		"age": []map[string]any{
			{"recipient": ident.Recipient().String(), "enc": armorDEK(t, ident, dek)},
		},
		"unencrypted_suffix": "_unencrypted",
		"version":            "3.9.0",
	}
}

// encLeaf produces a sops ENC[AES256_GCM,...] string: the inverse of
// decryptValue. It mirrors the wire format so round-trips exercise the real
// AES-GCM + AAD path without depending on the external sops binary.
func encLeaf(t *testing.T, dek []byte, plain, aad, typ string) string {
	t.Helper()
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand iv: %v", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	ct := gcm.Seal(nil, iv, []byte(plain), []byte(aad))
	data, tag := ct[:len(ct)-gcm.Overhead()], ct[len(ct)-gcm.Overhead():]
	return fmt.Sprintf("ENC[AES256_GCM,data:%s,iv:%s,tag:%s,type:%s]",
		base64.StdEncoding.EncodeToString(data),
		base64.StdEncoding.EncodeToString(iv),
		base64.StdEncoding.EncodeToString(tag),
		typ,
	)
}

// withAgeKey runs fn with SOPS_AGE_KEY=key and SOPS_AGE_KEY_FILE cleared, so
// loadAgeIdentities sees only the test identity.
func withAgeKey(t *testing.T, key string, fn func()) {
	t.Helper()
	t.Setenv("SOPS_AGE_KEY", key)
	t.Setenv("SOPS_AGE_KEY_FILE", "")
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
		{"comment mention only", "// sops manages this\npackage x", false},
		{"plain text", "hello world", false},
		{"sops without suffix", `{"sops":{}}`, false},
		{"suffix without sops", `{"unencrypted_suffix":"_"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeSops([]byte(tc.raw)); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// --- typedValue ------------------------------------------------------------

func TestTypedValue(t *testing.T) {
	tests := []struct {
		plain, typ string
		want       any
	}{
		{"42", "int", int64(42)},
		{"-7", "int", int64(-7)},
		{"3.14", "float", 3.14},
		{"True", "bool", true},
		{"true", "bool", true},
		{"False", "bool", false},
		{"hello", "str", "hello"},
		{"not-a-number", "int", "not-a-number"}, // parse fail -> verbatim string
		{"oops", "unknown", "oops"},             // unknown tag -> string
		{"", "str", ""},
	}
	for _, tc := range tests {
		got := typedValue(tc.plain, tc.typ)
		switch w := tc.want.(type) {
		case int64:
			if g, ok := got.(int64); !ok || g != w {
				t.Errorf("typedValue(%q,%q)=%#v want %v", tc.plain, tc.typ, got, w)
			}
		case float64:
			if g, ok := got.(float64); !ok || g != w {
				t.Errorf("typedValue(%q,%q)=%#v want %v", tc.plain, tc.typ, got, w)
			}
		case bool:
			if g, ok := got.(bool); !ok || g != w {
				t.Errorf("typedValue(%q,%q)=%#v want %v", tc.plain, tc.typ, got, w)
			}
		case string:
			if g, ok := got.(string); !ok || g != w {
				t.Errorf("typedValue(%q,%q)=%#v want %q", tc.plain, tc.typ, got, w)
			}
		}
	}
}

// --- decryptValue round-trip ----------------------------------------------

func TestDecryptValueRoundTrip(t *testing.T) {
	dek := bytes.Repeat([]byte{0xAB}, 32)
	tests := []struct{ plain, typ, aad string }{
		{"supersecret", "str", "tokens:TOKEN:"},
		{"12345", "int", "config:port:"},
		{"1.5", "float", "limits:cpu:"},
		{"True", "bool", "flags:enabled:"},
		{"", "str", "empty:"},
	}
	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			enc := encLeaf(t, dek, tc.plain, tc.aad, tc.typ)
			plain, typ, err := decryptValue(enc, dek, tc.aad)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if plain != tc.plain || typ != tc.typ {
				t.Errorf("got (%q,%q) want (%q,%q)", plain, typ, tc.plain, tc.typ)
			}
		})
	}
}

// Wrong AAD must fail AEAD authentication - pins the AAD contract.
func TestDecryptValueWrongAADFails(t *testing.T) {
	dek := bytes.Repeat([]byte{0xCD}, 32)
	enc := encLeaf(t, dek, "secret", "a:b:", "str")
	if _, _, err := decryptValue(enc, dek, "a:c:"); err == nil {
		t.Fatal("decrypt with wrong AAD unexpectedly succeeded")
	}
}

// --- walkAndDecrypt: AAD construction (walkSlice compatibility) ----------

// TestWalkAndDecryptAAD pins the tree-path AAD convention: a leaf's AAD is the
// colon-joined map keys from root to leaf, terminated with ':'. List indices
// do NOT contribute (matching upstream sops walkSlice). Encrypted leaves are
// planted at known paths; decryption succeeding proves the walker rebuilds
// the same AAD sops used at encryption time.
func TestWalkAndDecryptAAD(t *testing.T) {
	dek := bytes.Repeat([]byte{0x11}, 32)
	tree := map[string]any{
		"tokens": map[string]any{
			"TOKEN": encLeaf(t, dek, "tok", "tokens:TOKEN:", "str"),
		},
		"list": []any{
			encLeaf(t, dek, "elem0", "list:", "str"), // index excluded -> AAD "list:"
		},
		"nested": map[string]any{
			"deep": map[string]any{
				"value": encLeaf(t, dek, "d", "nested:deep:value:", "str"),
			},
		},
	}
	out, err := walkAndDecrypt(tree, dek, nil)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	m := out.(map[string]any)
	if m["tokens"].(map[string]any)["TOKEN"] != "tok" {
		t.Errorf("tokens.TOKEN = %#v", m["tokens"])
	}
	if m["nested"].(map[string]any)["deep"].(map[string]any)["value"] != "d" {
		t.Errorf("nested.deep.value = %#v", m["nested"])
	}
	if list := m["list"].([]any); len(list) != 1 || list[0] != "elem0" {
		t.Errorf("list = %#v want [elem0]", list)
	}
}

// --- decryptSops end-to-end: native JSON ----------------------------------

func TestDecryptSopsNativeJSON(t *testing.T) {
	ident, dek := genAge(t)
	envelope := map[string]any{
		"tokens": map[string]any{
			"USER": encLeaf(t, dek, "svc-foo", "tokens:USER:", "str"),
			"PASS": encLeaf(t, dek, "s3cret", "tokens:PASS:", "str"),
		},
		"sops": sopsMeta(t, ident, dek),
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	withAgeKey(t, ident.String(), func() {
		out, err := decryptSops(raw)
		if err != nil {
			t.Fatalf("decryptSops: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("unmarshal output: %v\n%s", err, out)
		}
		if got["sops"] != nil {
			t.Errorf("sops block not stripped: %v", got["sops"])
		}
		toks := got["tokens"].(map[string]any)
		if toks["USER"] != "svc-foo" || toks["PASS"] != "s3cret" {
			t.Errorf("tokens = %#v want USER=svc-foo PASS=s3cret", toks)
		}
	})
}

// --- decryptSops end-to-end: binary store ---------------------------------

func TestDecryptSopsBinaryStore(t *testing.T) {
	ident, dek := genAge(t)
	plaintext := "apiVersion: v1\nkind: Secret\n"
	envelope := map[string]any{
		"data": encLeaf(t, dek, plaintext, "data:", "str"),
		"sops": sopsMeta(t, ident, dek),
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	withAgeKey(t, ident.String(), func() {
		out, err := decryptSops(raw)
		if err != nil {
			t.Fatalf("decryptSops: %v", err)
		}
		if string(out) != plaintext {
			t.Errorf("binary store output = %q want %q", out, plaintext)
		}
	})
}

// --- sopsFilter: hard vs soft failure -------------------------------------

// A genuine age-encrypted sops file with the wrong key configured must fail
// hard - no ciphertext ever reaches the CUE compiler or a deployment.
func TestSopsFilterHardFailureOnWrongKey(t *testing.T) {
	ident, dek := genAge(t)
	envelope := map[string]any{
		"tokens": map[string]any{"TOKEN": encLeaf(t, dek, "secret", "tokens:TOKEN:", "str")},
		"sops":   sopsMeta(t, ident, dek),
	}
	raw, _ := json.MarshalIndent(envelope, "", "  ")

	other, _ := age.GenerateX25519Identity() // a DIFFERENT identity configured
	withAgeKey(t, other.String(), func() {
		_, err := sopsFilter("secrets.enc.json", raw)
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

// A non-age envelope (sops metadata but no age recipients) passes through.
func TestSopsFilterSoftPassthroughNonAge(t *testing.T) {
	raw, _ := json.MarshalIndent(map[string]any{
		"data": "ENC[AES256_GCM,data:xx,iv:yy,tag:zz,type:str]",
		"sops": map[string]any{
			"kms":                []any{map[string]any{"arn": "arn:aws:kms:..."}},
			"unencrypted_suffix": "_unencrypted",
		},
	}, "", "  ")
	out, err := sopsFilter("kms-secret.json", raw)
	if err != nil {
		t.Fatalf("expected soft passthrough for non-age envelope, got: %v", err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("passthrough altered bytes")
	}
}

// --- loadAgeIdentities sources --------------------------------------------

func TestLoadAgeIdentitiesFromEnv(t *testing.T) {
	ident, _ := genAge(t)
	withAgeKey(t, ident.String(), func() {
		ids, err := loadAgeIdentities()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if len(ids) != 1 {
			t.Fatalf("got %d identities, want 1", len(ids))
		}
	})
}

func TestLoadAgeIdentitiesNoneConfigured(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY", "")
	t.Setenv("SOPS_AGE_KEY_FILE", "")
	// Point XDG/HOME at a temp dir so the config fallback finds nothing.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	ids, err := loadAgeIdentities()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("got %d identities, want 0", len(ids))
	}
}

// --- parseFile -------------------------------------------------------------

func TestParseFileMissing(t *testing.T) {
	err := parseFile("/nonexistent/keys.txt", func(io.Reader) error { return nil })
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
