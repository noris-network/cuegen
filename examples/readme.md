# Examples

* [configmap](configmap): load external data into a ConfigMap
* [values](values): load external structured data into a CUE value
* [encrypted](encrypted): load and decrypt [SOPS][SOPS]-encrypted data
* [kustomize](kustomize): use `cuegen` as kustomize plugin
* [components](components): compose several charts into one

To run these examples, put `cuegen` into your `PATH` and execute

    cuegen examples/<name-of-example-dir>

unless otherwise described in the example.

[SOPS]:   https://github.com/mozilla/sops