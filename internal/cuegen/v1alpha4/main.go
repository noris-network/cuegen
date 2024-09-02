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
	"log/slog"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/interpreter/embed"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/pkg/encoding/yaml"
)

const (
	V1Alpha1 = "v1alpha1"
	V1Alpha4 = "v1alpha4"
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

var (
	Default = Cuegen{
		ApiVersion: V1Alpha4,
		Spec:       Spec{Export: "objects"},
	}
	cuegenWrapper = os.Getenv("CUEGEN_SKIP_DECRYPT") != "true"
)

func Exec(config Cuegen, path string) error {
	if cuegenWrapper {
		execWrapper()
	}

	slog := slog.With("app", "cuegen")

	os.Setenv("CUE_EXPERIMENT", "embed")
	ctx := cuecontext.New(cuecontext.Interpreter(embed.New()))
	instance := load.Instances([]string{path}, nil)[0]
	if instance.Err != nil {
		slog.Debug("load instance", "err", instance.Err)
		return fmt.Errorf("cue: %v", instance.Err)
	}
	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		slog.Debug("build instance", "err", value.Err())
		return fmt.Errorf("cue: %v", value.Err())
	}
	objects := value.LookupPath(cue.ParsePath(config.Spec.Export))

	err := objects.Validate(cue.Concrete(true), cue.Final())
	if err != nil {
		slog.Debug("validate", "err", err)
		return fmt.Errorf("cue: %v", err)
	}

	yamlString, err := yaml.MarshalStream(objects)
	// yamlString, err := yaml.MarshalStream(value)
	if err != nil {
		slog.Debug("marshal", "err", err)
		return fmt.Errorf("marshal stream: %v", err)
	}
	fmt.Print(noBinaryPrefix(yamlString))
	return nil
}

func noBinaryPrefix(yml string) string {
	return strings.ReplaceAll(yml, ": !!binary ", ": ")
}
