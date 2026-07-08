[![release](https://img.shields.io/github/release/noris-network/cuegen.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/noris-network/cuegen)](https://goreportcard.com/report/github.com/noris-network/cuegen)
[![downloads](https://img.shields.io/github/downloads/noris-network/cuegen/total.svg)](https://github.com/noris-network/cuegen/releases)

# Cuegen

cuegen renders [CUE](https://cuelang.org) modules into Kubernetes manifests
(YAML) and can be used as a Config Management Plugin in ArgoCD.

## What's new

This version supports `cuegen.apiVersion` **`v2`**. A v2 module is
rendered by a new, lean engine, every object under the configured export path 
is serialized as its own YAML document and emitted as a `---`-separated stream.

The export path is read from `cuegen.spec.export` (default `export.objects`).
SOPS-encrypted files (age) are transparently decrypted before loading — CUE
never notices and works with the cleartext.

## CUE version

cuegen now tracks the current [CUE](https://cuelang.org) release (currently
v0.17.0, up from the previously pinned v0.12.0). Since cuegen is a thin
wrapper around CUE without much "magic", it aims to follow CUE releases
closely going forward.

## Backward compatibility

**From a user perspective nothing changes.** Modules with an older or missing
`cuegen.apiVersion` (e.g. `v1beta1`, `v1alpha4`) are fully delegated to the
legacy binary. cuegen replaces its own process via `execve` with the legacy
binary — same PID, same stdin/stdout/stderr, same exit code. The behavior is
identical to having invoked the legacy binary directly.

If the legacy binary is not found in `PATH`, cuegen aborts with a pointer to
the release page:

```
cuegen: legacy binary "cuegen_v0.16.8" not found in PATH: ...
install it from https://github.com/noris-network/cuegen/releases/tag/v0.16.8
and ensure it is executable and on your PATH
```

## Old version (maintenance mode)

The previous cuegen version is in maintenance mode and receives **bugfixes and
security updates only** — no new features.

The full documentation of the old version (configuration, attributes such as
`@readfile`/`@readmap`/`@read`, components, environment variables) can be
found at:

<https://github.com/noris-network/cuegen/blob/legacy-api-version-lt-2/README.md>

## Installation

```
go install github.com/noris-network/cuegen@latest
```

For the legacy fallback, `cuegen_v0.16.8` is additionally required
([download](https://github.com/noris-network/cuegen/releases/tag/v0.16.8))
if modules with `apiVersion < v2` still need to be rendered.

## Usage

```
cuegen path/to/module
# or from within the module directory:
cuegen .
```

As an ArgoCD CMP, a module is detected via the `-is-cuegen-dir` flag (checks
for the presence of `cuegen.cue`).

## SOPS / age

The transparent SOPS decryption (age recipients only) was adopted from the
predecessor version. The age identity is loaded from `SOPS_AGE_KEY`,
`SOPS_AGE_KEY_FILE`, or `$XDG_CONFIG_HOME/sops/age/keys.txt`. If a genuine
SOPS file cannot be decrypted (e.g. missing or rotated key), cuegen fails
hard — encrypted values are never passed through. A pure heuristic false
positive (the markers appearing by chance in a non-SOPS file) is silently
passed through instead.
