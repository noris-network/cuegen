// Command cuegen renders a cuegen module (cuegen.apiVersion v2) into a
// "---"-separated YAML stream, behaving like `cue cmd exp`. cuegen.cue must
// exist in the current directory - its absence is a hard error, not a
// legacy fallback, since a directory without it isn't a cuegen module at
// all. Modules whose cuegen.cue exists but carries an older or missing
// cuegen.apiVersion are delegated to the legacy cuegen_v0.16.8 binary via
// execve.
//
// cuegen is Unix-only: the legacy fallback uses syscall.Exec and the overlay
// walker relies on inode metadata, neither of which is available on Windows.
// The sole deployment target is Linux (ArgoCD Config Management Plugin).
package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
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
	// Handle "version" before anything else - no banner, no module needed.
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version", "-version":
			printVersion()
			return
		case "-h":
			printUsage()
			return
		}
	}

	// Uniform, timestamp-free logging: informational lines use "[INFO]" via
	// fmt.Fprintf, fatal errors go through log with a "cuegen:" prefix.
	log.SetFlags(0)
	log.SetPrefix("cuegen: ")

	// argocd cmp check - must run before the version banner so the
	// detection probe produces no diagnostic output.
	cmpPluginCheck()

	// Scan for flags and remove them from args so they don't interfere
	// with path resolution or the legacy fallback.
	args := os.Args[1:]
	useKYAML := false
	useJSON := false
	useWide := false
	hashOnly := false
	cmpSHA1 := ""
	cmpSHA1Set := false
	// v2Flags records every recognized flag verbatim. All of them are
	// v2-only; runLegacy refuses the fallback when any are present, since
	// the legacy binary does not understand them (see the guard there).
	var v2Flags []string
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-kyaml":
			useKYAML = true
		case "-json":
			useJSON = true
		case "-wide":
			useWide = true
		case "-sha1":
			hashOnly = true
		case "-cmp-sha1":
			cmpSHA1Set = true
			// A dash-prefixed follower is the next flag, not a value: a
			// valid hash never starts with "-".
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				log.Fatalln("-cmp-sha1: missing value (expected a 40-character hex SHA1)")
			}
			cmpSHA1 = args[i+1]
			i++ // skip the hash value
		default:
			filtered = append(filtered, arg)
			continue
		}
		v2Flags = append(v2Flags, arg)
	}
	args = filtered

	// CUEGEN_WIDE=true enables wide sequence indentation without the -wide
	// flag, e.g. for ArgoCD CMP deployments where CLI flags aren't easily
	// passed. Invalid values are rejected with a diagnostic, consistent
	// with the strict flag validation; the -wide flag takes precedence
	// (both combine with OR).
	if v, ok := os.LookupEnv("CUEGEN_WIDE"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("CUEGEN_WIDE: %q is not a valid boolean (expect true/false/1/0)", v)
		}
		useWide = useWide || b
	}

	if cmpSHA1Set && !isValidSHA1(cmpSHA1) {
		log.Fatalf("-cmp-sha1: %q is not a valid SHA1 hash (expected 40 hex characters)", cmpSHA1)
	}
	// isValidSHA1 accepts both cases; canonicalize to lowercase so the
	// comparison against the %x-formatted (lowercase) digest cannot miss.
	cmpSHA1 = strings.ToLower(cmpSHA1)
	if useKYAML && useJSON {
		log.Fatalln("-kyaml and -json are mutually exclusive")
	}
	// -sha1 prints the hash, -cmp-sha1 compares it and only reports via the
	// exit code - one invocation cannot do both, and silently preferring one
	// would swallow the other's contract.
	if hashOnly && cmpSHA1Set {
		log.Fatalln("-sha1 and -cmp-sha1 are mutually exclusive")
	}

	// Suppress the version banner for hash-only and cmp-sha1 modes.
	if !hashOnly && !cmpSHA1Set {
		fmt.Fprintf(os.Stderr, "[INFO] cuegen %s (cue %s)\n", build, cueVersion())
	}

	// cuegen.cue is always read from the process's current working
	// directory, never from the path argument. CUE unifies a directory's
	// package with the same-named package in every ancestor directory up to
	// the module root, so `cuegen ./prod` merges values defined in ./prod with
	// those in the CWD (see engine.Exec and examples/webapp/prod) - the path
	// argument names a value to unify into the current module, not a
	// separate module to switch into.
	apiVersion, err := readAPIVersion(cuegenCue)
	if errors.Is(err, os.ErrNotExist) {
		// No cuegen.cue at all means this directory isn't a cuegen module,
		// legacy or otherwise - falling back would just exec the legacy
		// binary against a directory it can't handle either.
		log.Fatalf("%s: no such file (cuegen must be run from a module directory)", cuegenCue)
	}
	if err != nil {
		// cuegen.cue exists but could not be parsed or has no apiVersion
		// field (a genuinely old-style file predating apiVersion): defer to
		// the legacy binary for backward compat.
		fmt.Fprintf(os.Stderr, "[INFO] read apiVersion: %v\n", err)
		runLegacy(args, v2Flags)
		return
	}

	if !isV2(apiVersion) {
		runLegacy(args, v2Flags)
		return
	}

	// v2 accepts exactly one optional positional argument: the module path.
	// Reject anything else - a flag typo ("--json", "-shal") would otherwise
	// be mistaken for a path and silently change output format or target.
	// (The legacy path is not affected: it forwards all args verbatim, since
	// unknown flags may be meaningful to the legacy binary.)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "-is-cuegen-dir" {
			log.Fatalln("-is-cuegen-dir must be the only argument")
		}
		log.Fatalf("unknown flag %q", arg)
	}
	if len(args) > 1 {
		log.Fatalf("too many arguments %q: expected at most one module path", args)
	}
	path := "."
	if len(args) == 1 {
		path = args[0]
	}

	format := engine.FormatYAML
	switch {
	case useKYAML:
		format = engine.FormatKYAML
	case useJSON:
		format = engine.FormatJSON
	}
	opts := engine.Options{Format: format, WideSeqIndent: useWide, FileFilter: sopsFilter}

	if hashOnly || cmpSHA1Set {
		var buf bytes.Buffer
		if err := engine.Exec(path, &buf, opts); err != nil {
			log.Fatalln(err)
		}
		// SHA-1 is used here as a drift checksum, not a security primitive:
		// the threat model is accidental output change (e.g. a bumped
		// dependency altering the rendered manifest), not adversarial
		// collision. SHA-1 is retained for backward compatibility with
		// existing -cmp-sha1 consumers and CI pipelines; switching to a
		// stronger hash would break those without adding security value.
		sum := sha1.Sum(buf.Bytes())
		computed := fmt.Sprintf("%x", sum)
		if hashOnly {
			fmt.Println(computed)
			return
		}
		// -cmp-sha1: exit 0 on match, exit 100 on mismatch.
		if computed == cmpSHA1 {
			os.Exit(0)
		}
		os.Exit(100)
	}

	if err := engine.Exec(path, os.Stdout, opts); err != nil {
		log.Fatalln(err)
	}
}

