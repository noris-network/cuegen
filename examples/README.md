# Examples

Self-contained cuegen v2 modules demonstrating common patterns.

## Layout

Every example is a minimal CUE module with three files:

| File                 | Purpose                                             |
| -------------------- | --------------------------------------------------- |
| `cue.mod/module.cue` | CUE module declaration                              |
| `cuegen.cue`         | `cuegen.apiVersion` + optional `cuegen.spec.export` |
| `export.cue`         | Objects to render under the export path             |

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
