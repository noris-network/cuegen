package main

cuegen: {
	apiVersion: "v1alpha2"
	kind:       "Cuegen"
	metadata: {
		appVersion: "1.0.0"
		pkgVersion: "dev.1"
		name:       "myapp-dev"
	}
	spec: {
		debug: !true
		packages: [
			// {
			// 	_x:      "application"
			// 	package: "example.com/pkgs/" + _x
			// 	path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			// },
			// {
			// 	_x:      "database"
			// 	package: "example.com/pkgs/" + _x
			// 	path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			// },
			{
				_x:      "cache"
				package: "example.com/pkgs/" + _x
				path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			},
		]
		imports: [
			// {
			// 	_x:      "libone"
			// 	package: "example.com/pkgs/" + _x
			// 	path:    "/home/mlangner/workspace/github/cuegen/examples/v1alpha2/packages/" + _x
			// },
		]
	}
}
