@extern(embed)

package main

objects: [
	{
		Kind: "ConfigMap"
		spec: {} @embed(file=values.json)
	},
	{
		Kind: "Secret"
		spec: {} @embed(file=secrets.json)
	},
]
