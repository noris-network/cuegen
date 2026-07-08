// Command cuegen renders a cuegen module (cuegen.apiVersion v2) into a
// "---"-separated YAML stream, behaving like `cue cmd exp`. Modules with an
// older or missing cuegen.apiVersion are delegated to the legacy
// cuegen_v0.16.6 binary via execve.
//
// cuegen is Unix-only: the legacy fallback uses syscall.Exec and the overlay
// walker relies on inode metadata, neither of which is available on Windows.
// The sole deployment target is Linux (ArgoCD Config Management Plugin).
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	"github.com/noris-network/cuegen/internal/engine"
)

const (
	cuegenCue    = "cuegen.cue"
	legacyBinary = "cuegen_v0.16.8"
	// legacyReleaseURL points to the release page hosting the legacy binary.
	// Surfaced in the error message when the binary is missing from PATH so the
	// operator knows where to fetch it.
	legacyReleaseURL = "https://github.com/noris-network/cuegen/releases/tag/v0.16.8"
)

// build is set via -ldflags at release time (goreleaser/Dockerfile). Defaults
// to "dev" for local builds.
var build = "dev"

func main() {
	// Handle "version" before anything else — no banner, no module needed.
	if len(os.Args) == 2 && os.Args[1] == "version" {
		printVersion()
		return
	}

	// Uniform, timestamp-free logging: informational lines use "[INFO]" via
	// fmt.Fprintf, fatal errors go through log with a "cuegen:" prefix.
	log.SetFlags(0)
	log.SetPrefix("cuegen: ")

	fmt.Fprint(os.Stderr, "[INFO] *** cuegen ***\n")

	// argocd cmp check
	cmpPluginCheck()

	engine.FileFilter = sopsFilter

	args := os.Args[1:]
	path := "."
	if len(args) == 1 {
		path = args[0]
	}

	apiVersion, err := readAPIVersion(cuegenCue)
	if err != nil {
		// Could not determine apiVersion (missing cuegen.cue, missing field,
		// or parse error): defer to the legacy binary for backward compat.
		fmt.Fprintf(os.Stderr, "[INFO] read apiVersion: %v\n", err)
		runLegacy(args)
		return
	}

	if !isV2(apiVersion) {
		runLegacy(args)
		return
	}

	// v2 ignores any arguments beyond the path; surface that instead of
	// silently dropping them. (The legacy path forwards all args verbatim.)
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "[INFO] ignoring extra arguments beyond path: %v\n", args[1:])
	}

	if err := engine.Exec(path, os.Stdout); err != nil {
		log.Fatalln(err)
	}
}

// runLegacy replaces this process with the legacy binary via execve. The
// legacy program inherits the same PID, stdin/stdout/stderr file descriptors
// and environment, so to the caller (and to ArgoCD) it is indistinguishable
// from having invoked the legacy binary directly. Its exit code becomes ours.
// Used for every module whose cuegen.apiVersion is not v2.
func runLegacy(args []string) {
	binary, err := exec.LookPath(legacyBinary)
	if err != nil {
		log.Fatalf("legacy binary %q not found in PATH: %v\n"+
			"install it from %s and ensure it is executable and on your PATH",
			legacyBinary, err, legacyReleaseURL)
	}
	fmt.Fprintln(os.Stderr, "[INFO] fallback to", legacyBinary)
	argv := append([]string{legacyBinary}, args...)
	if err := syscall.Exec(binary, argv, os.Environ()); err != nil {
		log.Fatalf("exec %s: %v", legacyBinary, err)
	}
}

// isV2 reports whether apiVersion denotes the v2 generation ("v2alpha1",
// "v2", "v2.0.0", …) by comparing the numeric major version to 2. Anything
// else — including v1*, v0*, v3* and unparseable values — falls back to
// legacy.
func isV2(apiVersion string) bool {
	maj, ok := majorVersion(apiVersion)
	return ok && maj == 2
}

// majorVersion extracts the leading integer from a version string of the
// form "v<major><rest>" (e.g. "v2alpha1" -> 2, "v1beta1" -> 1, "v0.16.6" ->
// 0). The leading "v" is optional.
func majorVersion(s string) (int, bool) {
	s = strings.TrimPrefix(s, "v")
	var d strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		d.WriteRune(r)
	}
	if d.Len() == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(d.String())
	if err != nil {
		return 0, false
	}
	return n, true
}

// readAPIVersion extracts the literal value of `cuegen.apiVersion` from a
// CUE source file by walking the AST. We deliberately avoid building the
// file with CUE so unresolved imports (which a real module will have inside
// other cuegen.* fields) do not block the parse. Accepts both nested forms:
//
//	cuegen: { apiVersion: "v2alpha1" }    // struct literal
//	cuegen: apiVersion: "v2alpha1"        // chained label shorthand
func readAPIVersion(file string) (string, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	f, err := parser.ParseFile(file, src)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", file, err)
	}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		if label, _, _ := ast.LabelName(fd.Label); label != "cuegen" {
			continue
		}
		if lit, ok := findAPIVersion(fd.Value); ok {
			return lit, nil
		}
	}
	return "", fmt.Errorf("%s: cuegen.apiVersion not found", file)
}

// findAPIVersion descends one level into a struct literal looking for the
// `apiVersion: "..."` field. Both `cuegen: { apiVersion: "x" }` and the
// chained shorthand `cuegen: apiVersion: "x"` parse to a StructLit whose
// only Elt is the apiVersion field, so a single case covers both.
func findAPIVersion(expr ast.Expr) (string, bool) {
	s, ok := expr.(*ast.StructLit)
	if !ok {
		return "", false
	}
	for _, e := range s.Elts {
		fd, ok := e.(*ast.Field)
		if !ok {
			continue
		}
		if label, _, _ := ast.LabelName(fd.Label); label == "apiVersion" {
			return stringLit(fd.Value)
		}
	}
	return "", false
}

func stringLit(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func cmpPluginCheck() {
	if len(os.Args) != 2 || os.Args[1] != "-is-cuegen-dir" {
		return
	}
	if _, err := os.Stat(cuegenCue); err == nil {
		fmt.Println(true)
	}
	os.Exit(0)
}

// printVersion prints the cuegen version along with the embedded CUE version
// and build platform. The CUE version is read from the module build info so it
// stays accurate without a hardcoded constant.
func printVersion() {
	cueVer := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "cuelang.org/go" {
				cueVer = dep.Version
				break
			}
		}
	}
	fmt.Printf("cuegen %s (cue %s, %s/%s)\n", build, cueVer, runtime.GOOS, runtime.GOARCH)
}
