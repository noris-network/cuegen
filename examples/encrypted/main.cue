package kube

secret: [ID=_]: {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: name: ID
	type: string | *"Opaque"
	data: {[string]: bytes}
}

secret: "tls": {
	type: "kubernetes.io/tls"
	data: {} @readmap(tls)
}

values: {} @read(secret-values.yaml)

secret: "auth": {
	data: {
		DSN:        'postgresql://\(data.DB_USER):\(data.DB_PASS)@dbhost:5432'
		AUTH_TOKEN: '\(values.app."auth-token")'
	} @readmap(auth.env)
}

objects: [ for v in secret {v}]
