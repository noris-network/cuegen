// Package engine renders cuegen v2 modules into YAML/JSON streams.
//
// Exec loads a CUE module from the current working directory, resolves
// the configured export path, validates all exported objects for
// concreteness, and serializes the result as a "---"-separated YAML
// stream (or JSON/KYAML). A FileFilter hook allows transparent
// pre-processing of file contents before the CUE loader sees them,
// enabling features like SOPS decryption without coupling the engine
// to a specific encryption tool.
package engine

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/inject/embed"
	"cuelang.org/go/cue/load"
	cueyaml "cuelang.org/go/encoding/yaml"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	kyaml "sigs.k8s.io/yaml/kyaml"
)

// defaultExportPath is the CUE path consulted for the per-module object
// collection when a module does not set `cuegen.spec.export` explicitly.
const defaultExportPath = "export.objects"

// Format selects the output encoding of a render.
type Format int

const (
	// FormatYAML emits a "---"-separated stream of block-YAML documents.
	FormatYAML Format = iota
	// FormatKYAML emits KYAML: flow-style YAML with double-quoted strings.
	FormatKYAML
	// FormatJSON emits a single JSON object keyed by "<kind>/<name>".
	FormatJSON
)

// String returns the human-readable name of the format, used in error
// messages and satisfying fmt.Stringer.
func (f Format) String() string {
	switch f {
	case FormatYAML:
		return "yaml"
	case FormatKYAML:
		return "kyaml"
	case FormatJSON:
		return "json"
	default:
		return fmt.Sprintf("format(%d)", f)
	}
}

// FileFilter transforms the raw bytes of a file before the CUE loader sees
// them. It receives the absolute file path (for context-aware decisions,
// e.g. detecting `.enc.cue`) and the raw bytes; it returns the bytes CUE
// should compile. Returning the input unchanged signals "no change" so the
// file is skipped from the overlay.
type FileFilter func(path string, raw []byte) ([]byte, error)

// Options configures a render.
type Options struct {
	// Format selects the output encoding; the zero value is FormatYAML.
	Format Format

	// WideSeqIndent indents sequence/list items under their parent key
	// (two spaces) instead of keeping them flush left. Applies to YAML
	// output only; ignored for KYAML and JSON. Matches the style of
	// mikefarah/yq.
	WideSeqIndent bool

	// FileFilter transforms the raw bytes of every file below the module
	// root before the CUE loader sees them. nil means identity. Plug in
	// transparent decryption (SOPS, age, etc.) here so CUE never knows a
	// file was encrypted and loads the cleartext contents normally. With
	// CUE v0.17+ the resulting overlay also covers @embed targets, so a
	// single hook catches both CUE source and embedded data.
	//
	// Setting a filter is not free: the overlay walk reads every regular
	// file in the module tree into memory (bounded per file by
	// maxFilterFileSize) and runs the filter on each, on every render. For
	// large mono-repos this is a measurable cost paid even when no file is
	// actually encrypted - the filter is the only way to find out.
	FileFilter FileFilter
}

