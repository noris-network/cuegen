// Copyright 2023 cuegen Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	v1alpha1 "github.com/noris-network/cuegen/internal/app/v1alpha1"
	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha3"
	"github.com/nxcc/cueconfig"
)

const (
	cuegenCue  = "cuegen.cue"
	cuegenYaml = "cuegen.yaml"
)

//go:embed schema.cue
var configSchema []byte

var Build string
var debugLog = os.Getenv("CUEGEN_DEBUG") == "true"

func Main() int {

	flagIsCuegenDir := false

	flag.BoolVar(&flagIsCuegenDir, "is-cuegen-dir", false,
		"check current working directory for cuegen.{yaml,cue} (for cmp detection)")

	flag.Parse()

	path := "."

	switch {

	// cmp detection
	case flagIsCuegenDir:
		isCuegenDir()
		return 0

	// version
	case len(os.Args) == 2 && os.Args[1] == "version":
		printVersion()
		return 0

	// no chdir
	case len(os.Args) == 1,
		len(os.Args) == 2 && (os.Args[1] == "." || os.Args[1] == "./"):

	// chdir
	case len(os.Args) == 2 && (strings.HasPrefix(os.Args[1], "./") || strings.HasPrefix(os.Args[1], "../") || strings.HasPrefix(os.Args[1], "/")):
		path = os.Args[1]

	// fallback
	default:
		fallbackToV1Alpha1()
	}

	// read cuegen.cue
	config, err := configure()
	if err != nil {
		fmt.Printf("configure: %v\n", err)
		return 1
	}
	if config.ApiVersion != cuegen.V1Alpha3 {
		fallbackToV1Alpha1()
	}

	// exec
	if err := cuegen.Exec(config, path); err != nil {
		fmt.Printf("exec: %v\n", err)
		return 1
	}

	return 0
}

func isCuegenDir() {
	if _, err := os.Stat(cuegenCue); err == nil {
		fmt.Println(true)
	}
	if _, err := os.Stat(cuegenYaml); err == nil {
		fmt.Println(true)
	}
}

func printVersion() {
	fmt.Printf("cuegen version %v\n", Build)
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.Fatalln("Failed to read build info")
	}
	for _, dep := range bi.Deps {
		if dep.Path == "cuelang.org/go" {
			fmt.Printf("cue version %v\n", dep.Version)
			break
		}
	}
}

func configure() (cuegen.Cuegen, error) {
	cfg := struct{ Cuegen cuegen.Cuegen }{}
	if err := cueconfig.Load(cuegenCue, configSchema, nil, nil, &cfg); err != nil {
		if os.IsNotExist(err) {
			return cuegen.Default, nil
		}
		return cuegen.Cuegen{}, err
	}
	return cfg.Cuegen, nil
}

func fallbackToV1Alpha1() {
	if debugLog {
		fmt.Println("#@@@ fallback to v1alpha1")
	}
	os.Exit(v1alpha1.Main())
}
