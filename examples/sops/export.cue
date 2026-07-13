@extern(embed)

package sops

_vals: _ @embed(file="config.enc.yaml")

export: objects: configMap: "db-config": {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "db-config"
		namespace: "default"
	}
	data: {
		DATABASE_HOST: "\(_vals.database.host)"
		DATABASE_PORT: "\(_vals.database.port)"
		API_KEY:       "\(_vals.api.key)"
	}
}
