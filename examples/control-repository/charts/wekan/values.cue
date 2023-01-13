package kube

values: {
	wekan: close({
		hostname:          string
		image:             string
		alertingEnabled:   bool
		monitoringEnabled: bool
		namespace:         string
		mongodbURL:        string
		replicas:          *1 | number
		storage:           number
	})
}
