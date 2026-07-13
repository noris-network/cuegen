package webapp

// Shared values referenced by multiple objects.
let _appName = "webapp"
let _image = "nginx:1.27"
let _replicas = 3
let _port = 8080

export: objects: {
	deployment: webapp: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      _appName
			namespace: "default"
			labels: app: _appName
		}
		spec: {
			replicas: _replicas
			selector: matchLabels: app: _appName
			template: {
				metadata: labels: app: _appName
				spec: containers: [{
					name:  _appName
					image: _image
					ports: [{
						containerPort: _port
						name:          "http"
					}]
				}]
			}
		}
	}

	service: webapp: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name:      _appName
			namespace: "default"
			labels: app: _appName
		}
		spec: {
			type: "ClusterIP"
			selector: app: _appName
			ports: [{
				port:       _port
				targetPort: "http"
				protocol:   "TCP"
				name:       "http"
			}]
		}
	}

	configMap: appConfig: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "app-config"
			namespace: "default"
		}
		data: {
			LOG_LEVEL:   "info"
			PORT:        "\(_port)"
			ENVIRONMENT: $value
		}
	}
}

$value: string
