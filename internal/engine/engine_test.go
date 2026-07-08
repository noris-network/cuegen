package engine

import (
	"bytes"
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

// chdir changes into dir for the duration of the test, restoring the working
// directory on cleanup. The engine loads via ".", mirroring real CLI usage;
// CUE's loader rejects absolute paths, so tests must run from inside the
// module directory.
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

// writeModule scaffolds a self-contained v2alpha1 module in dir. If exportPath
// is empty, cuegen.spec.export is omitted so the default kicks in.
func writeModule(t *testing.T, dir, exportPath string) {
	t.Helper()
	writeFile(t, dir, "cue.mod/module.cue", `module: "t.local"
language: version: "v0.17.0"
`)
	if exportPath == "" {
		writeFile(t, dir, "cuegen.cue", `package control

cuegen: {
	apiVersion: "v2alpha1"
}
`)
	} else {
		writeFile(t, dir, "cuegen.cue", `package control

cuegen: {
	apiVersion: "v2alpha1"
	spec: export: "`+exportPath+`"
}
`)
	}
}

// TestExecDefaultExportPath renders a module that omits cuegen.spec.export;
// the engine must fall back to "export.objects" and emit one YAML document
// per object, separated by "---".
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

	var out bytes.Buffer
	chdir(t, dir)
	if err := Exec(".", &out); err != nil {
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

	var out bytes.Buffer
	chdir(t, dir)
	if err := Exec(".", &out); err != nil {
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

	var out bytes.Buffer
	chdir(t, dir)
	err := Exec(".", &out)
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
