## Load Remote charts

As no local data is required, just run

    cuegen https://github.com/nxcc/cuegen-remote-test.git

to render the chart located in the given repository. Environment Variables in the
given URL are expanded. Like in components, `ref` and `#`-prefixed subpaths can be used:

    cuegen "https://github.com/nxcc/cuegen-remote-test.git?ref=subpath#apps/app_b"

