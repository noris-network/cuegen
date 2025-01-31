// Copyright 2025 cuegen Authors
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
	"bytes"
	"fmt"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/interpreter/embed"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/yaml"
	"github.com/nxcc/cueconfig"
	yaml3 "gopkg.in/yaml.v3"
)

type Cuegen struct {
	Cuegen struct {
		ApiVersion string
		Spec       struct {
			Export string
		}
	}
}

func Exec(cuegen, path string) error {
	instance := load.Instances([]string{path}, nil)[0]
	if instance.Err != nil {
		return fmt.Errorf("cue: load instance: %v", instance.Err)
	}

	ctx := cuecontext.New(cuecontext.Interpreter(embed.New()))
	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		return fmt.Errorf("cue: build instance: %v", value.Err())
	}

	exportPath, err := getExportPath(cuegen)
	if err != nil {
		return fmt.Errorf("cuegen: %v", value.Err())
	}

	objects := value.LookupPath(cue.ParsePath(exportPath))
	err = objects.Validate(cue.Concrete(true), cue.Final())
	if err != nil {
		return fmt.Errorf("cue: validate: %v", err)
	}

	if os.Getenv("CUEGEN_USE_ITER") == "true" {
		err = printYaml(objects)
	} else {
		err = printYamlWorkaroundMarshalStreamPanic(objects)
	}
	if err != nil {
		return fmt.Errorf("exec: %v", err)
	}

	return nil
}

func noBinaryPrefix(yml string) string {
	return strings.ReplaceAll(yml, ": !!binary ", ": ")
}

func printYaml(objects cue.Value) error {
	iter, err := objects.List()
	if err != nil {
		return fmt.Errorf("objects list: %v", err)
	}

	yamlBytes, err := yaml.EncodeStream(iter)
	if err != nil {
		return fmt.Errorf("encode streamt: %v", err)
	}

	fmt.Print(noBinaryPrefix(string(yamlBytes)))
	return nil
}

func printYamlWorkaroundMarshalStreamPanic(objects cue.Value) error {
	yamlBytes, err := yaml.Encode(objects)
	if err != nil {
		return fmt.Errorf("encode: %v", err)
	}
	yamlBytes = bytes.ReplaceAll(yamlBytes, []byte(": !!binary "), []byte(": "))
	var objs []any
	err = yaml3.Unmarshal(yamlBytes, &objs)
	if err != nil {
		return fmt.Errorf("yaml3: %v", err)
	}
	yenc := yaml3.NewEncoder(os.Stdout)
	yenc.SetIndent(2)
	for _, obj := range objs {
		err := yenc.Encode(obj)
		if err != nil {
			return fmt.Errorf("yaml3: %v", err)
		}
	}
	return nil
}

func getExportPath(cuegen string) (string, error) {
	cfg := Cuegen{}
	if err := cueconfig.Load(cuegen, cuegenSchema, nil, nil, &cfg); err != nil {
		return "", fmt.Errorf("load config: %v: %v", cuegen, err)
	}
	return cfg.Cuegen.Spec.Export, nil
}

var cuegenSchema = []byte(`
	cuegen: {
		apiVersion:   *""        | string
		spec: export: *"objects" | string
	}
`)
