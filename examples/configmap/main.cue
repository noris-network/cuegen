package kube

configMap: [ID=_]: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: ID
}

configMap: "app-metadata": {
	data: {
		version: string @readfile(version.txt=trim)
	} @readmap(meta.yaml)
}

configMap: "scripts": {
	data: {} @readmap(scripts)
}

objects: [ for v in configMap {v}]
