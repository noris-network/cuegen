package kube

#namespace: "cuegen-demo-dev"

global: {
	applicationID: "567"
}

mongodbAuth: {} @readmap(mongodb-auth.yaml)

values: {
	mongodb: {
		image:             "\(global.registryPrefix)bitnami/mongodb:latest"
		namespace:         #namespace
		storage:           5G
		extraDatabase:     "wekan"
		alertingEnabled:   global.alertingEnabled
		monitoringEnabled: global.monitoringEnabled
		auth:              mongodbAuth
	}
	wekan: {
		image:             "\(global.registryPrefix)wekanteam/wekan:latest"
		namespace:         #namespace
		hostname:          "wekan-demo-dev.\(global.clusterBaseURL)"
		mongodbURL:        "mongodb://\(mongodbAuth.username):\(mongodbAuth.password)@mongodb:27017/wekan"
		replicas:          3
		monitoringEnabled: global.monitoringEnabled
		alertingEnabled:   global.alertingEnabled
	}
}
