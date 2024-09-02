package main

import (
	"os"

	v1alpha1 "github.com/noris-network/cuegen/internal/app/v1alpha1"
	v1alpha4 "github.com/noris-network/cuegen/internal/app/v1alpha4"
)

var build = "dev-build"

func main() {
	v1alpha1.Build = build
	v1alpha4.Build = build

	// shortcut to legacy version
	if os.Getenv("CUEGEN_COMPATIBILITY_0_14") == "true" {
		os.Exit(v1alpha1.Main())
	}

	os.Exit(v1alpha4.Main())
}
