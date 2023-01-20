package kube

values: {}

secret: "wekan-auth": data: {
	MONGO_URL: '\(values.wekan.mongodbURL)'
}

configMap: "wekan-env": data: {
	ROOT_URL:  "http://\(values.wekan.hostname)"
	MAIL_URL:  values.wekan.smtpURL
	MAIL_FROM: values.wekan.mailFrom
}

deployment: wekan: _enableGenerator: cuegenExampleV2: true
deployment: wekan: spec: {
	replicas: values.wekan.replicas
	template: spec: {
		containers: [
			{
				name:  "wekan"
				image: values.wekan.image
				if values.wekan.monitoringEnabled {
					env: [
						{name: "WEKAN_METRICS_ACCEPTED_IP_ADDRESS", value: "127.0.0.1"},
					]
				}
				envFrom: [
					{configMapRef: name: "wekan-env"},
					{secretRef: name:    "wekan-auth"},
				]
				ports: [{
					name:          "http"
					containerPort: 8080
					_ingress: hostname: values.wekan.hostname
				}]
				livenessProbe: {
					httpGet: path: "/sign-in"
					httpGet: port: "http"
					initialDelaySeconds: 20
					timeoutSeconds:      5
				}
				readinessProbe: {
					httpGet: path: "/sign-in"
					httpGet: port: "http"
					timeoutSeconds: 5
				}
				volumeMounts: [{
					name:      "temp"
					mountPath: "/data"
					_emptyDir: sizeLimit: values.wekan.tempStorage
				}]
			},
			if values.wekan.monitoringEnabled {
				{
					name:  "ip-auth-proxy"
					image: values.wekan.ipAuthProxyImage
					env: [
						{name: "UPSTREAM_URL", value:   "http://127.0.0.1:8080"},
						{name: "LISTEN_PORT", value:    "8000"},
						{name: "VERBOSE", value:        "true"},
						{name: "AUTHORIZED_IPS", value: values.wekan.authorizedIPS},
					]
					ports: [{
						name:          "metrics"
						containerPort: 8000
					}]
				}
			},
		]
	}
}

if values.wekan.alertingEnabled {
	prometheusRule: wekan: spec: groups: [{
		name: "wekan-\(values.wekan.namespace)"
		rules: [{
			alert: "wekan up"
			expr:  'absent(wekan_registeredboards)'
		}]
	}]
}