// Exec renders the cuegen module at path, resolved relative to the
// process's current working directory - exactly like `cue cmd exp <path>`.
// This is deliberate, not an oversight: CUE unifies a directory's package
// with the same-named package declared in every ancestor directory up to
// the module root, so e.g. `cuegen ./prod` merges values defined in ./prod with
// those in the CWD (see examples/webapp/prod, which overrides a value hole
// declared in the parent package). Passing an absolute path, or a path
// outside the enclosing module's tree, is not supported - matching plain
// `cue` CLI behavior. Every object under the configured export path is
// serialized as its own document and the documents are emitted as a single
// stream in the configured format.
//
// cuegen performs no $val scope plumbing, import composition or generator
// expansion - v2 modules express all of that in CUE itself. cuegen only
// loads (with transparent decryption via Options.FileFilter) and exports.
func Exec(path string, out io.Writer, opts Options) error {
	if !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "/") && path != "." {
		path = "./" + path
	}

	// The overlay only ever contains files the filter changed; without a
	// filter it would always be empty, so the walk is skipped entirely.
	var overlay map[string]load.Source
	if opts.FileFilter != nil {
		root, err := moduleRoot()
		if err != nil {
			return fmt.Errorf("find module root: %w", err)
		}
		overlay, err = buildOverlay(root, opts.FileFilter)
		if err != nil {
			return fmt.Errorf("build overlay for %q: %w", root, err)
		}
	}

	cfg := &load.Config{Overlay: overlay}
	insts := load.Instances([]string{path}, cfg)
	if len(insts) == 0 {
		return fmt.Errorf("load instance %q: no instance returned", path)
	}
	inst := insts[0]
	if err := inst.Err; err != nil {
		return fmt.Errorf("load instance %q:\n%s", path, cueDetail(err))
	}

	ctx := cuecontext.New(cuecontext.WithInjection(embed.New()))
	val := ctx.BuildInstance(inst)
	if err := val.Err(); err != nil {
		return fmt.Errorf("build instance %q:\n%s", path, cueDetail(err))
	}

	expPath, err := exportPath(val)
	if err != nil {
		return err
	}
	objs := val.LookupPath(cue.ParsePath(expPath))
	if !objs.Exists() {
		return fmt.Errorf("export path %q not found", expPath)
	}
	if err := objs.Err(); err != nil {
		return fmt.Errorf("lookup %s:\n%s", expPath, cueDetail(err))
	}

	values, err := flattenObjects(objs)
	if err != nil {
		return fmt.Errorf("collect objects from %s: %w", expPath, err)
	}
	if len(values) == 0 {
		return fmt.Errorf("export %s contains no objects, refusing to render an empty stream", expPath)
	}

	if err := requireConcrete(values); err != nil {
		return err
	}

	// Force-evaluate the whole export.objects struct - not just the
	// materialized leaves that requireConcrete checked. A field whose
	// dynamic key is non-concrete (typically metadata.name derived from an
	// unset optional value) is never yielded by Fields(), so flattenObjects
	// never sees it and the object would vanish from the output silently -
	// no error, exit 0. Validating the full struct with Concrete(true)
	// forces every dynamic key to resolve, surfacing the same diagnostic
	// `cue export -e export.objects` produces ("key value of dynamic field
	// must be concrete"). Since requireConcrete already cleared every
	// materialized object, any error here is a dropped object (incomplete
	// dynamic key) - turning a silent drop into a loud, located failure.
	if err := requireComplete(objs); err != nil {
		return err
	}

	// Convert each CUE value to a kyaml RNode directly. CUE encodes to
	// YAML bytes, yaml.Parse decodes a single document - no --- buffer
	// concatenation or regex-based splitting. Encoding all documents
	// before emitting any output matches `cue cmd exp`: a single
	// failure produces no partial output.
	nodes := make([]*yaml.RNode, 0, len(values))
	for i, obj := range values {
		b, err := cueyaml.Encode(obj)
		if err != nil {
			return fmt.Errorf("encode yaml for %s[%d]: %w", expPath, i, err)
		}
		node, err := yaml.Parse(string(b))
		if err != nil {
			return fmt.Errorf("parse yaml for %s[%d]: %w", expPath, i, err)
		}
		nodes = append(nodes, node)
	}

	// Apply filters directly on the node slice - no kio.Pipeline /
	// ByteReader infrastructure needed. Canonically format fields and
	// sort whitelisted lists, then sort documents by kind then
	// metadata.name.
	for _, f := range []kio.Filter{
		filters.FormatFilter{},
		sortByKindName{},
	} {
		nodes, err = f.Filter(nodes)
		if err != nil {
			return fmt.Errorf("filter output: %w", err)
		}
	}

	switch opts.Format {
	case FormatKYAML:
		return writeKyaml(nodes, out)
	case FormatJSON:
		return writeJSON(nodes, out)
	case FormatYAML:
		return writeYaml(nodes, out, opts.WideSeqIndent)
	default:
		return fmt.Errorf("unknown output format %s", opts.Format)
	}
}

