# YAML canonicalization in cuegen

## Overview

cuegen renders CUE modules into Kubernetes manifests as a YAML document
stream (separated by `---`). The output is routed through a filter
pipeline built on the
[`kyaml`](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml) library from the
Kustomize project. The pipeline provides canonical field ordering and a
stable document order.

Block YAML is emitted by default. With the `-kyaml` flag,
[KYAML](https://kubernetes.io/docs/reference/encodings/kyaml/) is emitted
instead - a flow-style YAML subset using `{}` for maps, `[]` for lists,
and double-quoted strings.

The `-sha1` and `-cmp-sha1` flags enable hash-based comparison of the
rendered output, e.g. for CI pipelines or cache invalidation.

## CLI flags

| Flag                                 | Description                                                                                                                                                                                               |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| (none)                               | Renders the module as block YAML to stdout                                                                                                                                                                |
| `-kyaml`                             | Renders as KYAML (flow-style) instead of block YAML                                                                                                                                                       |
| `-json`                              | Renders as a JSON object keyed by `<kind>/<name>` (deliberately without namespace). Suitable for `fx`/`jq`                                                                                                |
| `-sha1`                              | Prints only the SHA1 hash of the output (hex, with newline). Identical to `cuegen . \| sha1sum`. Suppresses the version banner                                                                            |
| `-cmp-sha1 <hash>`                   | Compares the output hash against `<hash>` (40 hex characters, case-insensitive). Exit 0 on match, exit 100 on mismatch, exit 1 on an invalid hash format. No stdout output. Suppresses the version banner |
| `-is-cuegen-dir`                     | ArgoCD CMP detection: prints `true` if `cuegen.cue` exists, nothing otherwise. Exit 0. Suppresses the version banner                                                                                      |
| `version` / `-version` / `--version` | Prints version, CUE version, and platform. No banner, no module required                                                                                                                                  |

All flags can be placed before or after the path and combined (e.g.
`cuegen -json -sha1 .`) - with two exceptions: `-kyaml`/`-json` and
`-sha1`/`-cmp-sha1` are each mutually exclusive (exit 1 with an error
message).

`-kyaml`, `-json`, `-sha1`, and `-cmp-sha1` are v2-only: if the module
falls back to the legacy binary, cuegen aborts with exit 1 and an error
message instead of silently dropping the flags - in particular,
`-cmp-sha1` must never exit 0 ("match") without having actually compared
anything.

v2 modules accept at most one positional argument (the module path).
Unknown flags (`--json`, typos like `-shal`) and extra arguments are
errors (exit 1) - otherwise they would be misinterpreted as a path or
silently discarded. On the legacy fallback, by contrast, all unrecognized
arguments are forwarded to the legacy binary unchanged.

If `cuegen.cue` does not exist in the current directory at all, cuegen
exits with an error (exit 1) instead of falling back to the legacy
binary - a directory without `cuegen.cue` isn't a cuegen module, legacy
or otherwise, so the legacy binary couldn't handle it either. Only a
`cuegen.cue` that exists but carries an older or missing
`cuegen.apiVersion` triggers the legacy fallback.

## Functionality

### 1. Canonical field ordering (`FormatFilter`)

Every YAML document is canonically formatted:

- **Mapping fields** are sorted by a fixed Kubernetes field preference
  (`apiVersion` → `kind` → `metadata` → `spec` → …), followed by
  alphabetical ordering of unknown fields.
- **Whitelisted lists** (e.g. `spec.template.spec.containers`) are sorted
  by their respective sort field (e.g. `name`).
- Scalar styles are corrected (e.g. values like `"true"` are properly
  quoted when the schema expects a string).

An object defined in CUE with fields in any order always appears in the
output in the same canonical order.

### 2. Document ordering (`sortByKindName`)

The documents in the output stream are sorted stably - primarily by
`.kind`, secondarily by `.metadata.name`. This corresponds to:

```
yq -P eval-all '[.] | sort_by(.kind,.metadata.name) | .[] | splitDoc'
```

Missing `kind` or `name` fields are treated as an empty string and sorted
accordingly.

### 3. Clean annotation handling

Internal reader annotations (`config.kubernetes.io/index`,
`config.kubernetes.io/path`, etc.) do not appear in the output.
`yaml.Parse` never sets such annotations, so there is nothing to strip.

### 4. Hash-based comparison (`-sha1`, `-cmp-sha1`)

The SHA1 hash is computed over the raw output bytes - the same bytes
`cuegen .` would write to stdout. The computation happens in-process: the
output is routed into a `bytes.Buffer` instead of stdout, then `sha1.Sum`
is applied to it.

```
cuegen -sha1 .          → 27b8b40e6fd5ef278dbf9dec1a82c956242ecc34
cuegen . | sha1sum      → 27b8b40e6fd5ef278dbf9dec1a82c956242ecc34  -
```

`-cmp-sha1` also renders into the buffer, but compares instead of
printing:

```go
if computed == cmpSHA1 {
    os.Exit(0)   // Match
}
os.Exit(100)     // Mismatch
```

The hash argument is validated before rendering: anything other than a
40-character hex string → usage error (exit 1 with an error message on
stderr). This clearly distinguishes a typo'd hash from a genuine
mismatch (exit 100). Uppercase hex is accepted and normalized to
lowercase before comparison, since `%x` formats the computed hash in
lowercase.

Both flags combine with `-kyaml` - the hash then refers to the KYAML
output.

### 5. Concreteness check before encoding

Before any YAML encoding starts, every exported object is validated
recursively for concreteness (`Value.Validate(cue.Concrete(true))`). A
non-concrete leaf - e.g. an unfilled `$value: string` hole - would
otherwise only surface as a cryptic encoder error
(`yaml: unsupported node string (*ast.Ident)`) with no hint where in the
chart the value lives.

All offending fields are collected in one pass and reported together,
each with its full CUE path, the reason, and the contributing source
positions (relative to the CWD):

```
cuegen: export contains non-concrete values, cannot render:
export.objects.configMap."cm-a".data.TOKEN: incomplete value string:
    ./export.cue:3:12
    ./export.cue:11:16
export.objects.deployment."dep-a".spec.replicas: incomplete value int:
    ./export.cue:4:12
    ./export.cue:17:19
```

Fields carrying a CUE default (`*"info" | "debug"`) are concrete for
export purposes and are not reported. Nothing is written to stdout on
error.

## Implementation

### File: `internal/engine/engine.go`

#### Path resolution

`Exec(path, ...)` resolves `path` relative to the process's current
working directory - exactly like `cue cmd exp <path>`. This is
deliberate: CUE unifies a directory's package with the same-named package
declared in every ancestor directory up to the module root, so
`cuegen ./prod` merges values defined in `./prod` with those in the CWD (see
`examples/webapp/prod`). An absolute path, or a path outside the enclosing
module's tree, is not supported - matching plain `cue` CLI behavior.
`buildOverlay` resolves `path` to an absolute path internally (overlay
keys must be absolute), independent of what `load.Instances` is given.

#### Data flow

```
CUE value → cueyaml.Encode → YAML bytes → yaml.Parse (one decode call)
    → collect []*yaml.RNode
    → call Filter.Filter() directly (×2)
    → output: ByteWriter (block YAML) or kyaml.Encoder (KYAML)
```

Each CUE value is serialized to YAML individually and turned into a
`*yaml.RNode` via `yaml.Parse` (a single decoder call). The filters
operate directly on `[]*yaml.RNode`; no `kio.Pipeline` or `kio.ByteReader`
infrastructure is needed.

#### CUE → RNode conversion

```go
nodes := make([]*yaml.RNode, 0, len(values))
for i, obj := range values {
    b, err := cueyaml.Encode(obj)       // CUE → YAML bytes
    node, err := yaml.Parse(string(b))  // YAML bytes → *yaml.RNode
    nodes = append(nodes, node)
}
```

`cueyaml.Encode` (from `cuelang.org/go/encoding/yaml`) serializes the CUE
value to YAML bytes. `yaml.Parse` (from `sigs.k8s.io/kustomize/kyaml/yaml`)
parses exactly one document - no multi-doc splitting, no annotation
assignment.

#### Filter pipeline

The filters are called directly in sequence:

```go
for _, f := range []kio.Filter{
    filters.FormatFilter{},    // canonical field/list ordering
    sortByKindName{},           // document ordering by kind, name
} {
    nodes, err = f.Filter(nodes)
}
```

Each filter implements the `kio.Filter` interface with the signature
`Filter([]*yaml.RNode) ([]*yaml.RNode, error)`.

#### Custom sort implementation

```go
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
```

The sort is stable (`SortStableFunc`), so objects with identical `kind`
and `name` retain their input order. Metadata extraction via `GetMeta()`
tolerates missing fields (returns empty strings).

#### Output

Default (block YAML):

```go
return (&kio.ByteWriter{Writer: stdout}).Write(nodes)
```

`kio.ByteWriter` handles the `---` separators, correct YAML encoding, and
sequence indentation.

With the `-kyaml` flag (KYAML flow-style):

```go
func writeKyaml(nodes []*yaml.RNode, stdout io.Writer) error {
    var buf bytes.Buffer
    for i, node := range nodes {
        if i > 0 {
            buf.WriteString("---\n")
        }
        s, err := node.String()
        buf.WriteString(s)
    }
    return (&kyaml.Encoder{}).FromYAML(&buf, stdout)
}
```

The filtered RNodes are serialized to YAML bytes and passed to
`kyaml.Encoder.FromYAML` (from `sigs.k8s.io/yaml/kyaml`). The encoder reads
the multi-document stream and emits each document as KYAML with a `---`
header. The canonical field ordering from `FormatFilter` is preserved,
since it is anchored in the node tree.

With the `-json` flag (JSON object):

```go
func writeJSON(nodes []*yaml.RNode, stdout io.Writer) error {
    obj := make(map[string]json.RawMessage, len(nodes))
    for i, node := range nodes {
        meta, _ := node.GetMeta()
        key := meta.Kind + "/" + meta.Name
        b, err := node.MarshalJSON()
        obj[key] = b
    }
    out, err := json.MarshalIndent(obj, "", "  ")
    _, err = stdout.Write(append(out, '\n'))
    return err
}
```

Each RNode is serialized to a JSON object via `MarshalJSON()`. The key is
`"<kind>/<name>"` - deliberately without the namespace, which has no
value for cuegen's use case. Two objects sharing kind and name (even in
different namespaces) are therefore a hard duplicate-key error.
Extraction happens via `GetMeta()`. Output uses 2-space indentation; the
canonical field ordering from `FormatFilter` is preserved, since
`MarshalJSON` carries over the node tree's field order.

