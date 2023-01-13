package kube

cLabels: {
	"clabel1": "one"
	"clabel2": "two"
}

myLabels: {
	"mylabel1": "myone"
	"mylabel2": "mytwo"
}

allLabels: (#mergeLabels & {in: [cLabels, myLabels]}).out

#mergeLabels: {
	in: [...]
	out: {for _, ilabels in in {ilabels}}
}
