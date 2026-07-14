package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile is a tiny helper to lay out a minimal cuegen module in dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeModule scaffolds a self-contained v2 module in dir. If exportPath
// is empty, cuegen.spec.export is omitted so the default kicks in.
func writeModule(t *testing.T, dir, exportPath string) {
	t.Helper()
	writeFile(t, dir, "cue.mod/module.cue", `module: "t.local"
language: version: "v0.17.0"
`)
	if exportPath == "" {
		writeFile(t, dir, "cuegen.cue", `package control

cuegen: {
	apiVersion: "v2"
}
`)
	} else {
		writeFile(t, dir, "cuegen.cue", `package control

cuegen: {
	apiVersion: "v2"
	spec: export: "`+exportPath+`"
}
`)
	}
}

// TestExecDefaultExportPath renders a module that omits cuegen.spec.export;
// the engine must fall back to "export.objects" and emit one YAML document
// per object, separated by "---". Exec loads via the process's current
// working directory (see the Exec doc comment), so tests chdir into the
// module rather than pass its absolute path.
func TestExecDefaultExportPath(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: {
	configMap: {
		"cm-one": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "cm-one"
			data: a: "1"
		}
		"cm-two": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "cm-two"
			data: b: "2"
		}
	}
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec(".", &out, Options{}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	got := out.String()

	// Exactly one separator between two documents, plus a trailing blank line.
	if c := strings.Count(got, "---\n"); c != 1 {
		t.Errorf("separator count = %d, want 1\n%s", c, got)
	}
	for _, want := range []string{"name: cm-one", "name: cm-two", "kind: ConfigMap", "a: \"1\"", "b: \"2\""} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output should end with newline")
	}
}

// TestExecCustomExportPath confirms cuegen.spec.export overrides the default.
func TestExecCustomExportPath(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "out.things")
	writeFile(t, dir, "export.cue", `package control

out: things: secret: s: {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: name: "s"
	data: x: "eA=="
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec(".", &out, Options{}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "name: s") || !strings.Contains(got, "kind: Secret") {
		t.Errorf("output missing expected object\n%s", got)
	}
	if strings.Contains(got, "---\n") {
		t.Errorf("single object should not contain a separator\n%s", got)
	}
}

// TestExecMissingExportPath errors when the configured path does not exist.
func TestExecMissingExportPath(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "export.nonexistent")
	writeFile(t, dir, "export.cue", `package control

export: objects: {}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	err := Exec(".", &out, Options{})
	if err == nil {
		t.Fatal("expected error for missing export path, got nil")
	}
	if !strings.Contains(err.Error(), "export.nonexistent") {
		t.Errorf("error = %q, want it to mention the missing path", err)
	}
	if out.Len() != 0 {
		t.Errorf("no output expected on error, got %q", out.String())
	}
}

// TestExecNonConcreteExport verifies that non-concrete values are caught
// before YAML encoding with a helpful error: every offending field is
// reported at once, each with its full CUE path and source position, and
// no partial output is written. A field carrying a CUE default is concrete
// for export purposes and must not be reported.
func TestExecNonConcreteExport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

$token:    string
$replicas: int

export: objects: {
	configMap: "cm-a": {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "cm-a"
		data: {
			TOKEN: $token
			LEVEL: *"info" | "debug"
		}
	}
	deployment: "dep-a": {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "dep-a"
		spec: replicas: $replicas
	}
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	err := Exec(".", &out, Options{})
	if err == nil {
		t.Fatal("expected error for non-concrete export, got nil")
	}
	msg := err.Error()
	// All holes reported in one pass, each with its full CUE path.
	for _, want := range []string{
		`export.objects.configMap."cm-a".data.TOKEN`,
		`export.objects.deployment."dep-a".spec.replicas`,
		"incomplete value",
		"export.cue:", // source position
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
	}
	// A defaulted field is concrete and must not be flagged.
	if strings.Contains(msg, "LEVEL") {
		t.Errorf("defaulted field must not be reported as non-concrete, got:\n%s", msg)
	}
	if out.Len() != 0 {
		t.Errorf("no output expected on error, got %q", out.String())
	}
}

// TestExecSubdirUnifiesWithParent pins the CUE semantics engine.Exec relies
// on: loading a subdirectory unifies its package with the same-named
// package in every ancestor directory up to the module root. dir declares
// a value hole ($value) that only "sub" fills in - the render must succeed
// and reflect sub's value, proving Exec resolved "./sub" relative to dir
// (the CWD) rather than treating sub as a self-contained module. This is
// the exact pattern examples/webapp/prod demonstrates.
func TestExecSubdirUnifiesWithParent(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

$value: string

export: objects: configMap: cm: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "cm"
	data: value: $value
}
`)
	writeFile(t, dir, "sub/export.cue", `package control

$value: "from-sub"
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec("./sub", &out, Options{}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "from-sub") {
		t.Errorf("output missing value unified from parent+sub\n%s", got)
	}
}

