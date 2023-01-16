package kube

#namespace: "cuegen-demo-qa"

global: {
	applicationID: "678"
}

mongodbAuth: {} @readmap(mongodb-auth.yaml)

values: {
	mongodb: {
		namespace:         #namespace
		storage:           5G
		extraDatabase:     "wekan"
		alertingEnabled:   global.alertingEnabled
		monitoringEnabled: global.monitoringEnabled
		auth:              mongodbAuth
	}
	wekan: {
		namespace:         #namespace
		hostname:          "wekan-demo-qa.\(global.clusterBaseURL)"
		mongodbURL:        "mongodb://\(mongodbAuth.username):\(mongodbAuth.password)@mongodb:27017/wekan"
		replicas:          3
		monitoringEnabled: global.monitoringEnabled
		alertingEnabled:   global.alertingEnabled
		smtpURL:           demo.smtpURL
	}
}
