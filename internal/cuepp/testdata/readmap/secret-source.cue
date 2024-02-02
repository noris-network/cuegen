package readfile

secret: mysecret: {
	data: {} @readmap(secret.yaml)
}
