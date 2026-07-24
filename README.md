[![release](https://img.shields.io/github/release/noris-network/cuegen.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://img.shields.io/github/downloads/noris-network/cuegen/total.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)

# Cuegen

cuegen renders [CUE](https://cuelang.org) modules into Kubernetes manifests
(YAML) and can be used as a Config Management Plugin in ArgoCD.

Every object under the configured export path is serialized as its own YAML
document and emitted as a `---`-separated stream. The output is canonically
formatted and sorted, so a module written with fields in any order always
renders identically.

The export path is read from `cuegen.spec.export` (default `export.objects`).
SOPS-encrypted files (age) are transparently decrypted before loading, so CUE
works with the cleartext and never notices the file was encrypted.

Errors are reported with enough detail to fix them in a single round:
non-concrete values, CUE validation failures, dropped objects, and empty
exports all fail hard with the CUE path and source position of the offending
field, rather than producing partial or silently empty output.

## Legacy modules

Modules whose `cuegen.cue` carries an older or missing `cuegen.apiVersion`
(e.g. `v1beta1`, `v1alpha4`) are delegated to the `cuegen_v0.16.8` binary,
preserving stdin/stdout/stderr and the exit code. If that binary is not on
`PATH`, cuegen aborts with a pointer to the release page.

A directory without a `cuegen.cue` at all is not a cuegen module: cuegen exits
with an error rather than falling back to the legacy binary.

## CUE version

cuegen tracks the current [CUE](https://cuelang.org) release (currently
v0.17.1). Since cuegen is a thin wrapper around CUE without much "magic", it
aims to follow CUE releases closely.

## Installation

```
go install github.com/noris-network/cuegen@latest
```

If modules with `apiVersion < v2` need to be rendered, `cuegen_v0.16.8` is
additionally required
([download](https://github.com/noris-network/cuegen/releases/tag/v0.16.8)).

## Usage

The usual invocation is a bare `cuegen` from the module root (equivalent to
`cuegen .`). Passing a subdirectory - `cuegen path/to/module` - pulls that
subdirectory into the evaluation and unifies its contents with the module
root, exactly as `cue path/to/module` does.

```
# from the module root:
cuegen

# output KYAML (flow-style) instead of block YAML:
cuegen -kyaml .

# output a JSON object keyed by <kind>/<name> (suitable for fx/yq/jq):
cuegen -json .

# indent list items under their parent key (yq-style):
cuegen -wide .

# or via env var (e.g. for ArgoCD CMP where CLI flags aren't easily passed):
CUEGEN_WIDE=true cuegen .

# print only the digest of the output, as "sha256:<hex>" (no YAML):
cuegen -hash .

# compare the output digest against a known value:
#   exit 0 on match, exit 100 on mismatch, exit 1 on a malformed digest
cuegen -cmp-hash <algo:hex> .
```

### CLI flags

| Flag                                 | Description                                                                                                                                                                                               |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| (none)                               | Renders the module as block YAML to stdout                                                                                                                                                                |
| `-kyaml`                             | Renders as KYAML (flow-style) instead of block YAML                                                                                                                                                       |
| `-json`                              | Renders as a JSON object keyed by `<kind>/<name>` (deliberately without namespace). Suitable for `fx`/`yq`/`jq`                                                                                                |
| `-wide`                              | Indents sequence items under their parent key (yq-style) instead of flush-left. YAML output only                                                                                                          |
| `-hash`                              | Prints only the digest of the output, as `sha256:<hex>` (with newline)                                                                                                                                    |
| `-cmp-hash <algo:hex>`               | Compares the output digest against `<algo:hex>` (or an `algo:<12+ hex chars>` prefix), case-insensitive. Exit 0 on match, exit 100 on mismatch (expected/actual digest on stderr), exit 1 on a malformed digest. No stdout output |
| `-is-cuegen-dir`                     | ArgoCD CMP detection: prints `true` if `cuegen.cue` exists, nothing otherwise. Exit 0                                                                                                                     |
| `-version`                           | Prints version, CUE version, and platform. No module required                                                                                                                                             |

Flags can be placed before or after the path and combined (e.g.
`cuegen -json -hash .`), with two exceptions: `-kyaml`/`-json` and
`-hash`/`-cmp-hash` are each mutually exclusive (exit 1 with an error).

On a v2 module, unknown flags (`--json`, typos like `-hsah`) and more than one
positional argument are errors (exit 1). The formatting and digest flags
(`-kyaml`, `-json`, `-hash`, `-cmp-hash`) are v2-only: if the module falls back
to the legacy binary, cuegen aborts rather than silently dropping them. On the
legacy fallback all unrecognized arguments are instead forwarded to the legacy
binary unchanged.

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

## Path resolution

`cuegen ./prod` resolves the path relative to the current working directory
and unifies the `./prod` package with the same-named package in every ancestor
directory up to the module root - exactly like `cue cmd exp ./prod`. So values
declared in the current directory are merged with those in `./prod` (see
`examples/webapp/prod`, which overrides a value hole declared in the parent
package). The path argument names a value to unify into the current module,
not a separate module to switch into.

`cuegen.cue` is always read from the current working directory, never from the
path argument. Passing an absolute path, or a path outside the enclosing
module's tree, is not supported - matching plain `cue` CLI behavior.

## Output

Block YAML is emitted by default. `-kyaml` selects
[KYAML](https://kubernetes.io/docs/reference/encodings/kyaml/) - a flow-style
YAML subset using `{}` for maps, `[]` for lists, and double-quoted strings.
`-json` selects a JSON object keyed by `<kind>/<name>`.

Every document is canonically formatted and the stream is stably ordered, so
the same CUE input always renders byte-identically:

- **Fields** are ordered by a fixed Kubernetes preference
  (`apiVersion` → `kind` → `metadata` → `spec` → …), then alphabetically for
  the rest. Scalar styles are corrected (e.g. a string `"true"` is quoted).
- **Whitelisted lists** (e.g. `spec.template.spec.containers`) are sorted by
  their sort field (e.g. `name`).
- **Documents** are sorted by `.kind`, then `.metadata.name`. A missing `kind`
  or `name` sorts as an empty string.

With `-json`, the key is `<kind>/<name>` without the namespace. Two objects
sharing kind and name (even in different namespaces) are therefore a
duplicate-key error, even though the same module renders fine as YAML/KYAML.
`-json` is a debugging aid for piping into `fx`/`yq`/`jq`, not a format every
module is expected to support.

## Validation and error reporting

Every exported object is validated for concreteness before any YAML encoding
starts. A non-concrete leaf - e.g. an unfilled `$value: string` hole - would
otherwise only surface as a cryptic encoder error. All offending fields are
collected in one pass and reported together, each with its full CUE path,
reason, and source position:

```
cuegen: export contains non-concrete values, cannot render:
export.objects.configMap."cm-a".data.TOKEN: incomplete value string:
    ./export.cue:3:12
    ./export.cue:11:16
export.objects.deployment."dep-a".spec.replicas: incomplete value int:
    ./export.cue:4:12
    ./export.cue:17:19
```

Fields carrying a CUE default (`*"info" | "debug"`) are concrete for export
purposes and are not reported. Nothing is written to stdout on error.

CUE validation failures (enums, types, required fields) are reported with the
full multi-line diagnosis `cue eval` produces - the conflicting values and
source positions, not a truncated `(and N more errors)` headline.

Two failure modes that would otherwise pass silently are also hard errors:

- An object whose **dynamic key** is non-concrete - typically `metadata.name`
  derived from an unset optional value - would vanish from the output with no
  error and exit 0. cuegen force-evaluates the whole `export.objects` struct
  and reports the same diagnostic `cue export -e export.objects` does, so a
  dropped Custom Resource becomes a located failure instead.
- An `export.objects` that resolves to **zero objects** is rejected rather than
  emitted as an empty stream. For a caller like ArgoCD an empty stream is
  indistinguishable from "nothing to render" and would prune the entire
  Application.

## Drift detection (`-hash`, `-cmp-hash`)

`-hash` and `-cmp-hash` compare the digest of the rendered output, e.g. for CI
pipelines or cache invalidation. This is a change-detection aid, **not a
security feature**: the threat model is accidental output change (a bumped
dependency altering a manifest), not an adversarial collision. The digest
carries its algorithm as a prefix (`sha256:<hex>`) so a future algorithm change
does not require a CLI rename.

The digest is computed over the exact bytes `cuegen .` would write to stdout:

```
cuegen -hash .                → sha256:5a83a6c36a52dec6fce78bbddbac70a0923f50f6661f28869fb154b421bea0c9
cuegen . | sha256sum          → 5a83a6c36a52dec6fce78bbddbac70a0923f50f6661f28869fb154b421bea0c9  -
```

`-cmp-hash` compares instead of printing: exit 0 on match, exit 100 on
mismatch. On a mismatch both digests are reported on stderr, so a CI job sees
what the module actually rendered to without a separate `-hash` run:

```
cuegen: digest mismatch: expected sha256:deadbeef..., got sha256:5a83a6c3...
```

The digest argument is validated before rendering, so a typo'd digest (exit 1)
is clearly distinguishable from a genuine mismatch (exit 100):

| Input                              | Result                                          |
| ---------------------------------- | ----------------------------------------------- |
| `sha256:<64 hex>`                  | Full comparison                                 |
| `sha256:<n hex>`, n ≥ 12           | Prefix comparison                               |
| Anything without an `algo:` prefix | Error, exit 1                                   |
| `<unknown>:<hex>`                  | Error, exit 1 (names the supported algorithms)  |

Uppercase hex is accepted and normalized. The prefix form mirrors how Git and
Docker shorten hashes: full hex is always computed internally, only the
comparison is shortened. Both flags combine with `-kyaml` and `-json`; the
digest then refers to that output.

For unchanged CUE input, the digest is stable across **patch** releases of
cuegen. A minor or major bump may change canonical formatting or a CUE/kyaml
dependency - and therefore the digest - and would call that out in the release
notes.

## SOPS / age

SOPS-encrypted files (age recipients only) are decrypted transparently in
memory before CUE loads them. The age identity is read from `SOPS_AGE_KEY`,
`SOPS_AGE_KEY_FILE`, or `$XDG_CONFIG_HOME/sops/age/keys.txt`.

If a genuine SOPS file cannot be decrypted (e.g. a missing or rotated key),
cuegen fails hard - encrypted values are never passed through into a rendered
manifest. Files that are not SOPS files are loaded unchanged.

## Examples

Runnable v2 modules live under `examples/`:

- **`examples/minimal/`** - one ConfigMap; a smoke test.
- **`examples/sops/`** - a SOPS/age-encrypted secret, demonstrating
  transparent in-memory decryption of `.enc.cue` and `.enc.yaml` files.
- **`examples/webapp/`** - a Deployment, Service, and ConfigMap sharing `let`
  bindings, plus a `prod/` subdirectory that overrides a value hole declared in
  the parent package. Demonstrates a multi-object stream, canonical field and
  document ordering, and subdirectory unification with the current directory.

Each example ships golden files for comparison:

```
cuegen . | diff expected.yaml -
cuegen -kyaml . | diff expected.kyaml -
cuegen -json . | diff expected.json -
```
