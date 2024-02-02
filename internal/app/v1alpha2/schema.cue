cuegen: close({
	apiVersion: string
	kind:       "Cuegen"
	spec: close({
		debug: *false | bool
		imports: [...string]
		objectsPath:    *"objects" | string
		secretDataPath: *"secret" | string
		postProcess:    *"" | string
	})
})
