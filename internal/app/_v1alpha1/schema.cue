cuegen: close({
	objectsPath: *"" | string
	checkPath:   *"" | string
	checkPaths?: [...string]
	secretDataPath: *"" | string
	debug:          *false | bool
	components?: [string, ...]
	rootIndex?:             >0 & <128
	enableOrderWorkaround?: bool
	// allow v1alpha2...
	apiVersion: *"" | string
	kind:       *"" | string
	metadata: {}
	spec: {}
})
