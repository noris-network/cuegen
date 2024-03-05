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
	"regexp"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/pkg/encoding/yaml"
)

const cuegenMergeObjects = "cuegen_generated_merge_objects"

var cuegenProcessComment = regexp.MustCompile("(?m)^//cuegen: process$")

func (c Cuegen) Exec() error {
	// new empty directory
	emptyDir, err := os.MkdirTemp("", "cuegen-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(emptyDir)

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	os.Chdir(emptyDir)
	defer os.Chdir(cwd)
	// load component overlays

	cfg, err := c.buildLoadConfig(emptyDir)
	if err != nil {
		return fmt.Errorf("Exec: %v", err)
	}

	// only load one instance
	instance := load.Instances([]string{"."}, cfg)[0]
	if instance.Err != nil {
		return fmt.Errorf("exec: load instance: %v", instance.Err)
	}
	ctx := cuecontext.New()
	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		return fmt.Errorf("exec: build instance: %v", value.Err())
	}

	// get objects for output
	objects := value.LookupPath(cue.ParsePath(cuegenMergeObjects))
	if objects.Err() != nil {
		return fmt.Errorf("exec: LookupPath: %v", objects.Err())
	}

	// dump objects to yaml
	yamlString, err := yaml.MarshalStream(objects)
	if err != nil {
		return fmt.Errorf("Exec: marshal stream: %v", err)
	}

	fixedYamlString := removeBinaryPrefix(yamlString)

	c.Output.Write([]byte(fixedYamlString))

	return nil
}
