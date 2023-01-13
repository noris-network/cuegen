package kube

#namespace: "myapp-prod"

global: {
	applicationID:     "1234"
	monitoringEnabled: true
	alertingEnabled:   true
}

mongodbAuth: {} @readmap(mongodb-auth.yaml)

values: {
	mongodb: {
		namespace:         #namespace
		image:             "harbor.prod.paas.pop.noris.de/dockerhub/bitnami/mongodb:6.0.3"
		exporterImage:     "harbor.prod.paas.pop.noris.de/dockerhub/bitnami/mongodb-exporter:0.36.0"
		monitoringEnabled: global.monitoringEnabled
		alertingEnabled:   global.alertingEnabled
		storage:           5G
		extraDatabase:     "wekan"
		auth:              mongodbAuth
	}
	wekan: {
		namespace:         #namespace
		image:             "harbor.prod.paas.pop.noris.de/dockerhub/wekanteam/wekan:v6.69"
		monitoringEnabled: global.monitoringEnabled
		alertingEnabled:   global.alertingEnabled
		hostname:          "wekan-cuegen.\(global.clusterBaseURL)"
		mongodbURL:        "mongodb://\(mongodbAuth.username):\(mongodbAuth.password)@mongodb:27017/wekan"
		replicas:          3
		storage:           5G
	}
}
