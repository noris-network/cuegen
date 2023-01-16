package kube

//// ConfigMap /////////////////////////////////////////////////////////////////////////////////////

configMap: [ID=_]: {
	_labels:    commonLabels
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: labels:    _labels
	metadata: name:      ID
	metadata: namespace: string | *#namespace
}

//// Deployment ////////////////////////////////////////////////////////////////////////////////////

deployment: [ID=_]: {
	_labels:    (#mergeLabels & {i: [commonLabels, {"app-component": ID}]}).o
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: labels:    _labels
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	spec: selector: matchLabels: _labels
	spec: template: metadata: labels: _labels
}

//// Ingress ///////////////////////////////////////////////////////////////////////////////////////

// TODO!!!!!!!!
ingress: [ID=_]: {
	//let svc = service
	apiVersion: "networking.k8s.io/v1"
	kind:       "Ingress"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	// spec: rules: [{
	// 	host: string
	// 	// http: paths: *[
	// 	// 		{
	// 	// 		backend: service: name: ID
	// 	// 		backend: service: port: number: svc.wekan.spec.ports[0].port
	// 	// 		pathType: "Prefix"
	// 	// 		path:     "/"
	// 	// 	},
	// 	// ] | [...]
	// }]
}

//// PersistentVolumeClaim /////////////////////////////////////////////////////////////////////////

persistentVolumeClaim: [ID=_]: {
	_labels:    commonLabels
	apiVersion: "v1"
	kind:       "PersistentVolumeClaim"
	metadata: labels:    _labels
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	spec: {
		accessModes: [
			*"ReadWriteOnce" | "ReadOnlyMany" | "ReadWriteMany",
		]
		resources: {
			requests: {
				storage: number
			}
		}
	}
}

//// PrometheusRule ////////////////////////////////////////////////////////////////////////////////

prometheusRule: [ID=_]: {
	_labels:    commonLabels
	apiVersion: "monitoring.coreos.com/v1"
	kind:       "PrometheusRule"
	metadata: labels:    _labels
	metadata: name:      ID
	metadata: namespace: string | *#namespace
}

//// Secret ////////////////////////////////////////////////////////////////////////////////////////

secret: [ID=_]: {
	_labels:    commonLabels
	apiVersion: "v1"
	kind:       "Secret"
	metadata: labels:    _labels
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	type: string | *"Opaque"
	data: {[string]: bytes}
}

//// Service ///////////////////////////////////////////////////////////////////////////////////////

service: [ID=_]: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
}

//// StatefulSet ///////////////////////////////////////////////////////////////////////////////////

statefulSet: [ID=_]: {
	_labels:    (#mergeLabels & {i: [commonLabels, {"app-component": ID}]}).o
	apiVersion: "apps/v1"
	kind:       "StatefulSet"
	metadata: labels:    _labels
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	spec: selector: matchLabels: _labels
	spec: template: metadata: labels: _labels
}

////////////////////////////////////////////////////////////////////////////////////////////////////

#namespace: string

commonLabels: {
	"myorg/app-id": global.applicationID
}

// object sets
configMap: {}
deployment: {}
ingress: {}
persistentVolumeClaim: {}
prometheusRule: {}
secret: {}
service: {}
statefulSet: {}

// gather all object sets
objectSets: [
	configMap,
	deployment,
	ingress,
	persistentVolumeClaim,
	prometheusRule,
	secret,
	service,
	statefulSet,
]

// gather all objects
objects: [ for v in objectSets for x in v {x}]

// merge labels 'function'
#mergeLabels: {i: [...], o: {for _, ilabels in i {ilabels}}}
