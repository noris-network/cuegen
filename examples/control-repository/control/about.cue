package kube

import "encoding/yaml"

chart: [ID=_]: {
	name:       ID
	version:    string
	appVersion: string
}

// gather all chart info
charts: [ for v in chart {v}]

configMap: "about-components": {
	data: components: yaml.Marshal(charts)
}
