package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// demoAgeKey decrypts the sops-encrypted sources under examples/sops. It is a
// throwaway identity generated for the example and documented in
// examples/README.md - it guards nothing.
const demoAgeKey = "AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ"

// TestExamplesMatchGoldenFiles renders every module under examples/ through
// the real binary and diffs stdout against the expected.<format> files that
// ship beside it, in all three formats.
//
// Those golden files are exactly what examples/README.md tells a reader to
// verify against, but nothing asserted them: TestExamplesRenderDeterministically
// diffs a render against a second render of the same input, never against the
// checked-in bytes. A stale golden file therefore stayed stale silently, and
// the documented command for examples/webapp had drifted to one that does not
// even render.
//
// The invocation per example is the one that example is built around, not a
// blanket ".": webapp declares a value hole that only its prod/ subdirectory
// fills, so `cuegen .` there is a legitimate hard error and the goldens
// describe `cuegen ./prod`. Keep this table and examples/README.md in step -
// they document the same contract.
func TestExamplesMatchGoldenFiles(t *testing.T) {
	examples := []struct {
		dir    string
		path   string
		ageKey string // non-empty for modules with sops-encrypted sources
	}{
		{dir: "minimal", path: "."},
		{dir: "webapp", path: "./prod"},
		{dir: "sops", path: ".", ageKey: demoAgeKey},
	}
	formats := []struct{ flag, ext string }{
		{"", "yaml"},
		{"-kyaml", "kyaml"},
		{"-json", "json"},
	}

	for _, ex := range examples {
		t.Run(ex.dir, func(t *testing.T) {
			dir := filepath.Join("..", "..", "examples", ex.dir)
			if _, err := os.Stat(dir); err != nil {
				t.Skipf("example module not found: %v", err)
			}
			compare := func() {
				for _, f := range formats {
					t.Run(f.ext, func(t *testing.T) {
						golden := filepath.Join(dir, "expected."+f.ext)
						want, err := os.ReadFile(golden)
						if err != nil {
							t.Fatalf("read golden file: %v", err)
						}
						args := make([]string, 0, 2)
						if f.flag != "" {
							args = append(args, f.flag)
						}
						args = append(args, ex.path)

						stdout, stderr, code := runCuegen(t, dir, args...)
						if code != 0 {
							t.Fatalf("cuegen %s in %s exited %d\nstderr:\n%s",
								strings.Join(args, " "), dir, code, stderr)
						}
						if stdout != string(want) {
							t.Errorf("output does not match %s\n--- want ---\n%s\n--- got ---\n%s",
								golden, want, stdout)
						}
					})
				}
			}
			// The subprocess inherits the test process's environment, so
			// setting the identity here is enough for the sops example.
			if ex.ageKey != "" {
				withAgeKey(t, ex.ageKey, compare)
				return
			}
			compare()
		})
	}
}

// TestExamplesWebappBareDotIsAnError pins the reason the webapp goldens are
// rendered with "./prod" rather than ".": the parent package deliberately
// leaves a value hole for an environment overlay to fill, so a bare render is
// expected to fail concreteness validation. If this ever starts succeeding,
// the example no longer demonstrates what it claims to and the golden table
// above needs revisiting.
func TestExamplesWebappBareDotIsAnError(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "webapp")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("example module not found: %v", err)
	}
	stdout, stderr, code := runCuegen(t, dir, ".")
	if code == 0 {
		t.Fatalf("cuegen . in examples/webapp unexpectedly succeeded:\n%s", stdout)
	}
	if !strings.Contains(stderr, "non-concrete") {
		t.Errorf("expected a non-concrete diagnostic, got:\n%s", stderr)
	}
}
