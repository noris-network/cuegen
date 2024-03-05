package cache

import (
)

// "example.com/pkgs/libone"
// "example.com/pkgs/libthree"
// "cuegen.local/config"
export: [
	// {
	// 	kind:       "cache-object-1"
	// 	fromlibone: libone.values.aaa
	// },
	// {
	// 	kind:         "cache-object-2"
	// 	fromlibthree: libthree.values.ccc
	// },
	// {
	// 	kind:       "cache-object-3"
	// 	fromconfig: config.values.instance.namespace
	// },
	{
		kind:       "cache-object-4"
		timeToLive: values.ttl
	},
]

values: ttl: *10 | int