### File: `cmd/cuegen/main.go`

#### Flag processing

The `-sha1`, `-cmp-sha1`, and `-is-cuegen-dir` flags are recognized early,
before the version banner is printed. These flags must produce no
diagnostic output.

`-kyaml`, `-sha1`, and `-cmp-sha1 <hash>` are filtered out of the argument
list so the remaining path is recognized correctly. All flags are
position-independent.

Every recognized flag is additionally recorded in `v2Flags`. `runLegacy`
checks this list before the `execve`: if it is non-empty, the legacy
fallback is refused with exit 1, since the legacy binary does not
understand the flags and they have already been stripped from `args` - a
silent fallback would render in the wrong format, or, for `-cmp-sha1`,
fake a hash match with exit 0.

`cuegen.cue` is always read from the process's current working directory
- never from the path argument. This is deliberate: CUE unifies a
directory's package with the same-named package declared in every
ancestor directory up to the module root, so `cuegen ./prod` merges values
defined in `./prod` with those in the CWD (see `examples/webapp/prod`, which
overrides a value hole declared in the parent package). The path argument
therefore names a value to unify into the current module, not a separate
module to switch into, and is passed straight to CUE relative to the CWD
- exactly like `cue cmd exp ./prod`. Passing an absolute path, or a path
outside the enclosing module's tree, is not supported, matching plain
`cue` CLI behavior.

