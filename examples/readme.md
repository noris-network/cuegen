# Examples

* [configmap](configmap): load external data into a ConfigMap
* [values](values): load external structured data into a CUE value
* [encrypted](encrypted): load and decrypt [SOPS][SOPS]-encrypted data
* [components](components): compose several charts into one
* [control-repository](control-repository): full example how to install several
  instances of an application in various environments
* [remote](remote): render chart from remote repository

To run these examples, put `cuegen` into your `PATH` and execute

    cuegen examples/<name-of-example-dir>

unless otherwise described in the example.

[SOPS]:   https://github.com/mozilla/sops