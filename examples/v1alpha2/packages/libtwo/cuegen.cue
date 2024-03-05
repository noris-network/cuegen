package libtwo

cuegen: {
	apiVersion: "v1alpha2"
	kind:       "CuegenLibrary"
	metadata: {
		name:    "libtwo"
		version: "0.2.0"
	}
	spec: {
		debug: !true
		// packages: [
		// 	{
		// 		_x:      "application"
		// 		package: "example.com/pkgs/" + _x
		// 		path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
		// 	},
		// ]
		// xximports: [
		// 	{
		// 		_x:      "application"
		// 		package: "example.com/pkgs/" + _x
		// 		path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
		// 	},
		// ]
	}
}

values: {
	bbb: "3xb from two"
}