// writeJSON marshals the filtered nodes into a single indented JSON object.
// Each key is "<kind>/<metadata.name>" - deliberately without the
// namespace, which has no value for cuegen's use case. Two objects sharing
// kind and name are therefore a hard duplicate-key error, even if the same
// module renders fine as YAML/KYAML (where namespace does disambiguate) -
// the error message says so, since -json is a debugging aid (piping into
// fx/jq) rather than a format every module is expected to support.
// Insertion order from the filter pipeline is preserved - the output is
// built manually rather than via a map, so json.MarshalIndent's alphabetical
// key sort cannot silently override the pipeline's sort order.
func writeJSON(nodes []*yaml.RNode, out io.Writer) error {
	seen := make(map[string]bool, len(nodes))
	var buf bytes.Buffer
	buf.Grow(1 << 16)
	buf.WriteString("{\n")
	for i, node := range nodes {
		meta, err := node.GetMeta()
		if err != nil {
			return fmt.Errorf("read metadata for node %d: %w", i, err)
		}
		key := meta.Kind + "/" + meta.Name
		if seen[key] {
			return fmt.Errorf("duplicate object key %q at node %d: -json keys objects by \"<kind>/<name>\" without namespace, so two objects sharing kind and name (even across different namespaces) collide - this module still renders fine without -json; -json is a debugging aid (e.g. for fx/jq), not required output", key, i)
		}
		seen[key] = true

		jb, err := node.MarshalJSON()
		if err != nil {
			return fmt.Errorf("marshal json for node %d: %w", i, err)
		}
		kb, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("marshal json key for node %d: %w", i, err)
		}
		buf.WriteString("  ")
		buf.Write(kb)
		buf.WriteString(": ")
		// json.Indent appends to buf; its prefix ("  ") is prepended to every
		// line after the first, so continuation lines align two spaces under
		// the key. This reproduces the previous manual indentation (a second
		// json.Indent pass plus a bytes.ReplaceAll) in a single pass, with no
		// intermediate buffer and no \n rewriting.
		if err := json.Indent(&buf, jb, "  ", "  "); err != nil {
			return fmt.Errorf("indent json for node %d: %w", i, err)
		}
		if i < len(nodes)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	_, err := out.Write(buf.Bytes())
	return err
}

// writeKyaml serializes the filtered nodes to a YAML byte stream and
// re-encodes it as KYAML - flow-style YAML with double-quoted strings.
func writeKyaml(nodes []*yaml.RNode, out io.Writer) error {
	var buf bytes.Buffer
	for i, node := range nodes {
		if i > 0 {
			buf.WriteString("---\n")
		}
		s, err := node.String()
		if err != nil {
			return fmt.Errorf("serialize node %d for kyaml: %w", i, err)
		}
		buf.WriteString(s)
	}
	if err := (&kyaml.Encoder{}).FromYAML(&buf, out); err != nil {
		return fmt.Errorf("encode kyaml: %w", err)
	}
	return nil
}

// writeYaml serializes the filtered nodes as a "---"-separated YAML stream.
// When wide is true, sequence/list items are indented under their parent key
// (matching mikefarah/yq); otherwise they stay flush left (compact style).
func writeYaml(nodes []*yaml.RNode, out io.Writer, wide bool) error {
	opts := &yaml.EncoderOptions{}
	if wide {
		opts.SeqIndent = yaml.WideSequenceStyle
	}
	encoder := yaml.NewEncoderWithOptions(out, opts)
	for i, node := range nodes {
		if err := encoder.Encode(node.Document()); err != nil {
			encoder.Close()
			return fmt.Errorf("encode yaml for node %d: %w", i, err)
		}
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("flush yaml output: %w", err)
	}
	return nil
}

// sortByKindName sorts the document stream by .kind then .metadata.name,
// mirroring `yq -P eval-all '[.] | sort_by(.kind,.metadata.name) | .[]'.
type sortByKindName struct{}

func (sortByKindName) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	slices.SortStableFunc(nodes, func(a, b *yaml.RNode) int {
		am, _ := a.GetMeta()
		bm, _ := b.GetMeta()
		return cmp.Or(
			cmp.Compare(am.Kind, bm.Kind),
			cmp.Compare(am.Name, bm.Name),
		)
	})
	return nodes, nil
}