If `readAPIVersion` fails because `cuegen.cue` does not exist
(`errors.Is(err, os.ErrNotExist)`), that is a hard error (`log.Fatalf`),
checked before the legacy-fallback branch - the CWD simply isn't a
cuegen module. Only a `cuegen.cue` that exists but fails to parse or has
no `apiVersion` field falls through to the legacy-fallback branch below.

Once the module is confirmed to be v2, the remaining arguments are
strictly validated: arguments with a `-` prefix are unknown flags (exit
1, `unknown flag`), more than one positional argument is an error (exit
1, `too many arguments`). This validation deliberately sits after
apiVersion detection, so the legacy path can forward arguments verbatim.
A `-cmp-sha1` without a value (at the end, or followed by another flag)
reports `missing value`.

#### Hash computation

```go
if hashOnly || cmpSHA1Set {
    var buf bytes.Buffer
    if err := engine.Exec(path, &buf, opts); err != nil {
        log.Fatalln(err)
    }
    sum := sha1.Sum(buf.Bytes())
    computed := fmt.Sprintf("%x", sum)
    if hashOnly {
        fmt.Println(computed)
        return
    }
    if computed == cmpSHA1 {
        os.Exit(0)
    }
    os.Exit(100)
}
```

The output is routed into a `bytes.Buffer` instead of stdout. The SHA1
hash is identical to `cuegen . | sha1sum`, since the same bytes underlie
both. `opts` is the `engine.Options` carrying the output format
(`FormatYAML`/`FormatKYAML`/`FormatJSON`) and the SOPS `FileFilter` - the
former package-level variable `engine.FileFilter` and the two bool
parameters have been replaced by this options struct.

### Dependencies

`go.mod`:

```
require (
    sigs.k8s.io/kustomize/kyaml v0.21.1   // FormatFilter, ByteWriter, RNode
    sigs.k8s.io/yaml v1.6.0                // kyaml.Encoder (KYAML output)
)

replace sigs.k8s.io/kustomize/kyaml => ../kustomize/kyaml
```

