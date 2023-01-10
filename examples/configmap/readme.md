## Load data into a ConfigMap

CUE input file:

    package kube

    configMap: [ID=_]: {metadata: name: ID}

    configMap: "app-metadata": {
        apiVersion: "v1"
        kind:       "ConfigMap"
        data: {
            version: string @readfile(version.txt=trim)
        } @readmap(meta.yaml)
    }

    configMap: "scripts": {
        apiVersion: "v1"
        kind:       "ConfigMap"
        data: {} @readmap(scripts)
    }

    objects: [ for v in configMap {v}]

External data, e.g. buildinfo or scripts can be imported into a ConfigMap.

* `@readfile(version.txt=trim)` reads the whole file, and, because of the `=trim`
  suffix, removes any leading and trailing whitespace
* `@readmap(meta.yaml)` reads the given YAML file as key/value data
  (readmap only allows one level) and adds it to the `data` struct
* `@readmap(scripts)` reads all files from the `scripts` directory as key/values
  into the ConfigMap

Using multiple attributes for the structure `data` is ok, this does not lead to
conflicts, as long as the loaded data itself does not conflict.

YAML output:

    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: app-metadata
    data:
      version: v1.0.0
      vcs.modified: "false"
      vcs.revision: 54f9ca
      vcs.time: "2023-01-10T12:00:00Z"
      vcs: git
      CGO_ENABLED: "0"
      go: go1.20
    ---
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: scripts
    data:
      start.sh: |
        #!/bin/bash
        echo "starting demo-app..."
      stop.sh: |
        #!/bin/bash
        echo "stopping demo-app..."
