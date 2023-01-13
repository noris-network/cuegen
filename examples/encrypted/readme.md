## Load encrypted data

CUE input file:

    package kube

    secret: [ID=_]: {
        apiVersion: "v1"
        kind:       "Secret"
        metadata: name: ID
        type: string | *"Opaque"
        data: {[string]: bytes}
    }

    secret: "tls": {
        type: "kubernetes.io/tls"
        data: {} @readmap(tls)
    }

    values: {} @read(secret-values.yaml)

    secret: "auth": {
        data: {
            DSN:        'postgresql://\(data.DB_USER):\(data.DB_PASS)@dbhost:5432'
            AUTH_TOKEN: '\(values.app."auth-token")'
        } @readmap(auth.env)
    }

    objects: [ for v in secret {v}]

[SOPS][SOPS]-encrypted plain files or structured data (YAML, JSON, .env) can be
imported into CUE values or structs. It can be used like any regular CUE value.
`secretDataPath: secret.*.data` needs to be set in `cuegen.yaml`. Otherwise
data in secrets would not be loaded as `bytes`. Values in `secrets.*.data` have to
be quoted using `'`, otherwise they would be handled as `string`.

* `@readmap(tls)` imports encrypted key and certificate into secret `tls`
* `@readmap(auth.env)` reads encrypted .env data into secret `auth`
* `@read(secret-values.yaml)` reads encrypted structured multi-level data

YAML output:

    apiVersion: v1
    kind: Secret
    metadata:
    name: tls
    type: kubernetes.io/tls
    data:
      tls.crt: |-
        LS0tLS1CRUdJTiBDRVJUSUZJQ0FUR...UVORCBDRVJUSUZJQ0FURS0tLS0t
      tls.key: |-
        LS0tLS1CRUdJTiBQUklWQVRFIEtFW...kQgUFJJVkFURSBLRVktLS0tLQo=
    ---
    apiVersion: v1
    kind: Secret
    metadata:
    name: auth
    type: Opaque
    data:
      DSN: cG9zdGdyZXNxbDovL2FkbWluOjUzY3IzdEBkYmhvc3Q6NTQzMg==
      DB_USER: YWRtaW4=
      DB_PASS: NTNjcjN0
      AUTH_TOKEN: c21obHJrdjYtZW50ODJoeGQtdG41NnVwb2ctdnppbG1wdmctaHdxdzQ3dTk=

To run this example you need set

    SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ

in the environment. (what is [age][age]?)

[SOPS]:   https://github.com/mozilla/sops
[age]:    https://age-encryption.org
