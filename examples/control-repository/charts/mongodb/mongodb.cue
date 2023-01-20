package kube

values: {}

secret: "mongodb-auth": data: {
	MONGODB_EXTRA_USERNAMES: '\(values.mongodb.auth.username)'
	MONGODB_EXTRA_PASSWORDS: '\(values.mongodb.auth.password)'
	MONGODB_ROOT_PASSWORD:   '\(values.mongodb.auth.rootPassword)'
}

deployment: mongodb: _enableGenerator: cuegenExampleV2: true
deployment: mongodb: spec: {
	strategy: type: "Recreate"
	template: spec: {
		containers: [
			{
				name:  "mongodb"
				image: values.mongodb.image
				envFrom: [ {
					secretRef: name: "mongodb-auth"
				}]
				env: [{
					name:  "MONGODB_EXTRA_DATABASES"
					value: values.mongodb.extraDatabase
				}]
				ports: [{
					name:          "mongodb"
					containerPort: 27017
				}]
				livenessProbe: {
					exec: command: [ "mongosh", "--eval", "db.adminCommand('ping')"]
					initialDelaySeconds: 5
					timeoutSeconds:      5
					failureThreshold:    6
				}
				readinessProbe: {
					exec: command: [ "bash", "-ec", "mongosh --eval 'db.hello().isWritablePrimary' | grep -q 'true'"]
					initialDelaySeconds: 5
					timeoutSeconds:      5
					failureThreshold:    6
				}
				volumeMounts: [{
					name:      "data"
					mountPath: "/bitnami/mongodb"
					_pvc: storage: values.mongodb.storage
				}]
			},
			if values.mongodb.monitoringEnabled {
				name:  "metrics"
				image: values.mongodb.exporterImage
				command: [ "sh", "-c"]
				args: [#"sleep 3 && mongodb_exporter --mongodb.uri "mongodb://root:$(echo "$MONGODB_ROOT_PASSWORD" | sed -r "s/@/%40/g;s/:/%3A/g")@127.0.0.1:27017/admin""#]
				envFrom: [{
					secretRef: name: "mongodb-auth"
				}]
				ports: [{
					name:          "metrics"
					containerPort: 9216
				}]
				livenessProbe: {
					httpGet: {path: "/", port: "metrics"}
					initialDelaySeconds: 10
					timeoutSeconds:      5
					failureThreshold:    3
				}
				readinessProbe: {
					httpGet: {path: "/", port: "metrics"}
					initialDelaySeconds: 5
					failureThreshold:    3
				}
			},
		]
	}
}

if values.mongodb.alertingEnabled {
	prometheusRule: mongodb: spec: groups: [{
		name: "mongodb-\(values.mongodb.namespace)"
		rules: [{
			alert: "mongodb up"
			expr:  'absent(mongodb_up)'
		}]
	}]
}
