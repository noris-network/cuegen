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
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	v1alpha1 "github.com/noris-network/cuegen/internal/app/v1alpha1"
	v1alpha4 "github.com/noris-network/cuegen/internal/cuegen/v1alpha4"
	"github.com/nxcc/cueconfig"
)

const (
	cuegenCue  = "cuegen.cue"
	cuegenYaml = "cuegen.yaml"
)

//go:embed schema.cue
var configSchema []byte

var (
	Build         string
	debugLog      = os.Getenv("CUEGEN_DEBUG") == "true"
	cuegenWrapper = os.Getenv("CUEGEN_SKIP_DECRYPT") != "true"
)

func Main() int {
	flagIsCuegenDir := false

	flag.BoolVar(&flagIsCuegenDir, "is-cuegen-dir", false,
		"check current working directory for cuegen.{yaml,cue} (for cmp detection)")

	flag.Parse()

	if debugLog {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

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

	// path = .
	case len(os.Args) == 1,
		len(os.Args) == 2 && (os.Args[1] == "." || os.Args[1] == "./"):

	// path != .
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

	switch config.ApiVersion {

	case v1alpha4.V1Alpha1:
		fallbackToV1Alpha1()

	case v1alpha4.V1Alpha4:
		if err := v1alpha4.Exec(config, path); err != nil {
			fmt.Printf("%v\n", err)
			return 1
		}

	default:
		panic("apiVersion unknown")
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
		slog.Error("failed to read build info")
		os.Exit(1)
	}
	for _, dep := range bi.Deps {
		if dep.Path == "cuelang.org/go" {
			fmt.Printf("cue version %v\n", dep.Version)
			break
		}
	}
}

func configure() (v1alpha4.Cuegen, error) {
	cfg := struct{ Cuegen v1alpha4.Cuegen }{}
	if err := cueconfig.Load(cuegenCue, configSchema, nil, nil, &cfg); err != nil {
		slog.Error("load cuegen.cue", "err", err)
		return v1alpha4.Cuegen{}, err
	}
	return cfg.Cuegen, nil
}

func fallbackToV1Alpha1() {
	if debugLog {
		slog.Debug("fallback to v1alpha1")
	}
	os.Exit(v1alpha1.Main())
}
