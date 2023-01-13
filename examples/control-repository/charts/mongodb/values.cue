package kube

values: {
	mongodb: close({
		image:             string
		exporterImage:     string
		alertingEnabled:   bool
		monitoringEnabled: bool
		namespace:         string
		storage:           number
		extraDatabase:     string
		auth: {
			username:     string
			password:     string
			rootPassword: string
		}
	})
}
