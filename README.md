[![release](https://img.shields.io/github/release/noris-network/cuegen.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://img.shields.io/github/downloads/noris-network/cuegen/total.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)

# Cuegen

cuegen renders [CUE](https://cuelang.org) modules into Kubernetes manifests
(YAML) and can be used as a Config Management Plugin in ArgoCD.

A v2 module is rendered by a lean engine: every object under the configured
export path is serialized as its own YAML document and emitted as a
`---`-separated stream. The output is canonically formatted and sorted using
the [`kyaml`](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml) library from the
Kustomize project.

The export path is read from `cuegen.spec.export` (default `export.objects`).
SOPS-encrypted files (age) are transparently decrypted before loading - CUE
never notices and works with the cleartext.

Non-concrete values are rejected before encoding: the error lists the full
CUE path and source position of every offending field, so a broken module
can be fixed in a single round.

Modules whose `cuegen.cue` exists but carries an older or missing
`cuegen.apiVersion` (e.g. `v1beta1`, `v1alpha4`) are delegated to the
`cuegen_v0.16.8` binary via `execve` - same PID, same stdin/stdout/stderr,
same exit code. If that binary is not found in `PATH`, cuegen aborts with a
pointer to the release page.

If `cuegen.cue` doesn't exist in the current directory at all, cuegen exits
with an error instead - such a directory isn't a cuegen module, legacy or
otherwise.

## CUE version

cuegen tracks the current [CUE](https://cuelang.org) release (currently
v0.17.0). Since cuegen is a thin wrapper around CUE without much "magic", it
aims to follow CUE releases closely.

## Installation

```
go install github.com/noris-network/cuegen@latest
```

If modules with `apiVersion < v2` need to be rendered, `cuegen_v0.16.8` is
additionally required
([download](https://github.com/noris-network/cuegen/releases/tag/v0.16.8)).

## Usage

```
cuegen path/to/module
# or from within the module directory:
cuegen .
# output KYAML (flow-style) instead of block YAML:
cuegen -kyaml .
# output a JSON object keyed by <kind>/<name> (suitable for fx/jq):
cuegen -json .
# indent list items under their parent key (yq-style):
cuegen -wide .
# or via env var (e.g. for ArgoCD CMP where CLI flags aren't easily passed):
CUEGEN_WIDE=true cuegen .
# print only the SHA1 hash of the output (no banner, no YAML):
cuegen -sha1 .
# compare the output hash against a known value (40 hex chars):
#   exit 0 on match, exit 100 on mismatch, exit 1 on a malformed hash
cuegen -cmp-sha1 <hash> .
```

### CLI flags

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

## Quick start

A minimal cuegen module needs a `cue.mod/module.cue`, a `cuegen.cue` declaring
the API version, and the objects to render. No external libraries required.

`cue.mod/module.cue`:

```cue
module: "demo.local"
language: version: "v0.17.0"
```

`cuegen.cue`:

```cue
package demo

cuegen: {
	apiVersion: "v2"
	spec: export: "export.objects"
}
```

`export.cue`:

```cue
package demo

export: objects: configMap: demo: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "demo"
		namespace: "default"
	}
	data: greeting: "Hello from cuegen!"
}
```

Render it:

```
$ cuegen .
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
  namespace: default
data:
  greeting: Hello from cuegen!
```

The `export.objects` struct maps `kind` to `name` to object. cuegen flattens
that two-level nesting and emits each object as its own YAML document in a
`---`-separated stream - the same format `kubectl apply -f -` expects.

## Output canonicalization

Output is routed through a filter pipeline built on the
[`kyaml`](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml) library from the
Kustomize project, giving canonical field ordering and a stable document
order.

Block YAML is emitted by default. With the `-kyaml` flag,
[KYAML](https://kubernetes.io/docs/reference/encodings/kyaml/) is emitted
instead - a flow-style YAML subset using `{}` for maps, `[]` for lists,
and double-quoted strings.

### Canonical field ordering (`FormatFilter`)

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

### Document ordering (`sortByKindName`)

The documents in the output stream are sorted stably - primarily by
`.kind`, secondarily by `.metadata.name`. This corresponds to:

```
yq -P eval-all '[.] | sort_by(.kind,.metadata.name) | .[] | splitDoc'
```

Missing `kind` or `name` fields are treated as an empty string and sorted
accordingly.

### Clean annotation handling

Internal reader annotations (`config.kubernetes.io/index`,
`config.kubernetes.io/path`, etc.) do not appear in the output.
`yaml.Parse` never sets such annotations, so there is nothing to strip.

## Drift detection (`-sha1`, `-cmp-sha1`)

The `-sha1` and `-cmp-sha1` flags enable hash-based comparison of the
rendered output, e.g. for CI pipelines or cache invalidation.

The SHA1 hash is computed over the raw output bytes - the same bytes
`cuegen .` would write to stdout. The computation happens in-process: the
output is routed into a `bytes.Buffer` instead of stdout, then `sha1.Sum`
is applied to it.

```
cuegen -sha1 .          → 27b8b40e6fd5ef278dbf9dec1a82c956242ecc34
cuegen . | sha1sum      → 27b8b40e6fd5ef278dbf9dec1a82c956242ecc34  -
```

`-cmp-sha1` also renders into the buffer, but compares instead of
printing: exit 0 on match, exit 100 on mismatch.

The hash argument is validated before rendering: anything other than a
40-character hex string → usage error (exit 1 with an error message on
stderr). This clearly distinguishes a typo'd hash from a genuine
mismatch (exit 100). Uppercase hex is accepted and normalized to
lowercase before comparison, since `%x` formats the computed hash in
lowercase.

Both flags combine with `-kyaml` - the hash then refers to the KYAML
output.

## Concreteness check before encoding

Before any YAML encoding starts, every exported object is validated
recursively for concreteness (`Value.Validate(cue.Concrete(true))`). A
non-concrete leaf - e.g. an unfilled `$value: string` hole - would
otherwise only surface as a cryptic encoder error
(`yaml: unsupported node string (*ast.Ident)`) with no hint where in the
module the value lives.

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

### Incomplete dynamic keys (silent-drop guard)

A non-concrete *leaf* is only half the story. An object whose **dynamic key**
is non-concrete — typically `metadata.name` derived from an unset optional
value injected through an opaque/open struct (`$val: _`) — is never yielded by
iteration, so it would vanish from the output without an error, exit 0. `cue
export -e export.objects` reports the same state loudly; cuegen now does too.

After the per-leaf check, the entire `export.objects` struct is force-evaluated
with `Concrete(true)`. This resolves every dynamic key and surfaces the same
diagnostic `cue export` produces, instead of dropping the object silently:

```
cuegen: export contains non-concrete values, cannot render:
export.objects.AWX.<dynamic>: key value of dynamic field must be concrete, found _|_(...):
    ./export.cue:...
```

So an object that "disappears" from the rendered manifest is now a hard,
located failure — fixing the pain point where a forgotten optional value
silently dropped a Custom Resource from the output.

## SOPS / age

The transparent SOPS decryption (age recipients only) loads the age identity
from `SOPS_AGE_KEY`, `SOPS_AGE_KEY_FILE`, or
`$XDG_CONFIG_HOME/sops/age/keys.txt`. If a genuine SOPS file cannot be
decrypted (e.g. missing or rotated key), cuegen fails hard - encrypted values
are never passed through. A pure heuristic false positive (the markers
appearing by chance in a non-SOPS file) is silently passed through instead.

## Implementation

### Data flow

```
CUE value → cueyaml.Encode → YAML bytes → yaml.Parse (one decode call)
    → collect []*yaml.RNode
    → call Filter.Filter() directly (×2)
    → output: block YAML / KYAML / JSON
```

Each CUE value is serialized to YAML individually and turned into a
`*yaml.RNode` via `yaml.Parse` (a single decoder call). The filters
operate directly on `[]*yaml.RNode`; no `kio.Pipeline` or `kio.ByteReader`
infrastructure is needed.

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

### Filter pipeline

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
`Filter([]*yaml.RNode) ([]*yaml.RNode, error)`. The custom document sort is
stable (`SortStableFunc`), so objects with identical `kind` and `name`
retain their input order; metadata extraction via `GetMeta()` tolerates
missing fields (returns empty strings).

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

### Output formats

- **Block YAML** (default): the `yaml.Encoder` handles the `---` separators,
  correct YAML encoding, and sequence indentation (`-wide` opts into the
  yq-style wide sequence indent).
- **KYAML** (`-kyaml`): the filtered RNodes are serialized to a YAML byte
  stream and passed to `kyaml.Encoder.FromYAML`, which emits each document as
  KYAML with a `---` header. The canonical field ordering from `FormatFilter`
  is preserved, since it is anchored in the node tree.
- **JSON** (`-json`): each RNode is serialized to a JSON object via
  `MarshalJSON()`. The key is `"<kind>/<name>"` - deliberately without the
  namespace, which has no value for cuegen's use case. Two objects sharing
  kind and name (even in different namespaces) are therefore a hard
  duplicate-key error. The canonical field ordering from `FormatFilter` is
  preserved, since `MarshalJSON` carries over the node tree's field order.

### Path resolution

`Exec(path, ...)` resolves `path` relative to the process's current
working directory - exactly like `cue cmd exp <path>`. This is
deliberate: CUE unifies a directory's package with the same-named package
declared in every ancestor directory up to the module root, so
`cuegen ./prod` merges values defined in `./prod` with the CWD (see
`examples/webapp/prod`, which overrides a value hole declared in the
parent package). The path argument names a value to unify into the
current module, not a separate module to switch into. Passing an absolute
path, or a path outside the enclosing module's tree, is not supported -
matching plain `cue` CLI behavior.

`cuegen.cue` is always read from the process's current working directory,
never from the path argument. If `readAPIVersion` fails because
`cuegen.cue` does not exist (`errors.Is(err, os.ErrNotExist)`), that is a
hard error - the CWD simply isn't a cuegen module. Only a `cuegen.cue`
that exists but fails to parse or has no `apiVersion` field falls through
to the legacy-fallback branch.

### Flag processing

The `-sha1`, `-cmp-sha1`, and `-is-cuegen-dir` flags are recognized early,
before the version banner is printed. These flags must produce no
diagnostic output.

`-kyaml`, `-sha1`, and `-cmp-sha1 <hash>` are filtered out of the argument
list so the remaining path is recognized correctly. All flags are
position-independent. Every recognized flag is additionally recorded in
`v2Flags`; `runLegacy` checks this list before the `execve`: if it is
non-empty, the legacy fallback is refused with exit 1, since the legacy
binary does not understand the flags and they have already been stripped
from `args` - a silent fallback would render in the wrong format, or, for
`-cmp-sha1`, fake a hash match with exit 0.

Once the module is confirmed to be v2, the remaining arguments are
strictly validated: arguments with a `-` prefix are unknown flags (exit 1,
`unknown flag`), more than one positional argument is an error (exit 1,
`too many arguments`). This validation deliberately sits after apiVersion
detection, so the legacy path can forward arguments verbatim. A
`-cmp-sha1` without a value (at the end, or followed by another flag)
reports `missing value`.

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
| `TestExecDropsIncompleteDynamicKey` | Non-concrete dynamic key (metadata.name from an unset optional value) → hard error matching `cue export`, no silent drop; with the value set both objects render |
| `TestExecSubdirUnifiesWithParent` | Loading a subdirectory unifies its package with the CWD's (a value hole only the subdirectory fills)                              |
| `TestExecJSONKeyScheme`           | JSON key is always `<kind>/<name>`, even with a namespace set                                                                     |
| `TestExecJSONDuplicateKindName`   | Same kind/name (even across two namespaces) → hard duplicate-key error                                                            |

## Examples

Runnable v2 modules live under `examples/`:

### `examples/minimal/`

A minimal module with one ConfigMap. Serves as a smoke test.

### `examples/sops/`

A SOPS/age-encrypted secret, demonstrating transparent in-memory decryption
of `.enc.cue` and `.enc.yaml` files.

### `examples/webapp/`

A Deployment, Service, and ConfigMap sharing `let` bindings, plus a `prod/`
subdirectory that overrides a value hole declared in the parent package.
Demonstrates:

- Multiple objects as a `---`-separated YAML stream
- Canonical field ordering (apiVersion, kind, metadata, spec)
- Document ordering by `.kind` then `.metadata.name`
- Subdirectory unification with the CWD's package

Each example ships golden files for comparison:

```
cuegen . | diff expected.yaml -
cuegen -kyaml . | diff expected.kyaml -
cuegen -json . | diff expected.json -
```