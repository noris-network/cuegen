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
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/load"
	"github.com/noris-network/cuegen/internal/cuepp"
)

func (cg Cuegen) buildLoadConfig(emptydir string) (*load.Config, error) {

	mainPkg := Package{}
	for _, pkg := range cg.Packages {
		if pkg.IsMain && pkg.Kind == Kinds.CuegenPackage {
			mainPkg = pkg
			break
		}
	}

	files := map[string][]byte{}
	for _, pkg := range cg.Packages {

		if pkg.Debug {
			dumpPackage(pkg)
		}

		if err := fs.WalkDir(
			pkg.Resource.FS, ".",
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
				if pkg.Kind == Kinds.Cuegen && !pkg.IsMain {
					dir := filepath.Dir(filename)
					if dir == "." {
						slog.Debug("skip file in root directory of root package", "file", filename)
						return nil
					}
				}
				if strings.HasSuffix(filename, ".cue") {

					data, err := fs.ReadFile(pkg.Resource.FS, filename)
					if err != nil {
						return err
					}

					output := string(data)

					if cuegenProcessComment.MatchString(output) {
						p := cuepp.CuePP{
							Tempdir:     emptydir,
							Debug:       true,
							SecretsPath: pkg.SecretDataPath,
						}
						dir := filepath.Dir(filename)
						out, err := p.Process(output, dir, pkg.Resource.FS)
						if err != nil {
							return fmt.Errorf("process: %v", err)
						}
						output = out
					}

					overlayFilename := filename
					if pkg.Kind != Kinds.Cuegen {
						overlayFilename = fmt.Sprintf("cue.mod/gen/%v/%v", pkg.Resource.Location.Package, filename)
					}

					// add overlay into imported package
					if mainPkg.Kind != Kinds.UNKNOWN {
						overlayPrefix := "overlay/" + mainPkg.Name + "/"
						if pkg.Kind == Kinds.Cuegen && strings.HasPrefix(filename, overlayPrefix) {
							file := strings.TrimPrefix(filename, overlayPrefix)
							overlayFilename = fmt.Sprintf("cue.mod/gen/%v/overlay-%v",
								mainPkg.Resource.Location.Package, file,
							)
						}
					}

					files[overlayFilename] = []byte(output)
					slog.Debug("map file", "source", filename, "target", overlayFilename)
				}
				return nil
			},
		); err != nil {
			return &load.Config{}, fmt.Errorf("walkdir: %v", err)
		}

		if pkg.IsMain {

			gen := ""

			switch pkg.Kind {
			case Kinds.Cuegen:
				gen = "package main\n\n" +
					cuegenMergeObjects + ": objects\n" +
					"objects: [...]\n"
			case Kinds.CuegenPackage:
				gen = fmt.Sprintf(
					"package main\n\n"+
						"import %q\n\n"+
						cuegenMergeObjects+": [\n"+
						"\tif %v.%v != _|_ for o in %v.%v {o} \n"+
						"]\n",
					pkg.Resource.Location.Package,
					pkg.Name, pkg.ObjectsPath, pkg.Name, pkg.ObjectsPath,
				)
			default:
				panic("CuegenLibrary marked as main")
			}

			if gen != "" {
				files["load_objects_gen.cue"] = []byte(gen)
			}
		}
	}

	files["cue.mod/module.cue"] = []byte(`module: "cuegen.local"`)

	dumpPrefix := ""
	dumpFiles := os.Getenv("DUMP_OVERLAYS_TO") != ""
	if dumpFiles {
		if mainPkg.Kind == Kinds.UNKNOWN {
			dumpPrefix = filepath.Join(os.Getenv("DUMP_OVERLAYS_TO"), "root")
		} else {
			dumpPrefix = filepath.Join(os.Getenv("DUMP_OVERLAYS_TO"), mainPkg.Name, mainPkg.Version)
		}
		if err := os.RemoveAll(dumpPrefix); err != nil {
			return &load.Config{}, fmt.Errorf("cleanup dumpdir: %v", err)
		}
	}

	overlay := map[string]load.Source{}
	for path, data := range files {
		if dumpFiles {
			dumpto := filepath.Join(dumpPrefix, path)
			if err := os.MkdirAll(filepath.Dir(dumpto), 0o755); err != nil {
				return &load.Config{}, fmt.Errorf("mkdir: %v", err)
			}
			if err := os.WriteFile(dumpto, []byte(data), 0o644); err != nil {
				return &load.Config{}, fmt.Errorf("writefile: %v", err)
			}
		}
		overlay[filepath.Join(emptydir, path)] = load.FromBytes(data)
	}

	return &load.Config{Overlay: overlay}, nil
}
