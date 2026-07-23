package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
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

// TestExecDropsIncompleteDynamicKey pins the fix for a silent-drop bug: an
// object whose dynamic key (here metadata.name derived from an unset
// optional value via an opaque $val) is non-concrete is never yielded by
// Fields(), so flattenObjects drops it without a peep - no error, exit 0.
// Before the fix cuegen rendered the concrete sibling (plain-cm) and silently
// omitted the AWX-like object, hiding a problem `cue export` reports loudly.
// Now requireComplete force-evaluates the whole export.objects struct, so the
// incomplete dynamic key becomes a hard, located error - matching cue export.
//
// State B (prefix unset): Exec errors, naming the dynamic-key failure and a
// source position; no partial output. State A (prefix: "") renders both.
func TestExecDropsIncompleteDynamicKey(t *testing.T) {
	for _, tc := range []struct {
		name      string
		setPrefix bool
		wantErr   bool
	}{
		{"state_b_prefix_unset", false, true},
		{"state_a_prefix_set", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeModule(t, dir, "")
			prefixLine := "// prefix: \"\"   // unset -> incomplete"
			if tc.setPrefix {
				prefixLine = "prefix: \"\""
			}
			writeFile(t, dir, "export.cue", `package control

// $val is the opaque injection point (like libmcs' $val: _). prefix is an
// OPTIONAL value the author forgot to set in state B.
$val: {
	`+prefixLine+`
	awx_operator: prefix: "awx-operator-"
}

// fullPrefix depends on $val.prefix. Unset prefix -> incomplete (NOT a CUE
// error by itself; that is the basis of *-defaults and overlays).
#fullPrefix: $val.prefix

#raw: {
	awx: {
		apiVersion: "awx.ansible.com/v1beta1"
		kind:       "AWX"
		metadata: name: #fullPrefix + "awx-instance"
		spec: {}
	}
	cm: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "plain-cm"
		data: x: "1"
	}
}

// Group by kind, then key each kind's struct by metadata.name - the same
// dynamic-key shape libmcs' export.cue uses. An incomplete metadata.name
// makes the dynamic key non-concrete.
export: objects: {
	for kind, objs in #groupByKind {
		"\(kind)": {
			for _, obj in objs {
				"\(obj.metadata.name)": obj
			}
		}
	}
}
#groupByKind: {
	for _, obj in #raw {
		let k = obj.kind
		"\(k)": {
			"\(obj.metadata.name)": obj
		}
	}
}
`)

			t.Chdir(dir)
			var out bytes.Buffer
			err := Exec(".", &out, Options{})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for incomplete dynamic key, got nil")
				}
				msg := err.Error()
				for _, want := range []string{
					"key value of dynamic field must be concrete",
					"export.cue:", // source position
				} {
					if !strings.Contains(msg, want) {
						t.Errorf("error should contain %q, got:\n%s", want, msg)
					}
				}
				if out.Len() != 0 {
					t.Errorf("no output expected on error, got %q", out.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("Exec: %v", err)
			}
			got := out.String()
			if !strings.Contains(got, "kind: AWX") {
				t.Errorf("state A output missing the AWX object (silent drop?)\n%s", got)
			}
			if !strings.Contains(got, "name: plain-cm") {
				t.Errorf("state A output missing the ConfigMap\n%s", got)
			}
		})
	}
}

