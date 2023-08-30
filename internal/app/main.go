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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/noris-network/cuegen/internal/cuegen"
	"github.com/nxcc/cueconfig"
	"gopkg.in/yaml.v3"
)

const defaultYamlCuegenFile = "cuegen.yaml"
const defaultCueCuegenFile = "cuegen.cue"

var Build = ""

func Main() int {

	checkForCuegenDir := false

	flag.BoolVar(&checkForCuegenDir, "is-cuegen-dir", false, "check current working directory for cuegen.{yaml,cue} (for cmp detection)")
	flag.Parse()

	log.SetFlags(0)

	// detect cuegen directory (for cmp-plugin)
	if checkForCuegenDir {
		if _, err := os.Stat("cuegen.cue"); err == nil {
			fmt.Println("true")
			return 0
		}
		if _, err := os.Stat("cuegen.yaml"); err == nil {
			fmt.Println("true")
			return 0
		}
		return 0
	}

	// check args
	if len(os.Args) == 2 && os.Args[1] == "version" {
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
	if len(os.Args) != 2 {
		log.Fatalln("usage: cuegen <configfile>")
		return 1
	}

	// load config
	configFile, config, err := loadConfig(os.Args[1])
	if err != nil {
		log.Fatalf("loadconfig: %v", err)
	}

	// logging
	log.SetPrefix("# cuegen: ")
	if config.Debug {
		log.Printf("build: %v", Build)
		log.Printf("---")
	}

	// setup paths
	chartRoot := filepath.Dir(configFile)

	chartRoot, err = filepath.Abs(chartRoot)
	if err != nil {
		log.Fatalf("abs: %v", err)
	}
	config.ChartRoot = chartRoot

	if err := os.Chdir(chartRoot); err != nil {
		log.Fatalf("chdir: %v", err)
	}

	if err = cuegen.Exec(config); err != nil {
		log.Fatalf("%v", err)
	}

	return 0
}

// loadConfig loads the cuegen config. When a directory is passed, cuegen will
// look for the default "cuegen.yaml" in that directory.
func loadConfig(path string) (string, cuegen.Config, error) {

	var rootfs fs.FS

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		path = os.ExpandEnv(path)
		gitfs, err := cuegen.GetGitFS(path)
		if err != nil {
			return "", cuegen.Config{}, fmt.Errorf("in loadConfig: %v", err)
		}
		rootfs = gitfs
		path = "."
	} else {
		rootfs = os.DirFS(".")
		path = strings.TrimRight(path, "/")
	}

	file, err := func() (string, error) {
		path = strings.TrimPrefix(path, "./")
		fileInfo, err := fs.Stat(rootfs, path)
		if err != nil {
			return "", fmt.Errorf("stat: %v", err)
		}
		if fileInfo.Mode().IsRegular() {
			return path, nil
		}
		if fileInfo.IsDir() {
			// cuegen.cue?
			file := filepath.Join(path, defaultCueCuegenFile)
			fileInfo, err := fs.Stat(rootfs, file)
			if err == nil && fileInfo.Mode().IsRegular() {
				return file, nil
			}
			// cuegen.yaml?
			file = filepath.Join(path, defaultYamlCuegenFile)
			fileInfo, err = fs.Stat(rootfs, file)
			if err == nil && fileInfo.Mode().IsRegular() {
				return file, nil
			}
		}
		return "", fmt.Errorf("config %q not found", path)
	}()
	if err != nil {
		return "", cuegen.Config{}, err
	}
	if err != nil {
		return "", cuegen.Config{}, err
	}
	ext := filepath.Ext(file)
	if ext == ".cue" {
		return loadCueConfig(rootfs, file)
	}
	if ext == ".yml" || ext == ".yaml" {
		return loadYamlConfig(rootfs, file)
	}
	return "", cuegen.Config{}, errors.New("no config found")
}

// loadYamlConfig loads the cuegen config
func loadYamlConfig(fsys fs.FS, file string) (string, cuegen.Config, error) {
	fh, err := fsys.Open(file)
	if err != nil {
		return "", cuegen.Config{}, err
	}
	config := cuegen.Config{}
	decoder := yaml.NewDecoder(fh)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		if errors.Is(err, io.EOF) {
			return file, cuegen.Config{RootFS: &fsys}, nil
		}
		return "", cuegen.Config{}, err
	}
	config.RootFS = &fsys
	return file, config, nil
}

//go:embed schema.cue
var cuegenConfigSchema []byte

// loadCueConfig loads the cuegen config
func loadCueConfig(fsys fs.FS, file string) (string, cuegen.Config, error) {
	config := struct{ Cuegen cuegen.Config }{}
	if err := cueconfig.LoadFS(fsys, file, cuegenConfigSchema, nil, nil, &config); err != nil {
		return "", cuegen.Config{}, fmt.Errorf("load cue: %v", err)
	}
	config.Cuegen.RootFS = &fsys
	return file, config.Cuegen, nil
}
