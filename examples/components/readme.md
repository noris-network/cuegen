## Import Components

cuegen.yaml:

    objectsPath: objects
    components:
    - ../component_a
    - ../component_b
    - ../component_c.zip
    - https://github.com/nxcc/cuegen-example-component-d?ref=v1

Components can be
  * local directories
  * zip files, the `ref` parameter allows to select tags or branches
  * references to git repositories

> *Local paths are not restricted. As this could be a security problem,
> this will change in a future release.*

This minimal example imports some components to the main chart. To
keep it simple, just a few ConfigMaps are imported. But components could
also be, e.g. complete Database deployments. Or the main chart can just
contain configuration data and all "real" objects like Deployments are
imported as component and configuration is applied. See example
"control-repository" (soon).

YAML output:

    data:
      page_a.html: <h1>Welcome to Page C</h1>
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: page-c
    ---
    data:
      page_a.html: <h1>Welcome to Page A</h1>
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: page-a
    ---
    data:
      page_a.html: <h1>Welcome to Page B</h1>
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: page-b
    ---
    data:
      page_a.html: |-
        <h1>Welcome to Page D</h1>
        Version 1
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: page-d


Run the example with

    cuegen examples/components/application