// requireConcrete validates every exported object recursively for
// concreteness before any YAML encoding starts. Without this check a
// non-concrete leaf (e.g. an unfilled `$value: string` hole) only surfaces
// as a cryptic encoder error - "yaml: unsupported node string (*ast.Ident)"
// - with no hint where in the chart the value lives. Validating up front
// reports the CUE path and source position of every offending field, and
// collects all of them in one pass so a broken chart can be fixed in a
// single round instead of one render per hole.
func requireConcrete(values []cue.Value) error {
	var errs cueerrors.Error
	for _, obj := range values {
		if err := obj.Validate(cue.Concrete(true)); err != nil {
			errs = cueerrors.Append(errs, cueerrors.Promote(err, "validate"))
		}
	}
	if errs == nil {
		return nil
	}
	// Details renders one block per offending field: the full CUE path,
	// the reason (e.g. "incomplete value string"), and the source position
	// on an indented follow-up line. Cwd shortens positions to relative
	// paths.
	cwd, _ := os.Getwd()
	details := strings.TrimRight(cueerrors.Details(errs, &cueerrors.Config{Cwd: cwd}), "\n")
	return fmt.Errorf("export contains non-concrete values, cannot render:\n%s", details)
}

// cueDetail renders a CUE error the way `cue eval`/`cue export` do: the full
// multi-line diagnosis with every conflicting value and source position, paths
// relativized to the CWD. err.Error() collapses this to a single headline plus
// "(and N more errors)", discarding exactly the detail a CUE-averse author
// needs to fix the problem (the bad value, the allowed values, the file:line).
// Used for build/load/lookup failures where there is no wrapper context to add
// the diagnosis themselves. Errors with no underlying CUE value (plain Go
// errors passed through by the CUE API) fall back to err.Error().
func cueDetail(err error) string {
	if err == nil {
		return ""
	}
	cwd, _ := os.Getwd()
	details := strings.TrimSpace(cueerrors.Details(err, &cueerrors.Config{Cwd: cwd}))
	if details == "" {
		return err.Error()
	}
	return details
}

// requireComplete force-evaluates the entire export.objects struct for
// concreteness, complementing requireConcrete's per-leaf check. It catches
// the case requireConcrete cannot: an object whose dynamic key (e.g.
// metadata.name derived from an unset optional value) is non-concrete.
// Such an object is never yielded by Fields(), so flattenObjects drops it
// silently. Validating the whole struct forces every dynamic key to resolve,
// surfacing the same diagnostic `cue export -e export.objects` produces
// ("key value of dynamic field must be concrete"). Called only after
// requireConcrete has cleared every materialized object, so any error here is
// a dropped object. Mirrors requireConcrete's Details formatting so both
// failure modes read identically.
func requireComplete(objs cue.Value) error {
	if err := objs.Validate(cue.Concrete(true)); err != nil {
		cwd, _ := os.Getwd()
		details := strings.TrimRight(cueerrors.Details(err, &cueerrors.Config{Cwd: cwd}), "\n")
		return fmt.Errorf("export contains non-concrete values, cannot render:\n%s", details)
	}
	return nil
}

// flattenObjects mirrors the `cue cmd exp` comprehension
//
//	[for _, kind in export.objects for _, obj in kind {yaml.Marshal(obj)}]
//
// i.e. it descends two levels (kind -> name -> object) and returns the leaf
// object values in iteration order. Objects are taken as values straight from
// the built tree - never re-embedded into a list - so hidden fields and
// dynamically generated secret data are not re-evaluated.
func flattenObjects(objs cue.Value) ([]cue.Value, error) {
	kinds, err := objs.Fields()
	if err != nil {
		return nil, fmt.Errorf("iterate kinds: %w", err)
	}
	var out []cue.Value
	for kinds.Next() {
		kind := kinds.Value()
		names, err := kind.Fields()
		if err != nil {
			return nil, fmt.Errorf("iterate objects of kind %q: %w", kinds.Selector(), err)
		}
		for names.Next() {
			out = append(out, names.Value())
		}
	}
	return out, nil
}

