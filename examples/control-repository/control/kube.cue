package kube

#namespace: string

configMap: [ID=_]: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	_labels: commonLabels
	metadata: labels: _labels
}

secret: [ID=_]: {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	_labels: commonLabels
	metadata: labels: _labels
	type: string | *"Opaque"
	data: {[string]: bytes}
}

deployment: [ID=_]: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	_labels: (#mergeLabels & {in: [commonLabels, {"app-component": ID}]}).out
	metadata: labels: _labels
	spec: selector: matchLabels: _labels
	spec: template: metadata: labels: _labels
}

statefulSet: [ID=_]: {
	apiVersion: "apps/v1"
	kind:       "StatefulSet"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	_labels: (#mergeLabels & {in: [commonLabels, {"app-component": ID}]}).out
	metadata: labels: _labels
	spec: selector: matchLabels: _labels
	spec: template: metadata: labels: _labels
}

service: [ID=_]: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
}

ingress: [ID=_]: {
	let svc = service
	apiVersion: "networking.k8s.io/v1"
	kind:       "Ingress"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	spec: rules: [{
		host: string
		http: paths: *[
				{
				backend: service: name: ID
				backend: service: port: number: svc.wekan.spec.ports[0].port
				pathType: "Prefix"
				path:     "/"
			},
		] | [...]
	}]
}

prometheusRule: [ID=_]: {
	apiVersion: "monitoring.coreos.com/v1"
	kind:       "PrometheusRule"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	_labels: commonLabels
	metadata: labels: _labels
	spec: {...}
}

persistentVolumeClaim: [ID=_]: {
	apiVersion: "v1"
	kind:       "PersistentVolumeClaim"
	metadata: name:      ID
	metadata: namespace: string | *#namespace
	_labels: commonLabels
	metadata: labels: _labels
	spec: {
		accessModes: [
			"ReadWriteOnce" | "ReadWriteMany" | *"ReadWriteOnce",
		]
		resources: {
			requests: {
				storage: number
			}
		}
	}
}

service: {
	for _, xxxxxxxxxxxx in [deployment, statefulSet]
	for deploymentName, deployment in xxxxxxxxxxxx {
		"\(deploymentName)": {
			metadata: labels: deployment._labels
			spec: {
				selector: deployment._labels
				ports: [
					for _, container in deployment.spec.template.spec.containers
					if container.ports != _|_
					for portName, portObj in container.ports {
						name:       portObj.name
						targetPort: portObj.name
						port:       portObj.containerPort
						if portObj._ingress != _|_ {
							_ingress: portObj._ingress
						}
					},
				]
			}
		}
	}
}

// Ingress: ... _ingress
ingress: {
	for serviceName, service in service
	for _, portObj in service.spec.ports {
		if portObj._ingress != _|_ {
			"\(serviceName)-\(portObj.name)": {
				spec: rules: [{
					host: portObj._ingress.hostname
					http: paths: [
						{
							backend: service: name: serviceName
							backend: service: port: name: portObj.name
							pathType: "Prefix"
							path:     "/"
						},
					]
				}]
				if portObj._ingress.secretName == _|_ {
					metadata: annotations: "route.openshift.io/termination": "edge"
				}
				if portObj._ingress.secretName != _|_ {
					spec: tls: [{
						hosts: [
							portObj._ingress.hostname,
						]
						secretName: portObj._ingress.secretName
					}]
				}
			}
		}
	}
}

commonLabels: {
	"myorg/app-id": global.applicationID
}

// object sets
configMap: {}
deployment: {}
ingress: {}
secret: {}
service: {}
prometheusRule: {}
persistentVolumeClaim: {}
statefulSet: {}

// gather all object sets
objectSets: [
	configMap,
	deployment,
	ingress,
	secret,
	service,
	prometheusRule,
	persistentVolumeClaim,
	statefulSet,
]

// gather all objects
objects: [ for v in objectSets for x in v {x}]

//

//

//
//allLabels: (#mergeLabels & {in: [cLabels, myLabels]}).out

#mergeLabels: {
	in: [...]
	out: {for _, ilabels in in {ilabels}}
}
