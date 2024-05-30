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

func Exec(config Cuegen) error {

	// new context
	ctx := cuecontext.New(cuecontext.EvaluatorVersion(cuecontext.EvalDefault))

	// prepare load.Config
	loadConfig := load.Config{}
	if loMo := os.Getenv("CUEGEN_USE_LOCAL_MODULES"); loMo != "" {
		loadConfig.Registry = NewDevRegistry(loMo)
	}

	// only load one instance
	instance := load.Instances([]string{"."}, &loadConfig)[0]
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