// exportPath resolves `cuegen.spec.export`, defaulting to defaultExportPath
// when unset. The default branch is taken so a `string | *"export.objects"`
// declaration collapses to its concrete value.
func exportPath(node cue.Value) (string, error) {
	p := node.LookupPath(cue.ParsePath("cuegen.spec.export"))
	if !p.Exists() {
		return defaultExportPath, nil
	}
	if err := p.Err(); err != nil {
		return "", fmt.Errorf("lookup cuegen.spec.export: %w", err)
	}
	d, _ := p.Default()
	s, err := d.String()
	if err != nil {
		return "", fmt.Errorf("cuegen.spec.export is not a string: %w", err)
	}
	return s, nil
}

// fileID identifies a file or directory by device and inode - the key for
// symlink-loop detection during the overlay walk.
type fileID struct{ dev, ino uint64 }

// maxOverlayDepth caps the overlay walk recursion as a belt-and-braces
// guard against actual symlink loops - deeper than any module tree
// observed in practice.
const maxOverlayDepth = 20

// maxFilterFileSize is the upper bound on file size the filter will read
// into memory. Larger files (e.g. multi-GB blobs in the module tree) are
// skipped: the module-wide walk makes encountering them realistic, and
// loading them wholesale would risk exhausting memory. 32 MiB comfortably
// exceeds any sops-encrypted CUE source or embedded data file.
const maxFilterFileSize = 32 << 20

// maxOverlayVisits caps the total number of files and directories the
// overlay walk visits. Without a global visited set (removed so each
// symlink alias path gets its own overlay entry), a maliciously nested
// symlink DAG can produce exponentially many paths while each individual
// chain stays within the depth budget. This cap turns that into a clear
// error instead of an unbounded walk. 1 M is far above any legitimate
// module tree. It is a var (not const) so tests can lower it.
var maxOverlayVisits = 1_000_000

// moduleRoot finds the module root by searching upward from the process's
// current working directory for a directory containing cue.mod. CUE unifies
// a directory's package with every ancestor up to the module root and loads
// @embed targets from anywhere in the module, so the overlay must cover
// the entire module tree - not just the render path argument (e.g. ./prod).
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "cue.mod")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no cue.mod found in %s or any ancestor", dir)
		}
		dir = parent
	}
}

