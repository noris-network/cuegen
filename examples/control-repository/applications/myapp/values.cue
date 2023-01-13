package kube

values: {
	myapp: {
		hostname:          string
		image:             string
		exporterImage:     string
		alertingEnabled:   bool
		monitoringEnabled: bool
		namespace:         string
	}
}
