package kube

_useGenerators: *"" | =~"^v\\d+$"

if _useGenerators == "v0" {

	// add volumes from volumeMounts
	deployment: [ID=_]: {
		spec: template: spec: {
			volumes: [
				for _, container in containers
				if container.volumeMounts != _|_
				for _, volumeMount in container.volumeMounts {
					if volumeMount._configMap != _|_ {
						name:      volumeMount.name
						configMap: volumeMount._configMap
					}
					if volumeMount._pvc != _|_ && volumeMount._pvc.storage != _|_ {
						name: volumeMount.name
						persistentVolumeClaim: {
							claimName: "\(ID)-\(volumeMount.name)"
							_pvc:      volumeMount._pvc
						}
					}
					if volumeMount._emptyDir != _|_ {
						name: volumeMount.name
						emptyDir: sizeLimit: volumeMount._emptyDir.sizeLimit
					}
				},
			]
			containers: [...{name: string}]
		}
	}

	//// Ingress ///////////////////////////////////////////////////////////////////////////////////////

	// create from _ingress in service.spec.ports.[*]
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

	//// PersistentVolumeClaim /////////////////////////////////////////////////////////////////////////

	// create from _pvc in deployment.spec.template.spec.volumes.[*]
	persistentVolumeClaim: {
		for _, deployment in deployment
		if deployment.spec.template.spec.volumes != _|_
		for _, volume in deployment.spec.template.spec.volumes {
			if volume.persistentVolumeClaim != _|_ &&
				volume.persistentVolumeClaim._pvc.storage != _|_ {
				"\(volume.persistentVolumeClaim.claimName)": {
					spec: {
						resources: requests: storage: volume.persistentVolumeClaim._pvc.storage
						if volume.persistentVolumeClaim._pvc.accessModes != _|_ {
							accessModes: volume.persistentVolumeClaim._pvc.accessModes
						}
					}
				}
			}
		}
	}

	//// Service ///////////////////////////////////////////////////////////////////////////////////////

	// create from deploymentOrStatefulSet.spec.template.spec.containers.ports.[*]
	service: {
		for _, deploymentOrStatefulSet in [deployment, statefulSet]
		for deploymentName, deployment in deploymentOrStatefulSet {
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

	////////////////////////////////////////////////////////////////////////////////////////////////////

	//// Deployment ////////////////////////////////////////////////////////////////////////////////////
}