// TestExecBuildErrorFullDiagnosis pins the fix for a usability bug: a CUE
// validation failure (here an enum/disjunction violation) was reported as a
// single truncated headline ("4 errors in empty disjunction: (and 4 more
// errors)") via err.Error(), discarding the conflicting values and source
// positions that `cue eval` prints. cuegen now renders the full multi-line
// diagnosis, so the bad value, the allowed values, and the file:line are all
// visible - matching cue eval on the same configuration.
func TestExecBuildErrorFullDiagnosis(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "values.cue", `package control

$ctx: {
	environment: "dev" | "prod" | "qsu" | "test"
	environment: "prodx"
}
`)
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: cm: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "cm"
	data: env: $ctx.environment
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	err := Exec(".", &out, Options{})
	if err == nil {
		t.Fatal("expected build error for invalid enum value, got nil")
	}
	msg := err.Error()
	// The full diagnosis must name the bad value, every allowed value (as
	// conflict partners), and a source position - not the truncated headline.
	if strings.Contains(msg, "(and 4 more errors)") {
		t.Errorf("error is still truncated to a headline:\n%s", msg)
	}
	for _, want := range []string{
		`conflicting values "dev" and "prodx"`,
		`conflicting values "prod" and "prodx"`,
		`conflicting values "qsu" and "prodx"`,
		`conflicting values "test" and "prodx"`,
		"values.cue:", // source position
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
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
	if !strings.Contains(err.Error(), "renders fine without -json") {
		t.Errorf("error = %q, want it to note the module still renders as YAML/KYAML", err)
	}

	if err := Exec(".", &out, Options{}); err != nil {
		t.Errorf("YAML render should succeed for the same module: %v", err)
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

// TestOverlaySkipsDanglingSymlink verifies a dangling symlink does not
// abort the overlay walk. Since the walk now covers the full module tree
// (including cue.mod/pkg vendoring), stray symlinks are common; CUE would
// ignore a dead link, so the walk must too.
func TestOverlaySkipsDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	// A real file the filter will transform.
	if err := os.WriteFile(filepath.Join(dir, "data.cue"), []byte("MARKER"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A dangling symlink pointing nowhere.
	if err := os.Symlink(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "dangling")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	filter := func(path string, raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "MARKER", "decrypted")), nil
	}

	overlay, err := buildOverlay(dir, filter)
	if err != nil {
		t.Fatalf("buildOverlay should skip dangling symlink, got error: %v", err)
	}
	if _, ok := overlay[filepath.Join(dir, "data.cue")]; !ok {
		t.Errorf("overlay missing entry for data.cue (dangling symlink aborted the walk?)")
	}
}

// TestOverlaySkipsLargeFile verifies files exceeding maxFilterFileSize are
// not read into memory. The module-wide walk can encounter multi-GB blobs;
// loading them wholesale would risk exhausting memory. A small file with
// the same marker must still get its overlay entry, proving the large file
// was skipped, not fatal.
func TestOverlaySkipsLargeFile(t *testing.T) {
	dir := t.TempDir()

	// Small file the filter will transform.
	if err := os.WriteFile(filepath.Join(dir, "small.cue"), []byte("MARKER"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Large file just over the limit. We don't write maxFilterFileSize bytes
	// to disk; instead we truncate a sparse file so the stat reports the
	// size without consuming disk or memory.
	largePath := filepath.Join(dir, "blob.bin")
	f, err := os.Create(largePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxFilterFileSize + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()

	filter := func(path string, raw []byte) ([]byte, error) {
		if strings.Contains(path, "blob.bin") {
			t.Error("filter called on oversized file")
		}
		return []byte(strings.ReplaceAll(string(raw), "MARKER", "decrypted")), nil
	}

	overlay, err := buildOverlay(dir, filter)
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	if _, ok := overlay[filepath.Join(dir, "small.cue")]; !ok {
		t.Errorf("overlay missing entry for small.cue")
	}
	if _, ok := overlay[largePath]; ok {
		t.Errorf("oversized file should not have an overlay entry")
	}
}

// TestOverlaySkipsVendoringDirs verifies the cue.mod/pkg, cue.mod/gen, and
// cue.mod/usr vendoring directories are skipped. Since CUE 0.17, dependencies
// are resolved via local-module.cue rewrites, not the legacy vendoring trees.
func TestOverlaySkipsVendoringDirs(t *testing.T) {
	dir := t.TempDir()

	// A normal CUE file the filter will transform.
	writeFile(t, dir, "export.cue", "MARKER")

	// Vendoring directories with filterable files that must NOT be reached.
	for _, sub := range []string{"pkg", "gen", "usr"} {
		writeFile(t, dir, filepath.Join("cue.mod", sub, "dep.cue"), "VENDOR_MARKER")
	}

	// A non-vendoring cue.mod subdirectory should still be walked.
	writeFile(t, dir, filepath.Join("cue.mod", "module.cue"), "MODULE_MARKER")

	filter := func(path string, raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "MARKER", "done")), nil
	}

	overlay, err := buildOverlay(dir, filter)
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}

	// Normal file must be filtered.
	if _, ok := overlay[filepath.Join(dir, "export.cue")]; !ok {
		t.Errorf("overlay missing entry for export.cue")
	}

	// cue.mod/module.cue must be filtered (not a vendoring dir).
	if _, ok := overlay[filepath.Join(dir, "cue.mod", "module.cue")]; !ok {
		t.Errorf("overlay missing entry for cue.mod/module.cue (should not skip non-vendoring cue.mod subdirs)")
	}

	// Vendoring dirs must NOT have entries.
	for _, sub := range []string{"pkg", "gen", "usr"} {
		if _, ok := overlay[filepath.Join(dir, "cue.mod", sub, "dep.cue")]; ok {
			t.Errorf("overlay should not contain cue.mod/%s/dep.cue (vendoring dir skipped)", sub)
		}
	}
}

// TestOverlaySkipsNonRegularFiles verifies that non-regular files (FIFOs,
// sockets, devices) are skipped. os.ReadFile on a FIFO blocks until a writer
// connects, which would hang the render. A timeout guard ensures the walk
// returns promptly even if the FIFO is opened.
func TestOverlaySkipsNonRegularFiles(t *testing.T) {
	dir := t.TempDir()

	// A regular file the filter will transform.
	if err := os.WriteFile(filepath.Join(dir, "data.cue"), []byte("MARKER"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A FIFO (named pipe) - os.ReadFile would block on this.
	fifoPath := filepath.Join(dir, "pipe.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o644); err != nil {
		t.Skipf("mkfifo not supported: %v", err)
	}

	filter := func(path string, raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "MARKER", "decrypted")), nil
	}

	done := make(chan struct{})
	var overlay map[string]load.Source
	var walkErr error
	go func() {
		overlay, walkErr = buildOverlay(dir, filter)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("buildOverlay hung on FIFO (non-regular file not skipped)")
	}
	if walkErr != nil {
		t.Fatalf("buildOverlay: %v", walkErr)
	}
	if _, ok := overlay[filepath.Join(dir, "data.cue")]; !ok {
		t.Errorf("overlay missing entry for data.cue")
	}
	if _, ok := overlay[fifoPath]; ok {
		t.Errorf("FIFO should not have an overlay entry")
	}
}

// TestOverlaySkipsSymlinkEscapingModuleRoot pins the containment guard: a
// symlink inside the module pointing at a directory OUTSIDE the module root
// must not be descended into. Without the guard the module-wide overlay walk
// would traverse the host filesystem (FS traversal / local DoS), reading
// arbitrary files it has no business touching - especially relevant in the
// ArgoCD CMP context where the rendered tree originates from a repo. Such a
// link is useless to CUE, so skipping it loses nothing.
func TestOverlaySkipsSymlinkEscapingModuleRoot(t *testing.T) {
	dir := t.TempDir()

	// A normal file inside the module the filter will transform.
	if err := os.WriteFile(filepath.Join(dir, "data.cue"), []byte("MARKER"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A directory OUTSIDE the module root with a file that must never be read.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "external.cue"),
		[]byte("EXTERNAL_SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	filter := func(path string, raw []byte) ([]byte, error) {
		if strings.Contains(path, "external.cue") {
			t.Errorf("filter called on file outside module root: %s", path)
		}
		return []byte(strings.ReplaceAll(string(raw), "MARKER", "decrypted")), nil
	}

	overlay, err := buildOverlay(dir, filter)
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	if _, ok := overlay[filepath.Join(dir, "data.cue")]; !ok {
		t.Errorf("overlay missing entry for data.cue")
	}
	if _, ok := overlay[filepath.Join(dir, "escape", "external.cue")]; ok {
		t.Errorf("overlay must not contain the escaped file")
	}
}

// TestOverlayFollowsInTreeSymlink complements the escape test: a symlink
// whose target stays WITHIN the module root is legitimate (module trees use
// symlinks heavily) and must still produce an overlay entry for the aliased
// path.
func TestOverlayFollowsInTreeSymlink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "data.cue"), []byte("MARKER"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, filepath.Join(dir, "alias")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	filter := func(path string, raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "MARKER", "decrypted")), nil
	}

	overlay, err := buildOverlay(dir, filter)
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	if _, ok := overlay[filepath.Join(dir, "alias", "data.cue")]; !ok {
		t.Errorf("overlay missing entry for in-tree symlink alias data.cue")
	}
}

