package application

import (
	"example.com/pkgs/libone"
	"example.com/pkgs/libtwo"
	"example.com/pkgs/libthree"
)

objects: [
	{
		kind:       "application-object-1"
		fromlibone: libone.values.aaa
	},
	{
		kind:       "application-object-2"
		fromlibtwo: libtwo.values.bbb
	},
	{
		kind: "application-object-3"
		fromlibthree: {
			alpha:     libthree.values.ccc
			signature: libthree.values.sig
		}

	},
	{
		kind:  "application-object-4"
		about: appInfo
	},
]
