package minimal

export: objects: configMap: demo: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "demo"
		namespace: "default"
	}
	data: greeting: "Hello from cuegen!"
}
