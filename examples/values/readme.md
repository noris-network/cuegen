## Load structured data into a CUE value

CUE input file:

    package kube

    import "encoding/yaml"

    values: {} @read(values.yaml)

    configMap: "app-metadata": {
        apiVersion: "v1"
        kind:       "ConfigMap"
        data: {
            "about.html":  """
                <h1>Welcome to \(values.instance.hostname)
                    \(values.instance.environment) environment!
                </h1>
                <i>\(values.instance.replicas) replicas configured</i>
                """
            "values-dump": yaml.Marshal(values)
        }
    }

    objects: [ for v in configMap {v}]

values.yaml:

    instance:
      environment: production
      hostname: myapp.example.com
      replicas: 7


External structured data (YAML, JSON, .env) can be imported into a CUE value.
It can be used like any regular CUE value.

* `@read(values.yaml)` imports structured data into `values`.

YAML output:

    apiVersion: v1
    kind: ConfigMap
    data:
    about.html: |2-
        <h1>Welcome to myapp.example.com
            production environment!
        </h1>
        <i>7 replicas configured</i>
    values-dump: |
        instance:
        environment: production
        hostname: myapp.example.com
        replicas: 7
