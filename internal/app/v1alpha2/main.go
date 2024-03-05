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

package v1alpha2

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha2"
)

const cuegenFile = "cuegen.cue"

var Build = "dev build"
var programLevel = new(slog.LevelVar)

func Main() int {

	log.SetFlags(log.Lmsgprefix)
	slog.SetDefault(slog.New(
		slog.NewTextHandler(
			os.Stderr,
			&slog.HandlerOptions{Level: programLevel},
		),
	))

	switch strings.ToLower(os.Getenv("CUEGEN_LOG_LEVEL")) {
	case "debug":
		programLevel.Set(slog.LevelDebug)
	case "info":
		programLevel.Set(slog.LevelInfo)
	default:
		programLevel.Set(slog.LevelError)
	}

	checkForCuegenDir := false
	// this is handled in v1alpha1 code; uncomment when v1alpha1 support is removed
	// flag.BoolVar(&checkForCuegenDir, "is-cuegen-dir", false, "check current working directory for cuegen.{yaml,cue} (for cmp detection)")
	// flag.Parse()

	switch {

	// detect cuegen directory (for cmp-plugin)
	case checkForCuegenDir:
		return isCuegenDir()

	// print version?
	case len(os.Args) == 2 && os.Args[1] == "version":
		return printVersion()

	// check args
	case len(os.Args) != 2:
		slog.Error("usage: cuegen <.>|<path/to/cuegen-dir>")
		return 1
	}

	// load root
	rootRes, err := loadRootResource(os.Args[1])
	if err != nil {
		slog.Error("loadRootResource", "error", err.Error())
		return 1
	}

	// load root package
	rootPkg, err := loadPackage(rootRes)
	if err != nil {
		slog.Error("loadPackage", "error", err.Error())
		return 1
	}

	err = process(rootPkg)
	if err != nil {
		slog.Error("process", "error", err.Error())
		return 1
	}

	return 0
}

func isCuegenDir() int {
	if _, err := os.Stat(cuegenFile); err == nil {
		fmt.Println("true")
	}
	return 0
}

func printVersion() int {
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
	return 0
}

func loadRootResource(path string) (cuegen.Resource, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		loc, err := parseGitURL(path)
		if err != nil {
			return cuegen.Resource{}, fmt.Errorf("parseGitURL: %v", err)
		}
		return loadResource(loc)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return cuegen.Resource{}, fmt.Errorf("stat: %v", err)
	}
	switch {

	case strings.HasPrefix(path, "/") && fileInfo.IsDir():
		return loadResource(cuegen.Location{
			Path:    path,
			Package: "@root@",
		})

	case fileInfo.IsDir():
		cwd, err := os.Getwd()
		if err != nil {
			return cuegen.Resource{}, fmt.Errorf("getwd: %v", err)
		}
		return loadResource(cuegen.Location{
			Path:    filepath.Join(cwd, path),
			Package: "@root@",
		})

	case fileInfo.Mode().Type().IsRegular():
		return cuegen.Resource{},
			errors.New("please provide path to directory containg 'cuegen.cue'")

	default:
		return cuegen.Resource{},
			errors.New("case not handled, please report")
	}
}
