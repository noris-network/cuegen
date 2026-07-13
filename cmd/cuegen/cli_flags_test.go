package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// buildCuegen compiles the cuegen binary into a temp file and returns its
// path. The binary is shared across all tests in the package.
var cuegenBin string

// wrongSHA1 is a syntactically valid SHA1 (40 hex chars) that no rendered
// output will ever hash to - used to exercise the mismatch path (exit 100)
// without tripping the -cmp-sha1 format validation.
const wrongSHA1 = "0000000000000000000000000000000000000000"

func TestMain(m *testing.M) {
	bin, err := os.CreateTemp("", "cuegen-test-*")
	if err != nil {
		panic(err)
	}
	bin.Close()
	cuegenBin = bin.Name()
	// Deferred so the binary is removed even if m.Run panics; the test
	// wrapper passes m.Run's result to os.Exit when TestMain returns.
	defer os.Remove(cuegenBin)
	cmd := exec.Command("go", "build", "-o", cuegenBin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("go build: %v\n%s", err, out))
	}
	m.Run()
}

// writeTestModule lays out a minimal v2 module in dir with two
// objects so the output is deterministic and multi-document.
func writeTestModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte(`module: "t.local"
language: version: "v0.17.0"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cuegen.cue"), []byte(`package control

cuegen: {
	apiVersion: "v2"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "export.cue"), []byte(`package control

export: objects: {
	configMap: {
		"cm-a": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "cm-a"
			data: a: "1"
		}
	}
	deployment: {
		"dep-a": {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: name: "dep-a"
			spec: replicas: 1
		}
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runCuegen executes the compiled binary with the given args inside dir,
// capturing stdout and stderr separately. It returns (stdout, stderr, exitCode).
func runCuegen(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(cuegenBin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run cuegen: %v", err)
	}
	return stdout.String(), stderr.String(), exitCode
}

// TestSha1Flag verifies that -sha1 prints only the SHA1 hex digest of the
// output, with no version banner on stderr.
func TestSha1Flag(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	stdout, stderr, exit := runCuegen(t, dir, "-sha1", ".")
	if exit != 0 {
		t.Fatalf("exit %d, stderr: %s", exit, stderr)
	}

	// Compute the expected hash from a normal render.
	normalOut, _, ne := runCuegen(t, dir, ".")
	if ne != 0 {
		t.Fatalf("normal render exit %d", ne)
	}
	sum := sha1.Sum([]byte(normalOut))
	want := fmt.Sprintf("%x\n", sum)

	if stdout != want {
		t.Errorf("-sha1 stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("-sha1 stderr should be empty, got %q", stderr)
	}
}

// TestSha1KyamlFlag verifies -sha1 works with -kyaml.
func TestSha1KyamlFlag(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	kyamlOut, _, ne := runCuegen(t, dir, "-kyaml", ".")
	if ne != 0 {
		t.Fatalf("kyaml render exit %d", ne)
	}
	sum := sha1.Sum([]byte(kyamlOut))
	want := fmt.Sprintf("%x\n", sum)

	stdout, stderr, exit := runCuegen(t, dir, "-sha1", "-kyaml", ".")
	if exit != 0 {
		t.Fatalf("exit %d, stderr: %s", exit, stderr)
	}
	if stdout != want {
		t.Errorf("-sha1 -kyaml stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
}

// TestCmpSha1Match verifies that -cmp-sha1 exits 0 when the hash matches.
func TestCmpSha1Match(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	normalOut, _, ne := runCuegen(t, dir, ".")
	if ne != 0 {
		t.Fatalf("normal render exit %d", ne)
	}
	sum := sha1.Sum([]byte(normalOut))
	hash := fmt.Sprintf("%x", sum)

	stdout, stderr, exit := runCuegen(t, dir, "-cmp-sha1", hash, ".")
	if exit != 0 {
		t.Errorf("expected exit 0 on match, got %d (stderr: %s)", exit, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on match, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}

	// The validator accepts uppercase hex, so the comparison must too:
	// an uppercase hash of the same digest is a match, not a mismatch.
	_, stderr, exit = runCuegen(t, dir, "-cmp-sha1", strings.ToUpper(hash), ".")
	if exit != 0 {
		t.Errorf("expected exit 0 on uppercase match, got %d (stderr: %s)", exit, stderr)
	}
}

// TestCmpSha1Mismatch verifies that -cmp-sha1 exits 100 when the hash differs.
func TestCmpSha1Mismatch(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	stdout, stderr, exit := runCuegen(t, dir, "-cmp-sha1", wrongSHA1, ".")
	if exit != 100 {
		t.Errorf("expected exit 100 on mismatch, got %d", exit)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on mismatch, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
}

// TestCmpSha1InvalidHash verifies that a malformed hash argument is a usage
// error (exit 1 with a diagnostic), not a mismatch (exit 100) - a typo'd
// hash must not be mistakable for drift.
func TestCmpSha1InvalidHash(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	for _, invalid := range []string{"deadbeef", "wrong", ""} {
		stdout, stderr, exit := runCuegen(t, dir, "-cmp-sha1", invalid, ".")
		if exit != 1 {
			t.Errorf("-cmp-sha1 %q: expected exit 1, got %d", invalid, exit)
		}
		if stdout != "" {
			t.Errorf("-cmp-sha1 %q: stdout should be empty, got %q", invalid, stdout)
		}
		if !strings.Contains(stderr, "not a valid SHA1 hash") {
			t.Errorf("-cmp-sha1 %q: stderr should explain the invalid hash, got %q", invalid, stderr)
		}
	}

	// A missing value - flag at the end, or followed by another flag -
	// gets its own diagnostic instead of swallowing the next argument.
	for _, args := range [][]string{
		{"-cmp-sha1"},
		{"-cmp-sha1", "-kyaml", "."},
	} {
		_, stderr, exit := runCuegen(t, dir, args...)
		if exit != 1 {
			t.Errorf("%v: expected exit 1, got %d", args, exit)
		}
		if !strings.Contains(stderr, "missing value") {
			t.Errorf("%v: stderr should report the missing value, got %q", args, stderr)
		}
	}
}

// TestCmpSha1Kyaml verifies -cmp-sha1 against a KYAML-output hash.
func TestCmpSha1Kyaml(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	kyamlOut, _, ne := runCuegen(t, dir, "-kyaml", ".")
	if ne != 0 {
		t.Fatalf("kyaml render exit %d", ne)
	}
	sum := sha1.Sum([]byte(kyamlOut))
	hash := fmt.Sprintf("%x", sum)

	_, _, exit := runCuegen(t, dir, "-cmp-sha1", hash, "-kyaml", ".")
	if exit != 0 {
		t.Errorf("expected exit 0 on kyaml match, got %d", exit)
	}

	_, _, exit = runCuegen(t, dir, "-cmp-sha1", wrongSHA1, "-kyaml", ".")
	if exit != 100 {
		t.Errorf("expected exit 100 on kyaml mismatch, got %d", exit)
	}
}

// TestIsCuegenDirSuppressesBanner verifies the ArgoCD detection probe
// produces no version banner.
func TestIsCuegenDirSuppressesBanner(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	stdout, stderr, exit := runCuegen(t, dir, "-is-cuegen-dir")
	if exit != 0 {
		t.Fatalf("exit %d, stderr: %s", exit, stderr)
	}
	if stdout != "true\n" {
		t.Errorf("stdout = %q, want %q", stdout, "true\n")
	}
	if stderr != "" {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
}

// TestNormalRenderShowsBanner verifies the version banner appears on a
// normal render (no suppression flags).
func TestNormalRenderShowsBanner(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	_, stderr, exit := runCuegen(t, dir, ".")
	if exit != 0 {
		t.Fatalf("exit %d, stderr: %s", exit, stderr)
	}
	if !strings.Contains(stderr, "[INFO]") {
		t.Errorf("stderr should contain [INFO] banner, got %q", stderr)
	}
}

// TestMutuallyExclusiveFlags verifies that contradictory flag pairs are
// rejected with a diagnostic instead of one flag silently winning: -sha1
// would otherwise swallow -cmp-sha1's compare-and-exit contract.
func TestMutuallyExclusiveFlags(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"sha1+cmp-sha1", []string{"-sha1", "-cmp-sha1", wrongSHA1, "."}},
		{"cmp-sha1+sha1", []string{"-cmp-sha1", wrongSHA1, "-sha1", "."}},
		{"kyaml+json", []string{"-kyaml", "-json", "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exit := runCuegen(t, dir, tc.args...)
			if exit != 1 {
				t.Errorf("expected exit 1, got %d (stderr: %s)", exit, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got %q", stdout)
			}
			if !strings.Contains(stderr, "mutually exclusive") {
				t.Errorf("stderr should report the exclusive flags, got %q", stderr)
			}
		})
	}
}

// TestUnknownFlagRejected verifies that v2 modules reject dash-prefixed
// arguments the flag scan did not recognize - a typo ("-shal", "--json")
// must not silently become a path argument and change the output.
func TestUnknownFlagRejected(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"double-dash flag", []string{"--json", "."}, `unknown flag "--json"`},
		{"flag typo", []string{"-shal", "."}, `unknown flag "-shal"`},
		{"is-cuegen-dir combined", []string{"-is-cuegen-dir", "."}, "-is-cuegen-dir must be the only argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exit := runCuegen(t, dir, tc.args...)
			if exit != 1 {
				t.Errorf("expected exit 1, got %d (stderr: %s)", exit, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got %q", stdout)
			}
			if !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr should contain %q, got %q", tc.wantErr, stderr)
			}
		})
	}
}

// TestExtraArgsRejected verifies that more than one positional argument is
// an error for v2 modules.
func TestExtraArgsRejected(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	stdout, stderr, exit := runCuegen(t, dir, "sub", "extra")
	if exit != 1 {
		t.Errorf("expected exit 1, got %d (stderr: %s)", exit, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty, got %q", stdout)
	}
	if !strings.Contains(stderr, "too many arguments") {
		t.Errorf("stderr should report too many arguments, got %q", stderr)
	}
}

// TestPathArgumentUnifiesWithCWD pins the corrected apiVersion/path
// semantics: cuegen.cue is always read from the CWD, never from the path
// argument, and the path argument is passed straight to CUE relative to
// the CWD - so `cuegen sub` merges sub's package with the enclosing one,
// exactly like `cue cmd exp sub` would (see examples/webapp/prod). The
// subdirectory has no cuegen.cue of its own, so a probe that (wrongly)
// followed the path argument would fall back to legacy instead of
// rendering.
func TestPathArgumentUnifiesWithCWD(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	// A value hole in the root export, only filled in by sub/ - proves the
	// render actually unified sub's package with the CWD's, not just that
	// sub happened to be self-sufficient.
	if err := os.WriteFile(filepath.Join(dir, "hole.cue"), []byte(`package control

$fromSub: string
export: objects: configMap: hole: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "hole"
	data: v: $fromSub
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "fill.cue"), []byte(`package control

$fromSub: "unified"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, exit := runCuegen(t, dir, "sub")
	if exit != 0 {
		t.Fatalf("exit %d, stderr: %s", exit, stderr)
	}
	if !strings.Contains(stdout, "unified") {
		t.Errorf("stdout should contain the value unified from sub/, got %q", stdout)
	}
	if strings.Contains(stderr, "fallback") {
		t.Errorf("must not fall back to legacy, stderr: %q", stderr)
	}
}

// TestHelpFlag verifies -h prints the usage synopsis and nothing else,
// without needing a module directory.
func TestHelpFlag(t *testing.T) {
	dir := t.TempDir() // no module needed

	for _, arg := range []string{"-h"} {
		stdout, stderr, exit := runCuegen(t, dir, arg)
		if exit != 0 {
			t.Errorf("%s: exit %d, stderr: %s", arg, exit, stderr)
		}
		if !strings.HasPrefix(stdout, "Usage:") {
			t.Errorf("%s: stdout = %q, want usage synopsis", arg, stdout)
		}
		if stderr != "" {
			t.Errorf("%s: stderr should be empty, got %q", arg, stderr)
		}
	}
}

// TestVersionAliases verifies `version` and `-version` both print the
// version line and nothing else.
func TestVersionAliases(t *testing.T) {
	dir := t.TempDir() // no module needed

	for _, arg := range []string{"version", "-version"} {
		stdout, stderr, exit := runCuegen(t, dir, arg)
		if exit != 0 {
			t.Errorf("%s: exit %d, stderr: %s", arg, exit, stderr)
		}
		if !strings.HasPrefix(stdout, "cuegen ") {
			t.Errorf("%s: stdout = %q, want version line", arg, stdout)
		}
		if stderr != "" {
			t.Errorf("%s: stderr should be empty, got %q", arg, stderr)
		}
	}
}

// TestMissingCuegenCueIsHardError verifies that a CWD with no cuegen.cue at
// all is a hard error, not a legacy fallback: such a directory isn't a
// cuegen module, legacy or otherwise, so exec'ing the legacy binary against
// it would only trade one failure for a more confusing one. This must hold
// regardless of which flags are given, since the check fires before the
// v2-flags-vs-legacy guard (TestLegacyFallbackRejectsV2Flags) even runs.
func TestMissingCuegenCueIsHardError(t *testing.T) {
	dir := t.TempDir() // deliberately empty: no cuegen.cue

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no flags", []string{"."}},
		{"v2 flag", []string{"-sha1", "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exit := runCuegen(t, dir, tc.args...)
			if exit != 1 {
				t.Errorf("expected exit 1, got %d (stderr: %s)", exit, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got %q", stdout)
			}
			if !strings.Contains(stderr, "cuegen.cue") {
				t.Errorf("stderr should mention the missing cuegen.cue, got %q", stderr)
			}
			if strings.Contains(stderr, "fallback") {
				t.Errorf("must not attempt the legacy fallback, stderr: %q", stderr)
			}
		})
	}
}

// writeLegacyModule lays out a cuegen.cue whose apiVersion selects the
// legacy fallback. No cue.mod is needed: the legacy paths never render.
func writeLegacyModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "cuegen.cue"), []byte(`package control

cuegen: apiVersion: "v1beta1"
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyFallbackRejectsV2Flags verifies that v2-only flags abort the
// legacy fallback with a diagnostic instead of being silently dropped -
// -cmp-sha1 exiting 0 without comparing would fake a hash match. The guard
// fires before the legacy binary is looked up, so none is needed here.
func TestLegacyFallbackRejectsV2Flags(t *testing.T) {
	legacyDir := t.TempDir()
	writeLegacyModule(t, legacyDir)

	for _, tc := range []struct {
		name string
		dir  string
		args []string
	}{
		{"sha1", legacyDir, []string{"-sha1", "."}},
		{"cmp-sha1", legacyDir, []string{"-cmp-sha1", wrongSHA1, "."}},
		{"json", legacyDir, []string{"-json", "."}},
		{"kyaml", legacyDir, []string{"-kyaml", "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exit := runCuegen(t, tc.dir, tc.args...)
			if exit != 1 {
				t.Errorf("expected exit 1, got %d (stderr: %s)", exit, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got %q", stdout)
			}
			if !strings.Contains(stderr, tc.args[0]) {
				t.Errorf("stderr should name the offending flag %q, got %q", tc.args[0], stderr)
			}
			if !strings.Contains(stderr, "only supported for v2 modules") {
				t.Errorf("stderr should explain the v2 requirement, got %q", stderr)
			}
		})
	}
}

// runWithFakeLegacy invokes cuegen in dir with a fake legacy binary on PATH
// that echoes its argv, returning cuegen's stdout and stderr.
func runWithFakeLegacy(t *testing.T, dir string, args ...string) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\necho \"legacy called with: $@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "cuegen_v0.16.8"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(cuegenBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), stderr.String()
}

// TestLegacyFallbackWithoutFlags verifies that a flag-free invocation still
// execs the legacy binary, forwarding the remaining args verbatim.
func TestLegacyFallbackWithoutFlags(t *testing.T) {
	dir := t.TempDir()
	writeLegacyModule(t, dir)

	stdout, stderr := runWithFakeLegacy(t, dir, ".")
	if want := "legacy called with: .\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if !strings.Contains(stderr, "fallback to cuegen_v0.16.8") {
		t.Errorf("stderr should announce the fallback, got %q", stderr)
	}
}

// TestLegacyFallbackForwardsUnknownFlags pins the asymmetry to
// TestUnknownFlagRejected: args the flag scan does not recognize are only
// rejected on the v2 path - the legacy binary receives them verbatim, since
// they may be meaningful to it.
func TestLegacyFallbackForwardsUnknownFlags(t *testing.T) {
	dir := t.TempDir()
	writeLegacyModule(t, dir)

	stdout, _ := runWithFakeLegacy(t, dir, "-debug", ".")
	if want := "legacy called with: -debug .\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestJsonFlag verifies -json produces a valid JSON object keyed by
// "<kind>/<metadata.name>", with the correct number of entries, and that
// the hash matches `cuegen -json | sha1sum`.
func TestJsonFlag(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	stdout, stderr, exit := runCuegen(t, dir, "-json", ".")
	if exit != 0 {
		t.Fatalf("exit %d, stderr: %s", exit, stderr)
	}

	// Must be a valid JSON object with 2 keys (ConfigMap/cm-a + Deployment/dep-a).
	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(obj) != 2 {
		t.Errorf("JSON object size = %d, want 2", len(obj))
	}
	for _, key := range []string{"ConfigMap/cm-a", "Deployment/dep-a"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("JSON object missing key %q, got keys: %v", key, slices.Sorted(maps.Keys(obj)))
		}
	}

	// Hash must match piped output.
	sum := sha1.Sum([]byte(stdout))
	want := fmt.Sprintf("%x\n", sum)

	hashOut, _, he := runCuegen(t, dir, "-json", "-sha1", ".")
	if he != 0 {
		t.Fatalf("hash exit %d", he)
	}
	if hashOut != want {
		t.Errorf("-json -sha1 = %q, want %q", hashOut, want)
	}
}

// TestCmpSha1Json verifies -cmp-sha1 works with -json output.
func TestCmpSha1Json(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	jsonOut, _, ne := runCuegen(t, dir, "-json", ".")
	if ne != 0 {
		t.Fatalf("json render exit %d", ne)
	}
	sum := sha1.Sum([]byte(jsonOut))
	hash := fmt.Sprintf("%x", sum)

	_, _, exit := runCuegen(t, dir, "-cmp-sha1", hash, "-json", ".")
	if exit != 0 {
		t.Errorf("expected exit 0 on json match, got %d", exit)
	}

	_, _, exit = runCuegen(t, dir, "-cmp-sha1", wrongSHA1, "-json", ".")
	if exit != 100 {
		t.Errorf("expected exit 100 on json mismatch, got %d", exit)
	}
}

// TestWideFlag verifies -wide indents list items under their parent key
// (matching mikefarah/yq), while the default keeps them flush left. The
// difference must show up as a "- " vs "  - " prefix on the first list item.
func TestWideFlag(t *testing.T) {
	dir := t.TempDir()
	writeModuleWithList(t, dir)

	compact, _, ce := runCuegen(t, dir, ".")
	if ce != 0 {
		t.Fatalf("compact render exit %d", ce)
	}
	wide, _, we := runCuegen(t, dir, "-wide", ".")
	if we != 0 {
		t.Fatalf("wide render exit %d", we)
	}

	// Compact: list dashes are at the same indentation as the parent key.
	// Wide: list dashes are indented 2 spaces deeper than the parent key.
	for _, line := range strings.Split(compact, "\n") {
		if strings.HasPrefix(line, "  - name: app") {
			goto compactOK
		}
	}
	t.Fatalf("compact output should have flush-left list items:\n%s", compact)
compactOK:
	for _, line := range strings.Split(wide, "\n") {
		if strings.HasPrefix(line, "    - name: app") {
			goto wideOK
		}
	}
	t.Fatalf("wide output should have indented list items:\n%s", wide)
wideOK:

	// -wide must produce different output than the default.
	if compact == wide {
		t.Fatalf("compact and wide output are identical (expected different indentation)")
	}
}

// TestWideEnvVar verifies CUEGEN_WIDE=true produces the same wide-indented
// output as the -wide flag, and that CUEGEN_WIDE=false leaves the default
// compact style intact.
func TestWideEnvVar(t *testing.T) {
	dir := t.TempDir()
	writeModuleWithList(t, dir)

	flagWide, _, we := runCuegen(t, dir, "-wide", ".")
	if we != 0 {
		t.Fatalf("-wide render exit %d", we)
	}

	t.Setenv("CUEGEN_WIDE", "true")
	envWide, _, ee := runCuegen(t, dir, ".")
	if ee != 0 {
		t.Fatalf("CUEGEN_WIDE=true render exit %d", ee)
	}
	if envWide != flagWide {
		t.Errorf("CUEGEN_WIDE=true output differs from -wide output")
	}

	t.Setenv("CUEGEN_WIDE", "false")
	envCompact, _, ce := runCuegen(t, dir, ".")
	if ce != 0 {
		t.Fatalf("CUEGEN_WIDE=false render exit %d", ce)
	}
	for _, line := range strings.Split(envCompact, "\n") {
		if strings.HasPrefix(line, "    - name: app") {
			t.Fatalf("CUEGEN_WIDE=false should produce compact output:\n%s", envCompact)
		}
	}
}

// writeModuleWithList lays out a minimal v2 module whose Deployment has a
// containers list, so the compact vs wide indentation difference is visible.
func writeModuleWithList(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte(`module: "t.local"
language: version: "v0.17.0"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cuegen.cue"), []byte(`package control

cuegen: {
	apiVersion: "v2"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "export.cue"), []byte(`package control

export: objects: deployment: app: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "app"
	spec: containers: [{
		name:  "app"
		image: "nginx:1.27"
	}]
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
}
