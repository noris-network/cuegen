package kube

configMap: [ID=_]: {metadata: name: ID}

configMap: "app-metadata": {
	apiVersion: "v1"
	kind:       "ConfigMap"
	data: {
		version: string @readfile(version.txt=trim)
	} @readmap(meta.yaml)
}

configMap: "scripts": {
	apiVersion: "v1"
	kind:       "ConfigMap"
	data: {} @readmap(scripts)
}

objects: [ for v in configMap {v}]
