package application

cuegen: {
	apiVersion: "v1alpha2"
	kind:       "CuegenPackage"
	metadata: name: "application"
	spec: {
		debug: !true
		imports: [
			{
				_x:      "libone"
				package: "example.com/pkgs/" + _x
				path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			},
			{
				_x:      "libtwo"
				package: "example.com/pkgs/" + _x
				path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			},
			{
				_x:      "libthree"
				package: "example.com/pkgs/" + _x
				path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			},
		]
	}
}
