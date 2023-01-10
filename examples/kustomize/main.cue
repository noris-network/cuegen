package kube

configMap: [ID=_]: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: ID
}

configMap: "hello": data: {
	greeting: string @readfile(greeting.txt)
}

objects: [ for v in configMap {v}]
