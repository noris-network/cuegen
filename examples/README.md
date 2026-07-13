# Examples

Self-contained cuegen v2 modules demonstrating common patterns.

## Layout

Every example is a minimal CUE module with three files:

| File                  | Purpose                                             |
| --------------------- | --------------------------------------------------- |
| `cue.mod/module.cue`  | CUE module declaration                              |
| `cuegen.cue`           | `cuegen.apiVersion` + optional `cuegen.spec.export` |
| `export.cue`          | Objects to render under the export path             |

Some examples carry additional files (e.g. encrypted sources, sub-directories);
see the individual sections below.

## Examples

### minimal

A single ConfigMap - the smallest runnable module.

```
cuegen .
```

### webapp

A Deployment, Service and ConfigMap sharing values via `let` bindings.
Demonstrates:

- Multi-object rendering YAML stream
- Canonical field ordering (apiVersion, kind, metadata, spec)
- Document sorting by `.kind` then `.metadata.name`

```
cuegen .
```

### sops

Two SOPS-encrypted files decrypted transparently before CUE compilation:

- `secret.enc.cue` - encrypted CUE source (SOPS binary store, JSON envelope).
  Renders a Secret with `username`/`password` data fields.
- `config.enc.yaml` - encrypted YAML file (native SOPS YAML, inline
  `ENC[AES256_GCM,…]` values), embedded into CUE via
  `@embed(file="config.enc.yaml")`. Its values feed a ConfigMap
  (`DATABASE_HOST`, `DATABASE_PORT`, `API_KEY`).

cuegen decrypts both files in the file overlay before CUE sees the cleartext.
The `@embed` reads the decrypted YAML through the same overlay, so a single
SOPS hook covers both CUE source and embedded data. Both binary store (JSON
envelope) and native YAML sops formats are supported.

Decryption requires the age identity via `SOPS_AGE_KEY`:

```
export SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ
cuegen .
```

The key above is a throwaway demo key used only for this example.

## Verifying

Each example ships golden files for all output formats:

```
cuegen . | diff expected.yaml -
cuegen -kyaml . | diff expected.kyaml -
cuegen -json . | diff expected.json -
```

Hash verification:

```
cuegen -sha1 .
cuegen -cmp-sha1 <hash> .   # exit 0 on match, 100 on mismatch
```
