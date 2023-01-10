## Kustomize plugin

If required, `cuegen` can be used as [kustomize][kust] plugin.
Currently only the deprecated ["exec plugin"][plug] method is supported.

When running on linux and `cuegen` is alredy in your `PATH`, just run:

    PLUGIN_DIR=${XDG_CONFIG_HOME:=$HOME}/kustomize/plugin/noris.net/mcs/v1beta1/cuegen
    mkdir -p $PLUGIN_DIR
    cp $(command -v cuegen) $PLUGIN_DIR/Cuegen

Run the example with

    kustomize build --enable-alpha-plugins examples/kustomize

[plug]:  https://kubectl.docs.kubernetes.io/guides/extending_kustomize/exec_plugins/
[kust]:  https://kustomize.io/
