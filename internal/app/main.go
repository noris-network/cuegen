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
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/noris-network/cuegen/internal/cuegen"
	"gopkg.in/yaml.v3"
)

const defaultCuegenFile = "cuegen.yaml"

var Build = ""
var runningAsKustomizePlugin = os.Getenv("KUSTOMIZE_PLUGIN_CONFIG_ROOT") != ""

func Main() int {

	log.SetFlags(0)

	// check args
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Printf("cuegen %v", Build)
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
	chartRoot := ""
	if runningAsKustomizePlugin {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("getwd: %v", err)
		}
		chartRoot = cwd
	} else {
		chartRoot = filepath.Dir(configFile)
	}

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
func loadConfig(file string) (string, cuegen.Config, error) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		return "", cuegen.Config{}, err
	}
	if fileInfo.IsDir() {
		file = filepath.Join(file, defaultCuegenFile)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", cuegen.Config{}, err
	}
	config := cuegen.Config{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", cuegen.Config{}, fmt.Errorf("unmarshal config: %v", err)
	}
	return file, config, nil
}