// runLegacy replaces this process with the legacy binary via execve. The
// legacy program inherits the same PID, stdin/stdout/stderr file descriptors
// and environment, so to the caller (and to ArgoCD) it is indistinguishable
// from having invoked the legacy binary directly. Its exit code becomes ours.
// Used for every module whose cuegen.apiVersion is not v2.
//
// v2Flags lists the v2-only flags found on the command line. The legacy
// binary does not understand them, and the flag scan has already stripped
// them from args - silently exec'ing would render with the wrong format or,
// worst of all, let -cmp-sha1 exit 0 without comparing anything. Refuse the
// fallback instead of producing misleading output.
func runLegacy(args, v2Flags []string) {
	if len(v2Flags) > 0 {
		log.Fatalf("%s: only supported for v2 modules (cuegen.apiVersion v2*); "+
			"this module would fall back to %s, which does not understand these flags",
			strings.Join(v2Flags, ", "), legacyBinary)
	}
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

// isV2 reports whether apiVersion denotes the v2 generation ("v2",
// "v2.0.0", …) by comparing the numeric major version to 2. Anything
// else - including v1*, v0*, v3* and unparseable values - falls back to
// legacy.
func isV2(apiVersion string) bool {
	maj, ok := majorVersion(apiVersion)
	return ok && maj == 2
}

// majorVersion extracts the leading integer from a version string of the
// form "v<major><rest>" (e.g. "v2" -> 2, "v1beta1" -> 1, "v0.16.6" ->
// 0). The leading "v" is optional.
func majorVersion(s string) (int, bool) {
	s = strings.TrimPrefix(s, "v")
	end := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if end == -1 {
		end = len(s)
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
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
//	cuegen: { apiVersion: "v2" }    // struct literal
//	cuegen: apiVersion: "v2"        // chained label shorthand
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

// isValidSHA1 reports whether s is a 40-character hex string (either case)
// - the textual representation of a SHA1 digest. SHA-1 is used for drift
// detection only (see the comment at the sha1.Sum call site).
func isValidSHA1(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha1.Size
}

// cmpPluginCheck implements the ArgoCD Config Management Plugin detection
// probe. When invoked as `cuegen -is-cuegen-dir` it prints "true" to stdout
// if cuegen.cue is present in the CWD, prints nothing otherwise, and in both
// cases exits 0. The probe must produce no other output (no version banner,
// no diagnostics) so ArgoCD's sidecar can decide ownership cleanly. Any other
// invocation is a no-op and falls through to normal processing.
func cmpPluginCheck() {
	if len(os.Args) != 2 || os.Args[1] != "-is-cuegen-dir" {
		return
	}
	if _, err := os.Stat(cuegenCue); err == nil {
		fmt.Println(true)
	}
	os.Exit(0)
}

// cueVersion returns the version of the embedded cuelang.org/go dependency,
// read from the module build info so it stays accurate without a hardcoded
// constant.
func cueVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "cuelang.org/go" {
				return dep.Version
			}
		}
	}
	return "unknown"
}

