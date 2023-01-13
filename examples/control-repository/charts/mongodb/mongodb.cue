package kube

// secret: "mongodb-auth": data: {
// 	MONGODB_EXTRA_USERNAMES: '\(values.mongodb.auth.username)'
// 	MONGODB_EXTRA_PASSWORDS: '\(values.mongodb.auth.password)'
// 	MONGODB_ROOT_PASSWORD:   '\(values.mongodb.auth.rootPassword)'
// }

deployment: mongodb: spec: template: spec: {
	containers: [
		{
			name:  "mongodb"
			image: values.mongodb.image
			ports: [{
				name:          "mongodb"
				containerPort: 27017
			}]
			volumeMounts: [{
				name:      "data"
				mountPath: "/bitnami/mongodb"
			}]
			envFrom: [ {
				secretRef: name: "mongodb-auth"
			}]
			env: [{
				name:  "MONGODB_EXTRA_DATABASES"
				value: values.mongodb.extraDatabase
			}]
		},
	]
	volumes: [{
		name: "data"
		persistentVolumeClaim: claimName: "mongodb-data"
	}]
}

persistentVolumeClaim: "mongodb-data": spec: {
	resources: requests: storage: values.mongodb.storage
}

// service: mongodb: {
// 	spec: ports: [
// 		{
// 			port:       27017
// 			targetPort: 27017
// 			name:       "mongodb"
// 		},
// 	]
// }