The `replace` directive points to the local Kustomize checkout in the
monorepo directory `../kustomize/kyaml`.

## Tests

### `cmd/cuegen/cli_flags_test.go`

Tests the CLI flags as a subprocess (the binary is compiled once per run,
since `main()` uses `os.Exit`):

| Test                                     | Verifies                                                                                                                             |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `TestSha1Flag`                           | `-sha1` prints only the SHA1 hex digest, no banner, identical to `cuegen \| sha1sum`                                                 |
| `TestSha1KyamlFlag`                      | `-sha1 -kyaml` combined, hash matches the KYAML output                                                                               |
| `TestCmpSha1Match`                       | `-cmp-sha1 <hash>` → exit 0 on match (including an uppercase hash), no output                                                        |
| `TestCmpSha1Mismatch`                    | `-cmp-sha1` with a valid but wrong hash → exit 100, no output                                                                        |
| `TestCmpSha1InvalidHash`                 | `-cmp-sha1` with an invalid hash (`deadbeef`, `wrong`, empty) → exit 1; missing value → `missing value`                              |
| `TestCmpSha1Kyaml`                       | `-cmp-sha1` with `-kyaml`: match → 0, mismatch → 100                                                                                 |
| `TestCmpSha1Json`                        | `-cmp-sha1` with `-json`: match → 0, mismatch → 100                                                                                  |
| `TestJsonFlag`                           | `-json` produces a valid JSON object with `<kind>/<name>` keys, correct count, consistent hash                                       |
| `TestMutuallyExclusiveFlags`             | `-sha1`+`-cmp-sha1` (both orders) and `-kyaml`+`-json` → exit 1, "mutually exclusive" on stderr                                      |
| `TestLegacyFallbackRejectsV2Flags`       | v2 flags + legacy module → exit 1, error names the flag and the v2 requirement                                                       |
| `TestMissingCuegenCueIsHardError`        | No `cuegen.cue` in the CWD → exit 1, no legacy fallback attempted (with or without flags)                                            |
| `TestLegacyFallbackWithoutFlags`         | A flag-free invocation still execs the legacy binary, args unchanged                                                                 |
| `TestLegacyFallbackForwardsUnknownFlags` | Unknown flags are forwarded verbatim to the binary on the legacy fallback                                                            |
| `TestUnknownFlagRejected`                | v2: `--json`, `-shal`, combined `-is-cuegen-dir` → exit 1 naming the flag                                                            |
| `TestExtraArgsRejected`                  | v2: more than one positional argument → exit 1, `too many arguments`                                                                 |
| `TestPathArgumentUnifiesWithCWD`         | `cuegen sub` reads `cuegen.cue` from the CWD, not `sub/`, and unifies `sub`'s package with the CWD's (a value hole only `sub` fills) |
| `TestVersionAliases`                     | `version`, `-version`, `--version` → version line, exit 0                                                                            |
| `TestIsCuegenDirSuppressesBanner`        | `-is-cuegen-dir` → only `true`, no banner                                                                                            |
| `TestNormalRenderShowsBanner`            | A normal invocation shows the `[INFO]` banner on stderr                                                                              |

### `internal/engine/engine_test.go`

Tests the engine directly (in-process):

| Test                              | Verifies                                                                                                                          |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `TestExecDefaultExportPath`       | Default export path, two documents, `---` separator                                                                               |
| `TestExecCustomExportPath`        | Custom export path via `cuegen.spec.export`                                                                                       |
| `TestExecMissingExportPath`       | Error on a nonexistent export path, no partial output                                                                             |
| `TestExecNonConcreteExport`       | Non-concrete fields → error listing every CUE path with source position before encoding; defaulted fields pass; no partial output |
| `TestExecSubdirUnifiesWithParent` | Loading a subdirectory unifies its package with the CWD's (a value hole only the subdirectory fills)                              |
| `TestExecJSONKeyScheme`           | JSON key is always `<kind>/<name>`, even with a namespace set                                                                     |
| `TestExecJSONDuplicateKindName`   | Same kind/name (even across two namespaces) → hard duplicate-key error                                                            |

## Examples

Two runnable v2 modules live under `examples/`:

### `examples/minimal/`

A minimal module with one ConfigMap. Serves as a smoke test.

### `examples/webapp/`

A Deployment, Service, and ConfigMap sharing `let` bindings. Demonstrates:

- Multiple objects as a `---`-separated YAML stream
- Canonical field ordering (apiVersion, kind, metadata, spec)
- Document ordering by `.kind` then `.metadata.name`

Each example ships golden files for comparison:

```
cuegen . | diff expected.yaml -
cuegen -kyaml . | diff expected.kyaml -
cuegen -json . | diff expected.json -
```
