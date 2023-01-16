package kube

global: {
	applicationID:     string
	monitoringEnabled: bool
	alertingEnabled:   bool
	clusterBaseURL:    string
	registryPrefix:    *"" | string
}
