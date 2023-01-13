package kube

secret: "wekan-auth": data: {
	MONGO_URL: '\(values.wekan.mongodbURL)'
}

configMap: "wekan-env": data: {
	ROOT_URL: "http://\(values.wekan.hostname)"
}

deployment: wekan: spec: {
	replicas: values.wekan.replicas
	template: spec: {
		containers: [
			{
				name:  "wekan"
				image: values.wekan.image
				envFrom: [
					{configMapRef: name: "wekan-env"},
					{secretRef: name:    "wekan-auth"},
				]
				ports: [{
					name:          "http"
					containerPort: 8080
					_ingress: hostname: values.wekan.hostname
				}]
				volumeMounts: [{
					name:      "temp"
					mountPath: "/data"
				}]
			},
		]
		volumes: [{
			name: "temp"
			emptyDir: sizeLimit: 200M
		}]
	}
}
