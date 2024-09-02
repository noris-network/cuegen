## Embed Experiment

By setting `apiVersion` to `v1alpha4` in `cuegen.cue` the experiment is enabled. 
`Cuegen` will decrypt all sops files, so values from `secrets.sops.yaml` will be
used.

Running `CUEGEN_SKIP_DECRYPT=true cuegen` will use values from `secrets.yaml`.
You could get the same result with

    cue export -e objects --out yaml | yq '.[] | split_doc'

which might be useful for (cue) debugging purposes.


To run this example you need to set

    SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