package kube

#namespace: "myapp-dev"

values: {
	global: {
		applicationID: "1234"
		release:       "v2"
	}
	myapp: {
		image:    "bitnami/nginx:latest"
		hostname: "myapp-dev.apps.testmcs-pop.noris.de"
	}
}
