package kube

configMap: [ID=_]: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: ID
}

objects: [ for v in configMap {v}]
