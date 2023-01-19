# Control Repository

This example shows one way to implement the GitOps approach with cuegen.
A control repository contains definitions of instances of an application for
different environments. The application, in this example [wekan][wekan], is
assembled using the versioned sub-charts ([wekan][wekanChart], [mongodb][mongodbChart]).
This is controlled by `cuegen.yaml` which references components and versions.


## Charts

["Cuegen-Charts"](charts) contain k8s objects (Deployments, ConfigMaps, etc.) that
define something useful, in this example mongodb and wekan. For now all cue resources
have to live in the `package kube`. Objects are defined like in other examples, e.g.

    deployment.mongodb: {...}
    configMap: config: {...}

In addition to that they should have

    values: {
        mongodb: close({
            key: <value-type>
        })
    }

This will require these values to be set in the main cuegen chart.

>Sidenote: Charts can only be composed into a new chart, charts that import
other sub-charts are not supported.

Finally, it is suggested to add metadata to charts

    chart: mongodb: {
        version:    "v1.0.0"
        appVersion: "6.0.3"
    }

When all included charts have these set, the ConfigMap `about-components`, defined
in [control/about.cue](control/about.cue) contains a list of installed components.

## Example Environments

The application consists of two charts, wekan and mongodb, which are joined with
`cuegen.yaml`. Depending on the environment, the charts are located in the local
file system (dev) or are pulled from git repositories with tagged versions
(qa, prod).

### Example Files

    control-repository/
    ├── charts                           # demo charts
    │   ├── mongodb
    │   │   ├── mongodb.cue              # k8s objects
    │   │   └── values.cue               # value definitions
    │   ...
    │
    ├── control                          # control repository root
    │   ├── about.cue                    # build "about-components" ConfigMap
    │   ├── cue.mod                      # required for cue
    │   ├── demo.cue.template            # demo config
    │   ├── dev-cluster
    │   │   ├── dev-cluster.cue          # global per-cluster settings
    │   │   └── wekan-dev                # wekan dev instance
    │   │       ...
    │   ├── generators_v0.cue            # generators, version "v0"
    │   ├── global.cue                   # required global values
    │   ├── kube.cue                     # basic k8s object definitions
    │   └── prod-cluster
    │       ├── prod-cluster.cue         # cluster wide settings
    │       ├── wekan-prod               # global per-cluster settings
    │       │   ├── app-config.cue       # application values...
    │       │   ├── cuegen.yaml          # instance cuegen config
    │       │   └── mongodb-auth.yaml    # some encrypted data
    ...

*  `charts` contains the demo charts, these will be included as components in the
   `control/dev-cluster/cuegen.yaml` file. In contrast `qa` and `prod`
   environments include components from git repositories ([wekan][wekanChart],
   [mongodb][mongodbChart]), versions are pinned by using git tags.
*  `control/kube.cue` basic k8s object definitions for this example. For a real setup
   this needs to be expanded to include all object kinds in use and `k8s.io/api/core/v1`
   should be imported. Objects could then be validated by, e.g.,<br>
   `configMap: "my-configmap": v1.ConfigMap & {`.


### Run example
To run this example you need to set

    SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ

in the environment, otherwise `mongodb-auth.yaml` could not be decrypted. In addition to
that execute

    cp demo.cue.template demo.cue

and, in case you want to deploy the demo, adjust `demo.cue` to your environment. For building
the "prod" manifest, execute

    cuegen examples/control-repository/control/prod-cluster/wekan-prod


[mongodbChart]: https://github.com/nxcc/cuegen-example-mongodb
[wekanChart]: https://github.com/nxcc/cuegen-example-wekan
[wekan]: https://wekan.github.io/

### Generators
Some "generators" are used (`generators_v0.cue`). The charts only define
"incomplete" `Deployment`s which are "finalized" by "generators".

*  `Deployment`: `spec.template.spec.volumes` is automatically added by looking
   at `...volumeMounts`
*  `PersistentVolumeClaim`s: automatically derived from `...volumeMounts/_pvc`
   in `Deployments`
* `Service`s: automatically derived from `...containers.ports` in `Deployments`
* `Ingress`es: automatically derived from `...ports._ingress`

Both charts define `_useGenerators: "v0"` as a "guard" which ensures that the
same version of generator is used. This simple example will certainly not cover
all usage scenarios, but it should show the power of creating k8s manifests with
CUE.
