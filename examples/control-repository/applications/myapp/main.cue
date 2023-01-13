package kube

configMap: pages: {
	data: {} @readmap(pages)
}

deployment: myapp: spec: {
	template: {
		spec: {
			containers: [
				{
					name:  "nginx"
					image: values.myapp.image
					volumeMounts: [ {
						mountPath: "/app"
						name:      "pages"
					}, {
						mountPath: "/opt/bitnami/nginx/conf/server_blocks"
						name:      "conf"
					}]
					ports: [ {
						containerPort: 8080
						name:          "http"
					}]
				},
				if values.myapp.monitoringEnabled {
					name:  "exporter"
					image: values.myapp.exporterImage
					ports: [{
						containerPort: 9113
						name:          "metrics"
					}]
					args: ["-nginx.scrape-uri=http://localhost:8080/stub_status"]
				},
			]
			volumes: [ {
				name: "pages", configMap: name: "pages"
			}, {
				name: "conf", configMap: name: "conf"
			}]
		}
	}
}

service: myapp: {
	spec: ports: [
		{
			port:       8080
			targetPort: 8080
			name:       "http"
		},
		if values.myapp.monitoringEnabled {
			port:       9113
			targetPort: 9113
			name:       "metrics"
		},
	]
}

ingress: myapp: spec:
	rules: [{
		host: values.myapp.hostname
	}]

if values.myapp.monitoringEnabled {
	serviceMonitor: myapp: spec: endpoints: [{
		path: "/metrics"
		port: "metrics"
	}]
}

if values.myapp.alertingEnabled {
	prometheusRule: myapp: spec: groups: [{
		name: "myapp-\(values.myapp.namespace)"
		rules: [{
			alert: "nginx up"
			expr:  'absent(nginx_up) == 0'
		}]
	}]
}
