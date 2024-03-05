package database

import (
	"example.com/pkgs/libone"
	"example.com/pkgs/libtwo"
	"cuegen.local/config"
)

objects: [
	{
		kind:       "database-object-1"
		fromlibone: libone.values.aaa
	},
	{
		kind:       "database-object-2"
		fromlibtwo: libtwo.values.bbb
	},
	{
		kind:       "database-object-3"
		fromconfig: config.values.global.cluster.name
	},
]