// printVersion prints the cuegen version along with the embedded CUE version
// and build platform.
func printVersion() {
	fmt.Printf("cuegen %s (cue %s, %s/%s)\n", build, cueVersion(), runtime.GOOS, runtime.GOARCH)
}

// printUsage prints a short usage synopsis to stdout. Handled before the
// module lookup so `cuegen -h` works outside a cuegen module directory.
func printUsage() {
	fmt.Print(`Usage: cuegen [flags] [path | well-known-arg]

Render a cuegen module (cuegen.apiVersion v2) into a YAML stream. Run from a
directory containing cuegen.cue; the optional path (default ".") names a value
to unify into the current module.

Flags:
  -h               print this usage and exit
  -kyaml           emit KYAML (flow-style) instead of block YAML
  -json            emit a JSON object keyed by <kind>/<metadata.name>, mainly
                   for debugging the generated manifest (e.g. with fx)
  -wide            indent list items under their parent key (yq-style)
                   (also enabled by CUEGEN_WIDE=true)
  -sha1            print only the SHA1 hash of the output
  -cmp-sha1 <hash> compare output hash to <hash>; exit 0 match, 100 mismatch,
                   1 on a malformed hash
  -is-cuegen-dir   ArgoCD probe: print "true" when cuegen.cue is present

Well-known arguments (replace the path, take no flags):
  version          print version and exit (alias: -version)

-kyaml/-json and -sha1/-cmp-sha1 are mutually exclusive. All flags except
-is-cuegen-dir require a v2 module; older modules fall back to cuegen_v0.16.8.
`)
}
