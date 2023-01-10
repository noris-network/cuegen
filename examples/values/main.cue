package kube

import "encoding/yaml"

values: {} @read(values.yaml)

configMap: "app-metadata": {
	apiVersion: "v1"
	kind:       "ConfigMap"
	data: {
		"about.html":  """
			  <h1>Welcome to \(values.instance.hostname)
			      \(values.instance.environment) environment!
			  </h1>
			  <i>\(values.instance.replicas) replicas configured</i>
			"""
		"values-dump": yaml.Marshal(values)
	}
}

objects: [ for v in configMap {v}]