// TestOverlayVisitCap verifies the maxOverlayVisits cap prevents runaway
// walks from symlink DAG explosions. Without a global visited set (removed
// so each alias path gets its own overlay entry), a nested symlink DAG can
// produce exponentially many paths while each individual chain stays within
// the depth budget. The cap turns that into a clear error.
//
// The real cap (1 M) is too large for a test; instead we temporarily lower
// it and create enough directories to trigger the guard.
func TestOverlayVisitCap(t *testing.T) {
	dir := t.TempDir()

	origMax := maxOverlayVisits
	maxOverlayVisits = 10
	t.Cleanup(func() { maxOverlayVisits = origMax })

	// Create 20 directories, each with a file - enough to exceed 10 visits.
	for i := 0; i < 20; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("d%d", i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "f.cue"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	filter := func(path string, raw []byte) ([]byte, error) { return raw, nil }

	_, err := buildOverlay(dir, filter)
	if err == nil {
		t.Fatal("expected error from visit cap, got nil")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("error should mention cap exceeded, got: %v", err)
	}
}

// TestExecEmptyExport errors when the export path exists but contains no
// objects, in every output format. A silent zero-document stream with exit 0
// is indistinguishable from "nothing to render" for a caller like ArgoCD,
// which would prune the entire Application - the same failure mode as the
// incomplete-dynamic-key silent drop (TestExecDropsIncompleteDynamicKey),
// one level up.
func TestExecEmptyExport(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatKYAML, FormatJSON} {
		t.Run(format.String(), func(t *testing.T) {
			dir := t.TempDir()
			writeModule(t, dir, "")
			writeFile(t, dir, "export.cue", `package control

export: objects: {}
`)

			t.Chdir(dir)
			var out bytes.Buffer
			err := Exec(".", &out, Options{Format: format})
			if err == nil {
				t.Fatal("expected error for empty export, got nil")
			}
			if !strings.Contains(err.Error(), "no objects") {
				t.Errorf("error = %q, want it to mention no objects", err)
			}
			if out.Len() != 0 {
				t.Errorf("no output expected on error, got %q", out.String())
			}
		})
	}
}

