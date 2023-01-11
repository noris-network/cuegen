[![Release Cuegen](https://github.com/noris-network/cuegen/actions/workflows/release.yaml/badge.svg)](https://github.com/noris-network/cuegen/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/noris-network/cuegen)](https://goreportcard.com/report/github.com/noris-network/cuegen)

# Cuegen

Cuegen is a tool to build kubernetes resources with [CUE][CUE]. For this purpose,
it extends the rich possibilities that [CUE][CUE] already provides out of the box
with the ability to load resources, controlled by attributes.

If CUE and creating k8s resources with CUE is new to you, the [k8s tutorial][k8stut]
on the [CUE homepage][CUE] as well as [examples][eg] in this repository are good
starting points.

## Features
  * Load file contents into CUE values, e.g. a script into a ConfigMap key
  * Load whole directories at once into a CUE struct, e.g. a ConfigMap as key/values
  * Load structured data (JSON/YAML/env) into a CUE struct
  * Automatically decrypt [SOPS][SOPS]-encrypted data
  * Merge CUE files from different sources (local directories, git-repositories)


## Usage
Cuegen can be used stand-alone or as generator in [kustomize][kust]
([example](examples/kustomize/)).


## Configuration
A configuration file (preferred name: `cuegen.yaml`) is required to run `cuegen`.

    # (required) objects from this cue path will be dumped to YAML
    objectsPath: objects

    # (optional) all values in this path are checked to be concrete values
    checkPath: values

    # (optional) same as 'checkPath', in case more than one path needs to be checked
    checkPaths:
      - values
      - morevalues

    # (optional) values in this path are loaded as []byte. This will
    # automatically base64 encode them when dumped to YAML
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

Example:

    components:
      - ../database-chart
      - ../common-static-resources.zip
      - https://github.com/nxcc/cuegen-test-chart-one?ref=v0.1.0

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


[CUE]:    https://cuelang.org
[SOPS]:   https://github.com/mozilla/sops
[kust]:   https://kustomize.io/
[k8stut]: https://cuelang.org/docs/tutorials/
[eg]:     examples/