// withinRoot reports whether p resolves to a path inside root. Regular
// (non-symlink) entries are assumed in-tree: the walk only descends into
// directories already known to be within root, so a non-symlink child is
// necessarily contained. A symlink is resolved and checked against root,
// rejecting targets that escape it - this is what keeps the overlay walk
// from traversing the host filesystem via an escaping symlink.
//
// A dangling or unreadable symlink is reported via the error so the caller
// can skip it the same way it skips a failed os.Stat.
func withinRoot(root, p string) (bool, error) {
	li, err := os.Lstat(p)
	if err != nil {
		return false, err
	}
	if li.Mode()&os.ModeSymlink == 0 {
		return true, nil
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return false, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// buildOverlay walks root (absolute path) for files, runs
// each through filter, and returns the absolute-path -> load.Source map for
// load.Config.Overlay, containing only files the filter actually changed.
//
// The walk follows symlinks (module trees use them heavily) but is confined
// to the module tree: a symlink whose target escapes root is skipped rather
// than descended into, so the walk never traverses the host filesystem.
// Symlink loops are detected via an ancestor stack: each directory's
// (device, inode) is checked against the chain of directories from the walk
// root to the current node, so recursing into an already-visited ancestor is
// skipped. Unlike a global visited set, this lets the same file reached via
// distinct symlink paths get its own overlay entry - CUE may load it through
// either path, and every path must resolve to the filtered content.
// cuegen is Unix-only (see the command doc), so stat always carries inode
// metadata.
func buildOverlay(root string, filter FileFilter) (map[string]load.Source, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// Normalize away any symlinks in the root path itself so the containment
	// check compares apples to apples: without this, a root reached through a
	// symlinked ancestor (/tmp -> /private/tmp on macOS) would make every
	// resolved child path look like it escaped, breaking the walk.
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, err
	}
	overlay := map[string]load.Source{}
	visits := 0
	var walk func(p string, depth int, ancestors []fileID) error
	walk = func(p string, depth int, ancestors []fileID) error {
		visits++
		if visits > maxOverlayVisits {
			return fmt.Errorf("overlay walk exceeded %d visited paths at %s (possible symlink DAG explosion)", maxOverlayVisits, p)
		}
		if depth > maxOverlayDepth {
			return fmt.Errorf("overlay walk exceeded depth %d at %s (possible symlink loop)", maxOverlayDepth, p)
		}
		info, err := os.Stat(p) // follows symlinks
		if err != nil {
			// A dangling symlink or unreadable entry: skip it rather than
			// aborting the entire render. The walk now covers the full
			// module tree (including cue.mod/pkg vendoring), where stray
			// symlinks are far more likely; CUE would ignore the dead
			// link anyway.
			return nil
		}
		// Confine the walk to the module tree. A symlink whose target
		// escapes absRoot would lead the walk into arbitrary host paths
		// (filesystem traversal / local DoS via the module-wide overlay
		// walk, especially relevant in the ArgoCD CMP context where the
		// rendered tree comes from a repo). Such a link is useless to CUE
		// - it never loads anything outside the module - so skip it
		// outright instead of reading the wider filesystem.
		inside, lerr := withinRoot(absRoot, p)
		if lerr != nil {
			return nil
		}
		if !inside {
			log.Printf("skipping %s: symlink target outside module root", p)
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("stat %s: no inode metadata (unsupported platform)", p)
		}
		id := fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}
		if info.IsDir() {
			// Cycle detection: if this directory is already an ancestor
			// on the current path, a symlink loop would cause infinite
			// recursion - skip it.
			if slices.Contains(ancestors, id) {
				return nil
			}
			// Skip VCS metadata. The walker would otherwise descend into
			// nested `.git` trees that live inside recursively-mounted
			// module sources (cue.mod/pkg/<dep>/cue.mod/pkg/<dep>/.git/…)
			// and consume both depth budget and time for files CUE can
			// never use.
			if filepath.Base(p) == ".git" {
				return nil
			}
			// Skip CUE vendoring directories under cue.mod/. Since CUE
			// 0.17, module dependencies are resolved via local-module.cue
			// rewrites, not the legacy pkg/gen/usr vendoring trees. Walking
			// them wastes time and risks hitting the large-file or depth
			// limits on deeply nested dependency copies.
			if filepath.Base(filepath.Dir(p)) == "cue.mod" {
				switch filepath.Base(p) {
				case "pkg", "gen", "usr":
					return nil
				}
			}
			entries, err := os.ReadDir(p)
			if err != nil {
				// An unreadable directory (e.g. cue.mod/gen with 0o000) is
				// skipped rather than aborting the entire render, mirroring the
				// graceful os.Stat handling above: the module-wide walk can hit
				// permission-restricted trees that CUE would ignore anyway.
				log.Printf("skipping unreadable directory %s: %v", p, err)
				return nil
			}
			ancestors = append(ancestors, id)
			for _, e := range entries {
				if err := walk(filepath.Join(p, e.Name()), depth+1, ancestors); err != nil {
					return err
				}
			}
			return nil
		}
		// Skip non-regular files (FIFOs, sockets, devices). os.ReadFile
		// on a FIFO blocks until a writer connects, hanging the render;
		// a socket or device read yields unpredictable bytes. CUE only
		// loads regular files, so these are irrelevant.
		if !info.Mode().IsRegular() {
			return nil
		}
		// Skip files too large to filter safely. The module-wide walk
		// can encounter multi-GB blobs; loading them into memory risks
		// exhaustion. CUE source files and sops-encrypted data are far
		// below the limit.
		if info.Size() > maxFilterFileSize {
			log.Printf("skipping %s (%d bytes, exceeds %d byte filter limit)",
				p, info.Size(), maxFilterFileSize)
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			// *fs.PathError already includes the path and operation.
			return err
		}
		filtered, err := filter(p, raw)
		if err != nil {
			return fmt.Errorf("filter %s: %w", p, err)
		}
		// The identity case (no encrypted file) is the common one: only
		// files the filter changed enter the overlay.
		if bytes.Equal(filtered, raw) {
			return nil
		}
		overlay[p] = load.FromBytes(filtered)
		return nil
	}
	if err := walk(absRoot, 0, nil); err != nil {
		return nil, err
	}
	return overlay, nil
}