// TestExecNilFilterDoesNotCrash verifies that Exec with a nil FileFilter
// skips the overlay walk entirely and renders normally - buildOverlay must
// not be called (or at least must not crash) with a nil filter.
func TestExecNilFilterDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: cm: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "cm"
	data: x: "1"
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec(".", &out, Options{FileFilter: nil}); err != nil {
		t.Fatalf("Exec with nil filter: %v", err)
	}
	if !strings.Contains(out.String(), "name: cm") {
		t.Errorf("output missing the rendered object:\n%s", out.String())
	}
}

// failingWriter is an io.Writer that accepts the first failAfter bytes, then
// returns an error on every subsequent write. Used to provoke a flush/write
// error from writeYaml's encoder.Close path.
type failingWriter struct {
	written   int
	failAfter int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	n := len(p)
	if w.written+n > w.failAfter {
		n = w.failAfter - w.written
		if n > 0 {
			w.written += n
		}
		return n, fmt.Errorf("simulated I/O error")
	}
	w.written += n
	return n, nil
}

// TestWriteYamlFlushError verifies that a write failure during YAML encoding
// surfaces as an error from Exec rather than being silently swallowed by the
// encoder. This covers the explicit encoder.Close error-check in writeYaml.
func TestWriteYamlFlushError(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: cm: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "cm"
	data: x: "1"
}
`)

	t.Chdir(dir)
	errW := &failingWriter{failAfter: 10}
	err := Exec(".", errW, Options{})
	if err == nil {
		t.Fatal("expected error from failing writer, got nil")
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error should mention yaml encoding, got: %v", err)
	}
}

// --- unit tests for the leaf helpers ----------------------------------------

// TestFlattenObjectsRejectsNonStruct verifies flattenObjects errors when the
// export path holds something other than the expected two-level kind->name
// struct (a list, scalar, or null). The error path was previously exercised
// only indirectly through a full render.
func TestFlattenObjectsRejectsNonStruct(t *testing.T) {
	ctx := cuecontext.New()
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"list", `[1, 2, 3]`},
		{"scalar", `"s"`},
		{"null", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var v cue.Value = ctx.CompileString(tc.src)
			if v.Err() != nil {
				t.Fatalf("compile %s: %v", tc.name, v.Err())
			}
			if _, err := flattenObjects(v); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

// TestExportPathDefaultsAndValidation covers exportPath in isolation: the
// default kicks in when cuegen.spec.export is absent, and a non-string value
// is rejected rather than silently coerced.
func TestExportPathDefaultsAndValidation(t *testing.T) {
	ctx := cuecontext.New()

	t.Run("defaults when absent", func(t *testing.T) {
		v := ctx.CompileString(`{}`)
		if v.Err() != nil {
			t.Fatal(v.Err())
		}
		got, err := exportPath(v)
		if err != nil || got != defaultExportPath {
			t.Fatalf("got %q err %v, want %q", got, err, defaultExportPath)
		}
	})

	t.Run("non-string errors", func(t *testing.T) {
		v := ctx.CompileString(`cuegen: spec: export: 42`)
		if v.Err() != nil {
			t.Fatal(v.Err())
		}
		if _, err := exportPath(v); err == nil {
			t.Fatal("expected error for non-string export path, got nil")
		}
	})

	t.Run("explicit value honored", func(t *testing.T) {
		v := ctx.CompileString(`cuegen: spec: export: "out.things"`)
		if v.Err() != nil {
			t.Fatal(v.Err())
		}
		got, err := exportPath(v)
		if err != nil || got != "out.things" {
			t.Fatalf("got %q err %v, want %q", got, err, "out.things")
		}
	})
}

// TestExecKYAMLFlowStyle pins the KYAML output shape directly (flow style,
// double-quoted strings) rather than relying on the indirect -hash -kyaml
// digest equivalence tested in the cmd package.
func TestExecKYAMLFlowStyle(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: cm: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "cm"
	data: greeting: "hi"
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec(".", &out, Options{Format: FormatKYAML}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	got := out.String()
	for _, want := range []string{`kind: "ConfigMap"`, `greeting: "hi"`, `name: "cm"`} {
		if !strings.Contains(got, want) {
			t.Errorf("kyaml output missing %q\n%s", want, got)
		}
	}
}

// TestWriteJSONShape verifies writeJSON produces the documented key scheme
// ("<kind>/<name>"), a key line indented exactly two spaces, and valid JSON.
// It guards the refactor that replaced the manual bytes.ReplaceAll
// indentation with a single json.Indent pass: the output shape (key at two
// spaces, inner-object properties at four) must be preserved.
func TestWriteJSONShape(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

export: objects: configMap: cm: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "cm"
	data: greeting: "hi"
}
`)

	t.Chdir(dir)
	var out bytes.Buffer
	if err := Exec(".", &out, Options{Format: FormatJSON}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// Must be valid JSON with the expected single key.
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if _, ok := obj["ConfigMap/cm"]; !ok || len(obj) != 1 {
		t.Fatalf("want single key ConfigMap/cm, got %v", obj)
	}

	// The key line must be indented exactly two spaces (aligned under the
	// opening brace), and the inner object's first property exactly four -
	// two for the key column plus two for the object's own indent. This is
	// the shape both the old ReplaceAll path and the new single-pass
	// json.Indent produce; pinning it catches an indentation regression.
	lines := strings.Split(out.String(), "\n")
	keyLine := `  "ConfigMap/cm": {`
	firstProp := `    "apiVersion": "v1",`
	if !slicesContains(lines, keyLine) {
		t.Errorf("missing key line %q (wrong key-column indent?)\n%s", keyLine, out.String())
	}
	if !slicesContains(lines, firstProp) {
		t.Errorf("missing inner property %q (wrong object-body indent?)\n%s", firstProp, out.String())
	}
}

