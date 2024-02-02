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

package cuegen

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/pkg/encoding/yaml"
	"github.com/noris-network/cuegen/internal/cuepp"
	"mvdan.cc/sh/v3/shell"
)

type CuegenConfig struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Spec       Config `yaml:"spec"`
}

type Config struct {
	Imports        []string `yaml:"imports"`
	Debug          bool     `yaml:"debug"`
	ObjectsPath    string   `yaml:"objectsPath"`
	PostProcess    string   `yaml:"yqPostProcess"`
	CheckPath      string   `yaml:"checkPath"`
	CheckPaths     []string `yaml:"checkPaths"`
	SecretDataPath string   `yaml:"secretDataPath"`
}

type Package struct {
	Name   string
	Type   string
	FS     fs.FS
	IsRoot bool
}

type Cuegen struct {
	Packages []Package
	Config   Config
	EmptyDir string
}

func (cg Cuegen) Exec() error {

	emptyDir := ""
	if dir := os.Getenv("DUMP_OVERLAYS_TO"); dir != "" {
		emptyDir = dir
	} else {
		// new empty directory
		dir, err := os.MkdirTemp("", "cuegen-")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		emptyDir = dir
	}
	os.Chdir(emptyDir)

	// load component overlays
	cfg, err := cg.buildLoadConfig(emptyDir)
	if err != nil {
		return fmt.Errorf("Exec: %v", err)
	}

	// only load one instance
	instance := load.Instances([]string{"."}, cfg)[0]
	if instance.Err != nil {
		return fmt.Errorf("Exec: load instance: %v", instance.Err)
	}
	ctx := cuecontext.New()
	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		return fmt.Errorf("Exec: build instance: %v", value.Err())
	}

	// get objects for output
	objects := value.LookupPath(cue.ParsePath(cg.Config.ObjectsPath))

	// dump objects to yaml
	yamlString, err := yaml.MarshalStream(objects)
	if err != nil {
		return fmt.Errorf("Exec: marshal stream: %v", err)
	}

	fixedYamlString := noBinaryPrefix(yamlString)

	if cg.Config.PostProcess != "" {
		args, err := shell.Fields(cg.Config.PostProcess, nil)
		if err != nil {
			return err
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(fixedYamlString)
		cmd.Stdout = os.Stdout
		err = cmd.Run()
		if err != nil {
			return err
		}
	} else {
		fmt.Print(fixedYamlString)
	}

	return nil
}

// noBinaryPrefix works around cueyaml.MarshalStream adding "!!binary" in front
// of base64 encoded []byte values. k8s rejects secrets with the binary indicator.
func noBinaryPrefix(yml string) string {
	return strings.ReplaceAll(yml, ": !!binary ", ": ")
}

// buildLoadConfig builds a 'load.Config' for 'load.Instances' containing all
// component files in the Overlay, filenames are prefixed with the component.ID
// to make them unique and to allow to locate the fs when attributes are found.
func (cg Cuegen) buildLoadConfig(emptydir string) (*load.Config, error) {

	files := map[string][]byte{}

	if cg.Config.Debug {
		log.Printf("Overlay Filesystems:")
	}

	for _, pack := range cg.Packages {

		if cg.Config.Debug {
			log.Printf("    ---")
			log.Printf("    Package:  %v", pack.Name)
			log.Printf("    Type:     %v", pack.Type)
		}

		if cg.Config.Debug {
			log.Printf("    Files:")
		}

		if err := fs.WalkDir(
			pack.FS, ".",
			func(filename string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.Type().IsRegular() {
					return nil
				}
				if strings.HasPrefix(filename, ".git/") {
					return nil
				}
				if strings.HasPrefix(filename, "cue.mod/module.cue") {
					return nil
				}
				if strings.HasSuffix(filename, ".cue") {

					data, err := fs.ReadFile(pack.FS, filename)
					if err != nil {
						return err
					}

					p := cuepp.CuePP{
						Tempdir:     emptydir,
						Debug:       true,
						SecretsPath: cg.Config.SecretDataPath,
					}
					output, err := p.Process(string(data), pack.FS)
					if err != nil {
						return fmt.Errorf("process: %v", err)
					}

					overlayFilename := filename
					if !pack.IsRoot {
						overlayFilename = fmt.Sprintf("cue.mod/gen/%v/%v", pack.Name, filename)
					}
					path := filepath.Join(emptydir, overlayFilename)
					files[path] = []byte(output)

					if cg.Config.Debug {
						log.Printf("        * %v -> %v", filename, path)
					}
				}
				return nil
			},
		); err != nil {
			return &load.Config{}, fmt.Errorf("walkdir: %v", err)
		}
	}

	files[filepath.Join(emptydir, "cue.mod/module.cue")] = []byte(`module: "cuegen.local"`)

	overlay := map[string]load.Source{}
	dumpFiles := os.Getenv("DUMP_OVERLAYS_TO") != ""
	for path, data := range files {
		if dumpFiles {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return &load.Config{}, fmt.Errorf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
				return &load.Config{}, fmt.Errorf("writefile: %v", err)
			}
		}
		overlay[path] = load.FromBytes(data)
	}

	return &load.Config{Overlay: overlay}, nil
}