// TestExecJSONKeyScheme pins the FormatJSON key scheme: keys are always
// "<kind>/<name>", deliberately without the namespace - it has no value for
// cuegen's use case. Consequently two objects sharing kind and name (here:
// in different namespaces) are a hard duplicate-key error rather than being
// silently disambiguated or dropped.
func TestExecJSONKeyScheme(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: {
	"a": {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: "cm", namespace: "ns-one"}
		data: x: "1"
	}
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec(".", &out, Options{Format: FormatJSON}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if _, ok := obj["ConfigMap/cm"]; !ok || len(obj) != 1 {
		t.Errorf("want single key %q, got %v", "ConfigMap/cm", out.String())
	}
}

// TestExecJSONDuplicateKindName errors on two objects with the same kind and
// name, even in different namespaces.
func TestExecJSONDuplicateKindName(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: {
	"a": {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: "cm", namespace: "ns-one"}
		data: x: "1"
	}
	"b": {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: "cm", namespace: "ns-two"}
		data: x: "2"
	}
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	err := Exec(".", &out, Options{Format: FormatJSON})
	if err == nil {
		t.Fatal("expected duplicate-key error, got nil")
	}
	if !strings.Contains(err.Error(), `duplicate object key "ConfigMap/cm"`) {
		t.Errorf("error = %q, want it to name the duplicate key", err)
	}
}

// TestOverlayCoversModuleRoot pins the fix for a security-relevant bug: the
// overlay walk must cover the entire module tree (rooted at cue.mod), not
// just the render path argument. CUE unifies ancestor packages up to the
// module root, so a file at the root level feeds into the build even when
// rendering a subdirectory. If the filter only walked the subdirectory,
// the root-level file would reach the CUE compiler unfiltered - in the
// SOPS case that means ciphertext flowing silently into the render output.
//
// The test plants a marker-bearing file at the module root and a sub that
// references it. A filter replaces the marker. Rendering ./sub must show
// the replaced value, proving the filter ran on the root-level file.
func TestOverlayCoversModuleRoot(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	// Root-level file with a marker the filter will replace. Simulates an
	// encrypted file whose cleartext only emerges after filtering.
	writeFile(t, dir, "values.cue", `package control

$secret: "MARKER_UNFILTERED"
`)
	// Root export references $secret so the marker would appear in output
	// if the filter didn't run.
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: root: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "root"
	data: key: $secret
}
`)
	// Subdirectory that also references $secret from the parent package.
	writeFile(t, dir, "sub/export.cue", `package control

export: objects: configMap: sub: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "sub"
	data: key: $secret
}
`)

	// Filter replaces the marker wherever it appears.
	filter := func(path string, raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "MARKER_UNFILTERED", "decrypted")), nil
	}

	t.Chdir(dir)

	// Rendering the subdirectory must filter the root-level values.cue.
	var out bytes.Buffer
	if err := Exec("./sub", &out, Options{FileFilter: filter}); err != nil {
		t.Fatalf("Exec ./sub: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "MARKER_UNFILTERED") {
		t.Errorf("output contains unfiltered marker; root-level file was not filtered:\n%s", got)
	}
	if !strings.Contains(got, "decrypted") {
		t.Errorf("output missing decrypted value; filter did not run on root-level file:\n%s", got)
	}

	// Sanity: rendering "." must also filter (this worked before the fix).
	out.Reset()
	if err := Exec(".", &out, Options{FileFilter: filter}); err != nil {
		t.Fatalf("Exec .: %v", err)
	}
	if strings.Contains(out.String(), "MARKER_UNFILTERED") {
		t.Errorf("render . contains unfiltered marker:\n%s", out.String())
	}
}

// TestOverlaySymlinkAliasGetsEntry pins the fix for a bug where the global
// (dev,ino) visited set caused the second symlink path to the same file to
// be skipped entirely - no overlay entry, so CUE read the raw (encrypted)
// bytes from disk. The fix replaces the visited set with ancestor-stack
// cycle detection: each alias path gets its own overlay entry.
//
// Layout: a real directory "real/" with a filtered file, plus two symlinks
// "a/" and "b/" pointing to "real/". The filter must produce overlay entries
// for BOTH a/data.cue and b/data.cue.
func TestOverlaySymlinkAliasGetsEntry(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file whose content the filter will transform.
	dataFile := filepath.Join(realDir, "data.cue")
	if err := os.WriteFile(dataFile, []byte("MARKER_UNFILTERED"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Two symlinks to the same directory.
	if err := os.Symlink(realDir, filepath.Join(dir, "a")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.Symlink(realDir, filepath.Join(dir, "b")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	filter := func(path string, raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "MARKER_UNFILTERED", "decrypted")), nil
	}

	overlay, err := buildOverlay(dir, filter)
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}

	// Both symlink paths must have overlay entries.
	aPath := filepath.Join(dir, "a", "data.cue")
	bPath := filepath.Join(dir, "b", "data.cue")
	if _, ok := overlay[aPath]; !ok {
		t.Errorf("overlay missing entry for %s (first symlink path)", aPath)
	}
	if _, ok := overlay[bPath]; !ok {
		t.Errorf("overlay missing entry for %s (second symlink path) - "+
			"global visited set skipped the alias", bPath)
	}
}
