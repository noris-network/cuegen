## Workaround file order bug in cue

Until [issue 2555][issue2555] is resolved in CUE, here is a temporary 
workaround. When `enableOrderWorkaround` is `true`, components are loaded
in the order they are listed. 

When you hit the bug, enable the workaround with `enableOrderWorkaround`, and 
try changing the order of components. With `rootIndex` the loading position of 
the root Directory (the one containg `cuegen.cue`) can be changed.

It is also advisable to set this option for new charts as this prevents the 
chart from suddenly breaking unpredictably due to small changes.

```
package kube

cuegen: {
	components: [
		"https://example.com/cuegen-components/comp-a",
		"https://example.com/cuegen-components/comp-b",
		"https://example.com/cuegen-components/comp-c",
	]
	enableOrderWorkaround: true
	rootIndex:             0 // the default
}
```

The order of components can be checked with:
```
CUEGEN_DEBUG=true cuegen . >/dev/null
```

[issue2555]:   https://github.com/cue-lang/cue/issues/2555
