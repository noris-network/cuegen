[![release](https://img.shields.io/github/release/noris-network/cuegen.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/noris-network/cuegen)](https://goreportcard.com/report/github.com/noris-network/cuegen)
[![downloads](https://img.shields.io/github/downloads/noris-network/cuegen/total.svg)](https://github.com/noris-network/cuegen/releases)

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
CUE path and source position of every offending field, so a broken chart
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
# output a JSON array (suitable for piping into fx/jq):
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

As an ArgoCD CMP, a module is detected via the `-is-cuegen-dir` flag (checks
for the presence of `cuegen.cue`). The `-sha1` and `-cmp-sha1` flags suppress
the version banner; all flags can be placed before or after the path and
combined (e.g. `cuegen -json -sha1 .`), except that `-kyaml`/`-json` and
`-sha1`/`-cmp-sha1` are mutually exclusive.

All flags require a v2 module: if the module would fall back to the legacy
binary, cuegen aborts with an error instead of silently dropping them.
For v2 modules, unknown flags and more than one positional argument are
rejected; legacy modules receive their arguments verbatim.

## Quick start

A minimal cuegen module needs a `cue.mod/module.cue`, a `cuegen.cue` declaring
the API version, and the objects to render. No external libraries required

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

Output is canonically formatted (fields sorted by Kubernetes precedence,
documents sorted by `.kind` then `.metadata.name`) using the
[`kyaml`](https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml) library.

With `-kyaml`, output is encoded as
[KYAML](https://kubernetes.io/docs/reference/encodings/kyaml/) - flow-style
YAML with `{}`/`[]` and double-quoted strings:

```
$ cuegen -kyaml .
---
{
  apiVersion: "v1",
  kind: "ConfigMap",
  metadata: {
    name: "demo",
    namespace: "default",
  },
  data: {
    greeting: "Hello from cuegen!",
  },
}
```

With `-json`, output is a JSON object keyed by `<kind>/<metadata.name>` -
suitable for piping into tools like `fx` or `jq`:

```
$ cuegen -json .
{
  "ConfigMap/demo": {
    "apiVersion": "v1",
    "data": {
      "greeting": "Hello from cuegen!"
    },
    "kind": "ConfigMap",
    "metadata": {
      "name": "demo",
      "namespace": "default"
    }
  }
}
```

## SOPS / age

The transparent SOPS decryption (age recipients only) loads the age identity
from `SOPS_AGE_KEY`, `SOPS_AGE_KEY_FILE`, or
`$XDG_CONFIG_HOME/sops/age/keys.txt`. If a genuine SOPS file cannot be
decrypted (e.g. missing or rotated key), cuegen fails hard - encrypted values
are never passed through. A pure heuristic false positive (the markers
appearing by chance in a non-SOPS file) is silently passed through instead.
