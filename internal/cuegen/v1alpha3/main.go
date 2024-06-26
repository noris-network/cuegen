// Copyright 2024 cuegen Authors
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

package cuegen

import (
	"fmt"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	cueyaml "cuelang.org/go/pkg/encoding/yaml"
)

const (
	V1Alpha3 = "v1alpha3"
)

type Cuegen struct {
	ApiVersion string
	Metadata   struct {
		Version struct {
			Pkg string
			App string
			Pre string
		}
	}
	Spec Spec
}

type Spec struct {
	Export string
}

var Default = Cuegen{
	ApiVersion: V1Alpha3,
	Spec:       Spec{Export: "objects"},
}

var debugLog = os.Getenv("CUEGEN_DEBUG") == "true"

func Exec(config Cuegen, path string) error {

	// new context
	ctx := cuecontext.New(cuecontext.EvaluatorVersion(cuecontext.EvalDefault))

	// prepare load.Config
	loadConfig := load.Config{}
	if loMo := os.Getenv("CUEGEN_USE_LOCAL_MODULES"); loMo != "" {
		loadConfig.Registry = NewDevRegistry(loMo)
	}

	// only load one instance
	instance := load.Instances([]string{path}, &loadConfig)[0]
	if instance.Err != nil {
		return fmt.Errorf("load instance: %v", instance.Err)
	}

	value := ctx.BuildInstance(instance)
	if err := value.Err(); err != nil {
		return fmt.Errorf("build instance: %v", value.Err())
	}

	// handle cuegen attributes
	paths := findAttributes(value)
	value, err := processAttributes(value, paths)
	if err != nil {
		return fmt.Errorf("process attributes: %v", value.Err())
	}

	// generate export
	exportPath := cue.ParsePath(config.Spec.Export)
	if exportPath.Err() != nil {
		return fmt.Errorf("parse export path: %v", exportPath.Err())
	}
	export := value.LookupPath(exportPath)
	if export.Err() != nil {
		return fmt.Errorf("lookup path: %v", export.Err())
	}

	// dump objects to yaml
	yamlString, err := cueyaml.MarshalStream(export)
	if err != nil {
		return fmt.Errorf("marshal stream: %v", instance.Err)
	}

	fixedYamlString := noBinaryPrefix(yamlString)
	fmt.Print(fixedYamlString)

	return nil
}

func noBinaryPrefix(yml string) string {
	return strings.ReplaceAll(yml, ": !!binary ", ": ")
}
