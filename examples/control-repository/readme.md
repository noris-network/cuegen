# Control Repository

The following example shows one way to implement the GitOps approach with cuegen.
A control repository contains definitions of instances of an application for
different environments. The application, in this example [wekan][wekan], is
assembled using the versioned sub-charts ([wekan][wekanChart], [mongodb][mongodbChart]).
This is controlled by `cuegen.yaml` which references components and versions.

## Charts

["Cuegen-Charts"](charts) contain k8s objects (Deployments, ConfigMaps, etc.) that
define something useful, in this example mongodb and wekan. For now all cue resources
have to live in  `package kube`. Objects are defined like in other examples, e.g.

    deployment.mongodb: {}
    configMap: config: {}

In addition to that they, should have

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
in [control/about.cue](control/about.cue) contains a list of installed components


## Example Files

    control-repository/
    ├── charts                           # demo charts
    │   ├── mongodb
    │   │   ├── mongodb.cue              # k8s objects
    │   │   └── values.cue               # value definitions
    │   ...
    │
    ├── control
    │   ├── about.cue                    # creates "about-components" ConfigMap
    │   ├── cue.mod                      # required for cue
    │   ├── demo.cue.template            # demo config
    │   ├── dev-cluster
    │   │   ├── dev-cluster.cue          # cluster wide settings
    │   │   └── wekan-dev                # wekan dev instance
    │   ├── generators_v0.cue
    │   ├── global.cue                   # define required global values
    │   ├── kube.cue                     # basic k8s object definitions
    │   └── prod-cluster
    │       ├── prod-cluster.cue         # cluster wide settings
    │       ├── wekan-prod               # wekan prod instance
    │       │   ├── app-config.cue       # values...
    │       │   ├── cuegen.yaml          # cuegen config
    │       │   └── mongodb-auth.yaml    # some encrypted data
    ...

*  `charts` contains the demo charts, these are included as components in the
   `control/dev-cluster/cuegen.yaml` file. In contrast `qa` and `prod`
   environments include components from git repositories ([wekan][wekanChart],
   [mongodb][mongodbChart]), versions are pinned by using git tags.
*  `generators_v0.cue` defines generators in version `v0`
*  `control/kube.cue` basic k8s object definitions for this example. For a real setup
   this needs to be expanded to include all object kinds in use and `k8s.io/api/core/v1`
   should be imported. Objects could then be validated by, e.g.,
   `configMap: "my-configmap": v1.ConfigMap & {`.


To run this example you need to set

    SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ

in the environment.


[mongodbChart]: https://github.com/nxcc/cuegen-example-mongodb
[wekanChart]: https://github.com/nxcc/cuegen-example-wekan
[wekan]: https://wekan.github.io/
