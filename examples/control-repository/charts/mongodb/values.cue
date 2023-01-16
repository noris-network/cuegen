package kube

values: {
	mongodb: close({
		image: *"\(global.registryPrefix)bitnami/mongodb:6.0.3" | string
		if values.mongodb.monitoringEnabled {
			exporterImage: *"\(global.registryPrefix)bitnami/mongodb-exporter:0.36.0" | string
		}
		namespace:         string
		storage:           number
		extraDatabase:     string
		alertingEnabled:   bool
		monitoringEnabled: bool
		auth: {
			username:     string
			password:     string
			rootPassword: string
		}
	})
}
