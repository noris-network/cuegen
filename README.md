[![release](https://img.shields.io/github/release/noris-network/cuegen.svg)](https://github.com/noris-network/cuegen/releases)
[![release](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/noris-network/cuegen)](https://goreportcard.com/report/github.com/noris-network/cuegen)
[![downloads](https://img.shields.io/github/downloads/noris-network/cuegen/total.svg)](https://github.com/noris-network/cuegen/releases)

# Cuegen

Cuegen is a tool to build kubernetes resources with [CUE][CUE]. It solves the
problems of composing and versioning of "charts", as well as easily importing
external data (scripts, certificates, keys, etc.) into CUE charts. For this
purpose, it extends the rich possibilities that [CUE][CUE] already provides
with the ability to "compose" charts from various sources and load resources,
controlled by attributes.

> *If CUE and creating k8s resources with CUE is new to you, the
[CUE homepage][CUE], the [k8s tutorial][k8stut], as well as [examples][eg]
in this repository (in that order) are good starting points.*

## Table of Contents
- [Cuegen](#cuegen)
  - [Table of Contents](#table-of-contents)
  - [Features](#features)
  - [Install](#install)
  - [Usage](#usage)
  - [Configuration](#configuration)
  - [Environment Variables](#environment-variables)
  - [Components](#components)
  - [Attributes](#attributes)
    - [Attribute "@readfile"](#attribute-readfile)
    - [Attribute "@readmap"](#attribute-readmap)
    - [Attribute "@read"](#attribute-read)
      - [with file Attribute value](#with-file-attribute-value)
      - [with directory Attribute value](#with-directory-attribute-value)
  - [Order Workaround](#order-workaround)
  - [0.15.0 Embed Experiment](#0150-embed-experiment)
  - [Changelog](#changelog)

## Features
  * Compose manifests from various, versioned sources (local directories, git-repositories)
  * Load file contents into CUE values, e.g. a script into a ConfigMap key
  * Load whole directories as key/values data at once, e.g. into a ConfigMap
  * Load structured data (JSON/YAML/env) into a CUE struct
  * Automatically decrypt [SOPS][SOPS]-encrypted data
  * Load remote charts

## Install
Download the [latest release][rel] or build with *go1.20rc3 or later*:

    go install github.com/noris-network/cuegen@latest


## Usage

    cuegen path/to/cuegen.yaml
    # or
    cuegen path/to/directory-containing-cuegen-dot-yaml
    # or
    cuegen https://git.example.com/deployments/myapp.git

Have a look at the [examples][eg] for some ready-to-run examples.

Cuegen can be used as [config management plugin][cmp] in ArgoCD. A container
with the [`plugin.yaml`](docker/plugin.yaml) configuration
[is available at docker hub][cuegen-cmp]. The cuegen-cmp plugin container
expects the cuegen config file to be named `cuegen.yaml`.

## Configuration
A configuration file (preferred name: `cuegen.cue`, [schema][cfgschema]) is
required to run `cuegen`. For backwards compatibility the yaml format will still
be supported in the future.

    cuegen: {
      objectsPath:    "objects"          // this will be dumped to YAML
      secretDataPath: "secret.*.data"    // values matching this path will be loaded as []byte and
                                         //     automatically be base64 encoded when dumped to YAML
      checkPath:                         // this path is checked to contain only concrete values
      checkPaths: [                      // when more than one path needs to be checked, the
        "values",                        //     'objectsPath' with always be checked.
        "global",
      ]
      components: [                      // merge cue files from these soures into the main entrypoint
        "https://$GITLAB_TOKEN@gitlab.noris.net/mcs/components/cuegen/mongodb.git?ref=v6.0.4-mcs.1",
        "https://$GITLAB_TOKEN@gitlab.noris.net/mcs/components/cuegen/wekan.git?ref=v6.71-mcs.0",
      ]
      debug: false                       // print some info useful for debugging
    }

## Environment Variables
Some environment variables can help working with cuegen:

    CUEGEN_DEBUG              turn on debug output with "true"
    CUEGEN_HTTP_PASSWORD      password for git authentication
    CUEGEN_HTTP_USERNAME      username for git authentication
    DUMP_OVERLAYS_TO          directory to dump overlays to (debug only)
    SOPS_AGE_KEY              age key for decryption
    SOPS_AGE_KEY_FILE         age key file for decryption
    YQ_PRETTYPRINT            !="": run yaml output thru `yq -P`
                              starting with `/`: use as path to yq

## Components
Components can be

  * directories outside the main chart
  * zip files
  * git repositories

> *Local paths are not restricted. As this could be a security problem,
> this will change in a future release.*

Environment variables in components are [expanded][expenv], that can be used to
e.g. add authentication to git urls or prefix local paths. For cloning private
repositories via Http url, `CUEGEN_HTTP_USERNAME` and `CUEGEN_HTTP_PASSWORD` can
also be set in the environment.

Components Example:

    cuegen: components: [
      "../database-chart",
      "../common-static-resources.zip",
      "https://github.com/nxcc/cuegen-test-chart-one?ref=v0.1.0",
      "https://github.com/noris-network/cuegen?ref=v0.2.1#examples/configmap",
    ]

Because of the way CUE and cuegen work, all `*.cue` files need to be in the root
of the components directory.
No special files need to be present for a chart, and although cuegen does not
require or evaluate them, it is recommended to add metadata (see [examples][eg]).
For git repositories, a branch or tag name can be specified with the `ref`
parameter.
If a zip file has only one directory in it's root, this level is skipped and
cuegen uses all files in that directory.


## Attributes
The directory containing `cuegen.yaml` is the base directory, all paths are relative
to this.


### Attribute "@readfile"
Attribute `@readfile` loads contents of given file(s) into a CUE value. An optional
suffix controlls whitespace (`=nl`: ensure `\n` at end, `=trim`: trim whitespace
from begin and end).

Load `version.txt` into `Version`, trim any whitespace from begin and end:

    Version: string @readfile(version.txt=trim)

Load `my.crt` and `chain.crt` concatenated into `Certificate`. A `\n` is ensured
to be placed after `my.crt`:

    Certificate: string @readfile(my.crt=nl, chain.crt)


### Attribute "@readmap"
Attribute `@readmap` loads contents of given file(s) into a CUE value as key/value
data. When the cue path matches `secretDataPath`, values are inserted as []byte.
This will automatically base64 encode them when dumped to YAML. This is useful
for Secrets. This can also be forced by an `=bytes` suffix.

Load values from `data.json` as []bytes:

    secret: foo: {
      data: {} @readmap(data.json=bytes)
    }


### Attribute "@read"

#### with file Attribute value
Attribute `@read(filename)` tries to load structured data (JSON/YAML/env) into a
CUE struct.

Load key/value data from `env.env` and `data.yaml` into `configMap.myconfig.data`:

    configMap: "myconfig": {
	    data: {} @read(env.env, data.yaml)
    }


#### with directory Attribute value
Attribute `@read(directory)` tries to load regular files from the given directory
as key/values into a CUE struct.

Load all files from directory `scripts` as key/values into `configMap.scripts.data`:

    configMap: "scripts": {
	    data: {} @read(scripts)
    }

## Order Workaround
Until [issue 2555][issue2555] is resolved in CUE, there is a [temporary workaround](examples/workaround/).

## 0.15.0 Embed Experiment
Cuegen `0.15.0` is based on cue v0.10.0. When `apiVersion` is set to "v1alpha4", cue attribute handling
is removed in favour for cue [native embedding][cue-embed]. Right now sops encrypted files need to be
named like `<filename>.sops.<ext>` for formats supported by sops, otherwise like `<filename>.<ext>.sops`.
They will be temporarily decrypted to files named like `<filename>.<ext>`, and removed again after Yaml
output was generated [(usage example)][eg-embed].

All future releases at least until release 1.0.0 will be backwards compatible to current
non-experimental `cuegen` behaviour.

## 0.16.0
Cuegen `0.16.0` will be released after `cue` v0.12.0 is out.

## Changelog

  * `v0.1.0`  - Initial release
  * `v0.1.1`  - Improved attribute lookup
  * `v0.2.0`  - Added checks when reading `cuegen.yaml`
  * `v0.2.1`  - Improved error messages
  * `v0.3.0`  - Added ability to read subpaths from git repos
  * `v0.3.1`  - No code changes, trigger cmp build
  * `v0.3.2`  - No code changes, bump go version to 1.20
  * `v0.4.0`  - switch default config to `cuegen.cue`, use cue v0.5.0-beta5
  * `v0.4.1`  - Make components & checkPaths optional
  * `v0.4.2`  - downgrade cue to v0.5.0-beta.2 ([performance regression][gh2243])
  * `v0.4.3`  - fix running as kustomize plugin
  * `v0.4.4`  - improve handling of git urls
  * `v0.5.0`  - add cuegen default config
  * `v0.6.0`  - add dumpOverlays option
  * `v0.7.0`  - upgrade cue to v0.5.0 (many fixes, rare performance regression still present)
  * `v0.7.1`  - fix secret handling of @readfile
  * `v0.7.2`  - internal cleanup
  * `v0.8.0`  - allow remote cuegen directories, rm kustomize plugin support
  * `v0.9.0`  - upgrade cue to v0.6.0
  * `v0.10.0` - add `YQ_PRETTYPRINT` to filter output thru `yq -P`
  * `v0.11.0` - add `DUMP_OVERLAYS_TO` to dump overlays to directory (debug)
  * `v0.11.1` - upgrade sops
  * `v0.12.0` - add workaround for file order bug
  * `v0.13.0` - No code changes, bump cue version to 0.7.0
  * `v0.13.1` - bump deps
  * `v0.14.x` - "silent release" of v1alpha2 schema...
  * `v0.14.4` - No code changes, bump cue version to 0.8.0
  * `v0.14.5` - No code changes, bump cue version to 0.8.1
  * `v0.14.6` - No code changes, bump cue version to 0.8.2
  * `v0.15.0` - Introduce "embed experiment'
  * `v0.15.1` - No code changes, bump cue version to 0.11.0
  * `v0.15.2` - No code changes, bump cue version to 0.11.1

[CUE]:         https://cuelang.org
[SOPS]:        https://github.com/mozilla/sops
[k8stut]:      https://cuelang.org/docs/tutorials/
[eg]:          examples/
[eg-embed]:    examples/embed-experiment
[rel]:         https://github.com/noris-network/cuegen/releases/latest
[cmp]:         https://argo-cd.readthedocs.io/en/stable/operator-manual/config-management-plugins/#sidecar-plugin
[cuegen-cmp]:  https://hub.docker.com/r/nxcc/cuegen-cmp
[expenv]:      https://pkg.go.dev/os#ExpandEnv
[cfgschema]:   internal/app/schema.cue
[gh2243]:      https://github.com/cue-lang/cue/issues/2243
[issue2555]:   https://github.com/cue-lang/cue/issues/2555
[modules]:     https://cuelang.org/docs/reference/modules/
[cue-embed]:   https://cuelang.org/docs/howto/embed-files-in-cue-evaluation/