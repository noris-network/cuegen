# Examples

Self-contained cuegen v2 modules demonstrating common patterns.

## Layout

Every example is a minimal CUE module with three files:

| File                 | Purpose                                             |
| -------------------- | --------------------------------------------------- |
| `cue.mod/module.cue` | CUE module declaration                              |
| `cuegen.cue`         | `cuegen.apiVersion` + optional `cuegen.spec.export` |
| `export.cue`         | Objects to render under the export path             |

Some examples carry additional files (e.g. encrypted sources, subdirectories);
see the individual sections below.

## Examples

### minimal

A single ConfigMap - the smallest runnable module.

```
cuegen .
```

### webapp

A Deployment, Service, and ConfigMap sharing values via `let` bindings.
Demonstrates:

- Multi-object rendering as a `---`-separated YAML stream
- Canonical field ordering (apiVersion, kind, metadata, spec)
- Document sorting by `.kind` then `.metadata.name`
- Subdirectory unification: `prod/` and `dev/` each fill a value hole left
  open by the parent package

The parent package deliberately leaves `ENVIRONMENT` unset, so it has no
renderable form of its own - a bare `cuegen .` here fails concreteness
validation on purpose. Render it through an environment directory instead:

```
cuegen ./prod
cuegen ./dev
```

The path argument names a value to unify into the current module, not a
module to switch into: `./prod` is merged with the package in the current
directory. The golden files describe `cuegen ./prod`.

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
SOPS hook covers both CUE source and embedded data. Both the binary-store
(JSON envelope) and native-YAML SOPS formats are supported.

Decryption requires the age identity via `SOPS_AGE_KEY`:

```
export SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ
cuegen .
```

The key above is a throwaway demo key used only for this example.

## Verifying

Each example ships golden files for all output formats. Substitute the
example's own render argument for `<path>` - `.` for minimal and sops,
`./prod` for webapp (see above):

```
cuegen <path> | diff expected.yaml -
cuegen -kyaml <path> | diff expected.kyaml -
cuegen -json <path> | diff expected.json -
```

These comparisons run in CI as `TestExamplesMatchGoldenFiles`
(`cmd/cuegen/examples_golden_test.go`), so a golden file that drifts from
what the module renders fails the build. Keep that test's table and this
section in step.

Digest verification:

```
cuegen -hash <path>
cuegen -cmp-hash <algo:hex> <path>   # exit 0 on match, 100 on mismatch
```