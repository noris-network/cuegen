package main

import (
	"os"

	v1alpha3 "github.com/noris-network/cuegen/internal/app/v1alpha3"
)

var build = "dev-build"

func main() {
	v1alpha3.Build = build
	os.Exit(v1alpha3.Main())
}
