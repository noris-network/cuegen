package engine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/inject/embed"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/yaml"
)

// defaultExportPath is the CUE path consulted for the per-module object
// collection when a module does not set `cuegen.spec.export` explicitly.
const defaultExportPath = "export.objects"

// FileFilter transforms the raw bytes of every CUE source file before the
// loader sees them. It receives the absolute file path (for context-aware
// decisions, e.g. detecting `.enc.cue`) and the raw bytes; it returns the
// bytes CUE should compile. Default is identity. Set this to plug in
// transparent decryption (SOPS, age, etc.) so CUE never knows a file was
// encrypted and loads the cleartext contents normally.
var FileFilter = func(path string, raw []byte) ([]byte, error) { return raw, nil }

// Exec renders the cuegen module rooted at path. It behaves like
// `cue cmd exp`: every object under the configured export path is serialized
// as its own YAML document and the documents are emitted as a "---"-separated
// stream to stdout.
//
// Unlike cuegen-ref, cuegen performs no $val scope plumbing, import
// composition or generator expansion — v2 modules express all of that in CUE
// itself. cuegen only loads (with transparent SOPS decryption via FileFilter)
// and exports.
func Exec(path string, stdout io.Writer) error {
	if !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "/") && path != "." {
		path = "./" + path
	}

	overlay, err := buildOverlay(path)
	if err != nil {
		return fmt.Errorf("build overlay for %q: %w", path, err)
	}
	cfg := &load.Config{Overlay: overlay}
	insts := load.Instances([]string{path}, cfg)
	if len(insts) == 0 {
		return fmt.Errorf("load instance %q: no instance returned", path)
	}
	inst := insts[0]
	if err := inst.Err; err != nil {
		return fmt.Errorf("load instance %q: %w", path, err)
	}

	ctx := cuecontext.New(cuecontext.WithInjection(embed.New()))
	val := ctx.BuildInstance(inst)
	if err := val.Err(); err != nil {
		return fmt.Errorf("build instance %q: %w", path, err)
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
		return fmt.Errorf("lookup %s: %w", expPath, err)
	}

	values, err := flattenObjects(objs)
	if err != nil {
		return fmt.Errorf("collect objects from %s: %w", expPath, err)
	}

	// Render every document first so a single encode failure produces no
	// partial output — matching `cue cmd exp`, which marshals the whole
	// comprehension before printing.
	docs := make([][]byte, 0, len(values))
	for i, obj := range values {
		b, err := yaml.Encode(obj)
		if err != nil {
			return fmt.Errorf("encode yaml for %s[%d]: %w", expPath, i, err)
		}
		docs = append(docs, b)
	}

	w := stdout
	for i, b := range docs {
		if i > 0 {
			if _, err := w.Write([]byte("---\n")); err != nil {
				return err
			}
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	if len(docs) > 0 {
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

// flattenObjects mirrors the `cue cmd exp` comprehension
//
//	[for _, kind in export.objects for _, obj in kind {yaml.Marshal(obj)}]
//
// i.e. it descends two levels (kind -> name -> object) and returns the leaf
// object values in iteration order. Objects are taken as values straight from
// the built tree — never re-embedded into a list — so hidden fields and
// dynamically generated secret data are not re-evaluated (see the rationale
// in case2/exp_tool.cue).
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
			return nil, fmt.Errorf("iterate objects of kind %q: %w", kinds.Label(), err)
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

// sameBytes returns true if a and b share the same backing array and length.
// This is the fast path for FileFilter: the identity filter returns its
// input slice unchanged, and we'd rather skip a full bytes.Equal on every
// CUE source file.
func sameBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

// buildOverlay walks `root` for files, runs each through FileFilter, and
// returns the absolute-path -> load.Source map for load.Config.Overlay. With
// CUE v0.17+ this overlay also covers @embed targets, so a single hook
// catches both cue source and embedded data.
//
// The walk follows symlinks (module trees use them heavily). Visited entries
// are deduplicated by (device, inode) when the platform exposes them — taken
// from the os.Stat we already perform, so there is no extra syscall — so the
// same underlying file reached via several symlink-distinct paths is read and
// filtered once. On platforms without a syscall.Stat_t the walked path string
// is used as the dedup key. As a belt-and-braces guard against actual symlink
// loops the recursion is capped at maxOverlayDepth — deeper than any module
// tree observed in practice.
const maxOverlayDepth = 20

func buildOverlay(root string) (map[string]load.Source, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	overlay := map[string]load.Source{}
	visited := map[string]bool{}
	var walk func(p string, depth int) error
	walk = func(p string, depth int) error {
		if depth > maxOverlayDepth {
			return fmt.Errorf("overlay walk exceeded depth %d at %s (possible symlink loop)", maxOverlayDepth, p)
		}
		info, err := os.Stat(p) // follows symlinks
		if err != nil {
			return fmt.Errorf("stat %s: %w", p, err)
		}
		// Dedup by inode when available (covers symlink-distinct paths to the
		// same file); fall back to the walked path otherwise.
		key := p
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			key = fmt.Sprintf("ino:%d:%d", st.Dev, st.Ino)
		}
		if visited[key] {
			return nil
		}
		visited[key] = true
		if info.IsDir() {
			// Skip VCS metadata. The walker would otherwise descend into
			// nested `.git` trees that live inside recursively-mounted
			// module sources (cue.mod/pkg/<dep>/cue.mod/pkg/<dep>/.git/…)
			// and consume both depth budget and time for files CUE can
			// never use.
			if filepath.Base(p) == ".git" {
				return nil
			}
			entries, err := os.ReadDir(p)
			if err != nil {
				return fmt.Errorf("read dir %s: %w", p, err)
			}
			for _, e := range entries {
				if err := walk(filepath.Join(p, e.Name()), depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		filtered, err := FileFilter(p, raw)
		if err != nil {
			return fmt.Errorf("filter %s: %w", p, err)
		}
		// Identity filter is the common case (no SOPS file). Skip the
		// content compare when the filter returned the same backing slice
		// and only fall back to bytes.Equal otherwise.
		if sameBytes(filtered, raw) || bytes.Equal(filtered, raw) {
			return nil
		}
		overlay[p] = load.FromBytes(filtered)
		return nil
	}
	if err := walk(absRoot, 0); err != nil {
		return nil, err
	}
	return overlay, nil
}