// slicesContains reports whether ss contains s, a tiny local helper so the
// test avoids importing slices solely for a membership check.
func slicesContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// TestExamplesRenderDeterministically renders each runnable module under
// examples/ twice in independent Exec calls and diffs the bytes, in every
// format. This is the "real module" counterpart to
// TestExecDeterministicAcrossRepeatedRenders: synthetic fixtures are
// designed to expose nondeterminism, but the modules that actually ship in
// the repo are what a regression would actually hit. examples/sops is
// excluded - it needs an age key set up in the environment, which is
// SOPS-specific test plumbing, not a determinism concern.
func TestExamplesRenderDeterministically(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// (chdir, path) pairs - cuegen.cue is always read from chdir (see Exec's
	// doc comment), so a subdirectory like webapp/prod that has no
	// cuegen.cue of its own must be reached as path "./prod" from its
	// parent, exactly like the CLI examples in the README.
	modules := []struct{ chdir, path string }{
		{"minimal", "."},
		{"webapp", "./prod"},
		{"webapp", "./dev"},
	}
	for _, m := range modules {
		t.Run(m.chdir+"/"+m.path, func(t *testing.T) {
			dir := filepath.Join(repoRoot, "examples", filepath.FromSlash(m.chdir))
			if _, err := os.Stat(dir); err != nil {
				t.Skipf("example module not found: %v", err)
			}
			t.Chdir(dir)
			for _, format := range []Format{FormatYAML, FormatKYAML, FormatJSON} {
				t.Run(format.String(), func(t *testing.T) {
					var first, second bytes.Buffer
					if err := Exec(m.path, &first, Options{Format: format}); err != nil {
						t.Fatalf("render 1: %v", err)
					}
					if err := Exec(m.path, &second, Options{Format: format}); err != nil {
						t.Fatalf("render 2: %v", err)
					}
					if first.String() != second.String() {
						t.Fatalf("render 1 differs from render 2:\n--- render 1 ---\n%s\n--- render 2 ---\n%s", first.String(), second.String())
					}
				})
			}
		})
	}
}

