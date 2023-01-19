[![Release Cuegen](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/noris-network/cuegen)](https://goreportcard.com/report/github.com/noris-network/cuegen)

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
  * [Features](#features)
  * [Install](#install)
  * [Usage](#Usage)
  * [Configuration](#configuration)
  * [Components](#components)
  * [Attributes](#attributes)
  * [Changelog](#changelog)

## Features
  * Compose manifests from various, versioned sources (local directories, git-repositories)
  * Load file contents into CUE values, e.g. a script into a ConfigMap key
  * Load whole directories as key/values data at once, e.g. into a ConfigMap
  * Load structured data (JSON/YAML/env) into a CUE struct
  * Automatically decrypt [SOPS][SOPS]-encrypted data

## Install
[![Releases](https://img.shields.io/github/release/noris-network/cuegen.svg)](https://github.com/noris-network/cuegen/releases)
[![Releases](https://img.shields.io/github/downloads/noris-network/cuegen/total.svg)](https://github.com/noris-network/cuegen/releases)

Download the [latest release][rel] or build with *go1.20rc3 or later*:

    go install github.com/noris-network/cuegen@latest

To use cuegen as kustomize plugin, find instructions in the [kustomize example][kusteg].

## Usage
Cuegen can be used stand-alone or as generator in [kustomize][kust]
(see [example](examples/kustomize/)).

    cuegen path/to/cuegen.yaml
    # or
    cuegen path/to/directory-containing-cuegen-dot-yaml

Have a look at the [examples][eg] for some ready-to-run examples.

## Configuration
A configuration file (preferred name: `cuegen.yaml`) is required to run `cuegen`.

    # (required) objects from this cue path will be dumped to YAML
    objectsPath: objects

    # (optional) all values in this path are checked to be concrete values
    checkPath: values

    # (optional) same as 'checkPath', in case more than one path needs to be
    # checked. The 'objectsPath' with always be checked.
    checkPaths:
      - values
      - morevalues

    # (optional) values in this path are loaded as []byte. This will
    # automatically base64 encode values when dumped to YAML
    secretDataPath: secret.*.data

    # (optional, default: false) print some info useful for debugging
    debug: false

    # (optional) merge cue files from these soures into the main entrypoint
    components:
      - component1
      - component2

## Components
Components can be

  * directories outside the main chart
  * zip files
  * git repositories

> *Local paths are not restricted. As this could be a security problem,
> this will change in a future release.*

Example:

    components:
      - ../database-chart
      - ../common-static-resources.zip
      - https://github.com/nxcc/cuegen-test-chart-one?ref=v0.1.0
      - https://github.com/noris-network/cuegen?ref=v0.2.1#examples/configmap

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


## Changelog

  * `v0.1.0` - Initial release
  * `v0.1.1` - Improved attribute lookup
  * `v0.2.0` - Added checks when reading `cuegen.yaml`
  * `v0.2.1` - Improved error messages
  * `v0.3.0` - Added ability to read subpaths from git repos

[CUE]:         https://cuelang.org
[SOPS]:        https://github.com/mozilla/sops
[kust]:        https://kustomize.io/
[k8stut]:      https://cuelang.org/docs/tutorials/
[eg]:          examples/
[rel]:         https://github.com/noris-network/cuegen/releases/latest
[kusteg]:      examples/kustomize