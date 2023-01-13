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
		image:             "harbor.prod.paas.pop.noris.de/dockerhub/bitnami/mongodb:4.4.10-debian-10-r44"
		exporterImage:     "harbor.prod.paas.pop.noris.de/dockerhub/bitnami/mongodb-exporter:0.11.2-debian-10-r327"
		monitoringEnabled: global.monitoringEnabled
		alertingEnabled:   global.alertingEnabled
		storage:           5G
		extraDatabase:     "wekan"
		auth:              mongodbAuth
	}
	wekan: {
		namespace:         #namespace
		image:             "harbor.prod.paas.pop.noris.de/dockerhub/wekanteam/wekan:v5.60"
		monitoringEnabled: global.monitoringEnabled
		alertingEnabled:   global.alertingEnabled
		hostname:          "wekan-cuegen.\(global.clusterBaseURL)"
		mongodbURL:        "mongodb://\(mongodbAuth.username):\(mongodbAuth.password)@mongodb:27017/wekan"
		replicas:          3
	}
}
