package kube

chart: wekan: {
	version:    "dev"
	appVersion: "6.69"
}

values: {
	wekan: close({
		hostname: string
		image:    *"\(global.registryPrefix)wekanteam/wekan:v6.69" | string
		if values.wekan.monitoringEnabled {
			ipAuthProxyImage: *"\(global.registryPrefix)nxcc/ip-auth-proxy:0.1.2" | string
			authorizedIPS:    *"10.0.0.0/8,::1/128" | string
		}
		mongodbURL:        string
		namespace:         string
		tempStorage:       *500M | number
		replicas:          *1 | number
		alertingEnabled:   bool
		monitoringEnabled: bool
		smtpURL:           *"" | string
		mailFrom:          *"Wekan Notifications <wekan-demo@\(wekan.hostname)>" | string
	})
}