// TestExecDeterministicAcrossRepeatedRenders guards against the classic
// failure mode of a tool like this: nondeterminism from Go map iteration
// leaking into the output (e.g. via a CUE comprehension over a struct,
// which internally has arc/map machinery of its own), surfacing only as a
// sporadic diff once someone diffs two renders of the same input in
// production. The export below generates its object keys dynamically with
// a `for k, v in #items` comprehension - the shape most likely to expose
// unstable iteration - and cuegen.Exec is run many times in the same
// process; every render of the same input must produce byte-identical
// output, in every format.
func TestExecDeterministicAcrossRepeatedRenders(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "")
	writeFile(t, dir, "export.cue", `package control

#items: {
	c: "3"
	a: "1"
	e: "5"
	b: "2"
	d: "4"
}

export: objects: configMap: {
	for k, v in #items {
		"cm-\(k)": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "cm-\(k)"
			data: value: v
		}
	}
}
`)
	t.Chdir(dir)

	const renders = 20
	for _, format := range []Format{FormatYAML, FormatKYAML, FormatJSON} {
		t.Run(format.String(), func(t *testing.T) {
			var first string
			for i := range renders {
				var out bytes.Buffer
				if err := Exec(".", &out, Options{Format: format}); err != nil {
					t.Fatalf("render %d: %v", i, err)
				}
				if i == 0 {
					first = out.String()
					continue
				}
				if got := out.String(); got != first {
					t.Fatalf("render %d differs from render 0:\n--- render 0 ---\n%s\n--- render %d ---\n%s", i, first, i, got)
				}
			}
		})
	}
}
