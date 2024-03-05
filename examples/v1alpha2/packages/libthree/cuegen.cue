package libthree

cuegen: {
	apiVersion: "v1alpha2"
	kind:       "CuegenLibrary"
	metadata: {
		name:    "libthree"
		version: "0.3.0"
	}
	spec: {
		debug: !true
	}
}

values: {
	ccc: "3xc from three"
	sig: signature
}

signature: string
