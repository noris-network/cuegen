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
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
	"github.com/joho/godotenv"
	"go.mozilla.org/sops/v3/cmd/sops/formats"
	"go.mozilla.org/sops/v3/decrypt"
	"golang.org/x/exp/slices"

	"cuelang.org/go/cue/cuecontext"
	cueyaml "cuelang.org/go/pkg/encoding/yaml"
	"gopkg.in/yaml.v3"
)

type valueAttributes struct {
	attrs   []cue.Attribute
	subPath string
}

type pathValueAttributes map[string]valueAttributes

var cuegenAttrs = []string{"read", "readfile", "readmap"}

func (cg Cuegen) Exec() error {

	// load component overlays
	cfg, err := cg.buildLoadConfig()
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

	// collect cuegen attributes with path
	paths := pathValueAttributes{}
	seen := map[string]bool{}
	value.Walk(func(v cue.Value) bool {
		attrs := []cue.Attribute{}
		for _, attr := range v.Attributes(cue.ValueAttr) {
			if slices.Contains(cuegenAttrs, attr.Name()) {
				attrs = append(attrs, attr)
			}
		}
		if len(attrs) > 0 {
			filename := v.Pos().Filename()
			subPath, _ := strings.CutPrefix(filename, cg.ChartRoot+"/")
			cuePath := v.Path().String()
			if cg.Debug {
				// suggest workaround
				if strings.Contains(filename, "/cue.mod/gen/k8s.io/") {
					log.Printf("INFO: %v: in case some attributes are skipped by error,", cuePath)
					log.Printf("      try to add some dummy value to {}? see https://github.com/cue-lang/cue/discussions/2206")
				}
			}
			// prepare data for extending seenKey (workaround)
			attrsContents := ""
			for _, attr := range attrs {
				attrsContents = attrsContents + attr.Contents()
			}
			seenKey := fmt.Sprintf("%v:%v:%v", subPath, v.Pos().Offset(), attrsContents)
			// This workaround might still fail in some situations, because subPath
			// (derived from v.Pos().Filename()) points to k8s definitions when objects
			// are base on that. See above linked github discussion.
			if seen[seenKey] {
				return true
			}
			paths[cuePath] = valueAttributes{
				attrs:   attrs,
				subPath: subPath,
			}
			seen[seenKey] = true
		}
		return true
	}, func(v cue.Value) {})

	if cg.Debug {
		paths.PrintAttributes()
	}

	// process known attributes
	value, err = cg.processAttributes(value, paths)
	if err != nil {
		return fmt.Errorf("Exec: %v", err)
	}

	// check for non-concretene values in CheckPaths
	if len(cg.CheckPaths) > 0 {
		walkError := false
		for _, checkPath := range cg.CheckPaths {
			values := value.LookupPath(cue.ParsePath(checkPath))
			values.Walk(func(v cue.Value) bool {
				defaultValue, _ := v.Default()
				if !v.IsConcrete() && !defaultValue.IsConcrete() {
					walkError = true
					path := v.Path().String()
					log.Printf("VALUES: WARN: %q is not set", path)
				}
				return true
			}, func(v cue.Value) {})
		}
		if walkError {
			return errors.New("Exec: encountered non-concretene values in checkPath")
		}
	}

	// get objects for output
	objects := value.LookupPath(cue.ParsePath(cg.ObjectsPath))

	// check for non-concretene ObjectsPath
	walkError := false
	objects.Walk(func(v cue.Value) bool {
		defaultValue, _ := v.Default()
		if !v.IsConcrete() && !defaultValue.IsConcrete() {
			walkError = true
			path := v.Path().String()
			object, remain, _ := strings.Cut(path, ".")
			kind := value.LookupPath(cue.ParsePath(object + ".kind"))
			name := value.LookupPath(cue.ParsePath(object + ".metadata.name"))
			if kind.Err() == nil && name.Err() == nil {
				log.Printf(`WARN: "%s/%s/%s" is not set`, kind, name, remain)
			} else {
				log.Printf("WARN: %q is not set", path)
			}
		}
		return true
	}, func(v cue.Value) {})
	if walkError {
		return errors.New("Exec: encountered non-concretene values in objectsPath")
	}

	// dump objects to yaml
	yamlString, err := cueyaml.MarshalStream(objects)
	if err != nil {
		return fmt.Errorf("Exec: marshal stream: %v", err)
	}

	fmt.Print(noBinaryPrefix(yamlString))

	return nil
}

// noBinaryPrefix works around cueyaml.MarshalStream adding "!!binary" in front
// of base64 encoded []byte values. k8s rejects secrets with the binary indicator.
func noBinaryPrefix(yml string) string {
	return strings.ReplaceAll(yml, ": !!binary ", ": ")
}

const localFilesystem = "<local>"
const overlayFmt = "overlay-%v--%v"

// buildLoadConfig builds a 'load.Config' for 'load.Instances' containing all
// component files in the Overlay, filenames are prefixed with the component.ID
// to make them unique and to allow to locate the fs when attributes are found.
func (cg Cuegen) buildLoadConfig() (*load.Config, error) {
	overlay := map[string]load.Source{}
	if cg.Debug {
		log.Printf("Filesystems:")
	}
	for filesystemID, component := range cg.Components {
		if cg.Debug {
			log.Printf("    ---")
			log.Printf("    Filesystem:  %v", filesystemID)
			log.Printf("    Component:   %v", component.Path)
			log.Printf("    Type:        %v", component.Type)
		}
		if component.ID == localFilesystem {
			continue
		}
		if cg.Debug {
			log.Printf("    Files:")
		}
		if err := fs.WalkDir(
			component.Filesystem, ".",
			func(filename string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.Type().IsRegular() {
					return nil
				}
				if strings.HasSuffix(filename, ".cue") {
					data, err := fs.ReadFile(component.Filesystem, filename)
					if err != nil {
						return err
					}
					path := filepath.Join(cg.ChartRoot, fmt.Sprintf(overlayFmt, filesystemID, filename))
					overlay[path] = load.FromBytes(data)
					if cg.Debug {
						log.Printf("        * %v -> %v", filename, path)
					}
				} else {
					if cg.Debug {
						log.Printf("        * %v", filename)
					}
				}
				return nil
			},
		); err != nil {
			return &load.Config{}, fmt.Errorf("walkdir: %v", err)
		}
	}
	if cg.Debug {
		log.Printf("---")
	}
	return &load.Config{Overlay: overlay}, nil
}

// processAttributes processes all attributes that are known to cuegen (see cuegenAttrs)
func (cg Cuegen) processAttributes(value cue.Value, attrs pathValueAttributes) (cue.Value, error) {
	if cg.Debug {
		log.Printf("ProcessAttributes:")
	}
	for cuePath, valAttr := range attrs {
		if cg.Debug {
			log.Printf("    ---")
			log.Printf("    CuePath:    %v", cuePath)
			log.Printf("    SubPath:    %v", valAttr.subPath)
		}

		selectedFs := Component{
			Filesystem: os.DirFS("."),
			Type:       "dirfs",
			Path:       "<entrypoint>",
			ID:         localFilesystem,
		}

		for name, component := range cg.Components {
			prefix := fmt.Sprintf(overlayFmt, name, "")
			if strings.Contains(valAttr.subPath, prefix) {
				if cg.Debug {
					log.Printf("    load from:  %v", name)
				}
				selectedFs = component
				break
			}
		}
		for _, attr := range valAttr.attrs {
			if cg.Debug {
				log.Printf("    Attribute:  @%v(%v)", attr.Name(), attr.Contents())
			}
			if attr.Contents() == "" {
				return cue.Value{}, fmt.Errorf("empty attribute: @%v() at <%s>", attr.Name(), cuePath)
			}
			var err error
			switch attr.Name() {
			case "readfile":
				value, err = cg.attrReadFile(value, cuePath, attr, selectedFs)
			case "readmap":
				value, err = cg.attrReadMap(value, cuePath, attr, selectedFs)
			case "read":
				value, err = cg.attrRead(value, cuePath, attr, selectedFs)
			default:
				panic("unknown attribute")
			}
			if err != nil {
				return cue.Value{}, fmt.Errorf("@%v at <%s>: %v", attr.Name(), cuePath, err)
			}
		}
	}
	if cg.Debug {
		log.Printf("---")
	}
	return value, nil
}

// attrReadFile reads all files given in the attribute and puts them concatenated
// into the given cue.Value. With a '=trim' suffix all whitespace ( \t\n) will be
// removed from the front and end of the file. With '=nl' it will be ensured that
// one newline is a the end of the file. The updated cue.Value is then returned.
func (cg Cuegen) attrReadFile(value cue.Value, path string, attr cue.Attribute, component Component) (cue.Value, error) {
	alldata := ""
	for i := 0; i < attr.NumArgs(); i++ {
		file, flag := attr.Arg(i)
		data, err := cg.readFile(component, file)
		if err != nil {
			return value, fmt.Errorf("attrReadFile: %v", err)
		}
		switch flag {
		case "nl":
			alldata = alldata + strings.TrimRight(data, "\n") + "\n"
		case "trim":
			alldata = alldata + strings.Trim(data, " \t\n")
		default:
			alldata = alldata + data
		}
	}
	value = value.FillPath(cue.ParsePath(path), alldata)
	return value, nil
}

// attrRead reads all paths found in an attribute and fills the data into
// the given cue.Value. The updated cue.Value is then returned.
func (cg Cuegen) attrRead(value cue.Value, cuePath string, attr cue.Attribute, component Component) (cue.Value, error) {
	for i := 0; i < attr.NumArgs(); i++ {
		item, _ := attr.Arg(i)
		data, err := cg.readPath(component, item)
		if err != nil {
			return value, fmt.Errorf("attrRead: %v", err)
		}
		value = value.FillPath(cue.ParsePath(cuePath), data)
	}
	return value, nil
}

// attrReadMap reads all paths found in an attribute and fills the data as
// map[string]any into the given cue.Value. Structured data (JSON,YAML) is only
// allowed one level deep. All values matching the secretDataPath are stored as
// []byte, everything else as string. The updated cue.Value is then returned.
func (cg Cuegen) attrReadMap(value cue.Value, cuePath string, attr cue.Attribute, component Component) (cue.Value, error) {
	for i := 0; i < attr.NumArgs(); i++ {
		item, suffix := attr.Arg(i)
		data, err := cg.readPath(component, item)
		if err != nil {
			return value, fmt.Errorf("attrRead: %v", err)
		}
		secretPath := strings.Split(cg.SecretDataPath, ".")
		asBytes := true
		if suffix != "bytes" {
			pathItem := strings.Split(cuePath, ".")
			for n, key := range secretPath {
				if key != "*" && key != pathItem[n] {
					asBytes = false
					break
				}
			}
		}
		for k, v := range data {
			pathItems := cue.ParsePath(fmt.Sprintf("%v.%q", cuePath, k))
			switch stringValue := v.(type) {
			case string:
				if asBytes {
					value = value.FillPath(pathItems, []byte(stringValue))
				} else {
					value = value.FillPath(pathItems, stringValue)
				}
			default:
				return value, fmt.Errorf("value of type %T not allowed with readmap", v)
			}
		}
	}
	return value, nil
}

// readPath checks the given path, when it points to a directory all regular files
// in that path are red into a filename/contents key-value map. If it points to a
// single file, that is expected to contain some structured data (yaml,json,env).
// Data is returned as map[string]any{}.
func (cg Cuegen) readPath(component Component, path string) (map[string]any, error) {
	fileinfo, err := fs.Stat(component.Filesystem, path)
	if err != nil {
		return nil, fmt.Errorf("readPath: %v", err)
	}
	if fileinfo.IsDir() {
		entries, err := fs.ReadDir(component.Filesystem, path)
		if err != nil {
			return nil, fmt.Errorf("readPath: %v", err)
		}
		data := map[string]any{}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			stringData, err := cg.readFile(component, filepath.Join(path, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("readPath: %v", err)
			}
			data[entry.Name()] = stringData
		}
		return data, nil
	}
	if fileinfo.Mode().IsRegular() {
		return cg.readStructFile(component, path)
	}
	return nil, fmt.Errorf("readPath: can't handle %q", path)
}

// readStructFile reads a given file and tries to unmarshal the contents into
// a map[string]any{} which is returned.
func (cg Cuegen) readStructFile(component Component, file string) (map[string]any, error) {
	contents, err := cg.readFile(component, file)
	if err != nil {
		return nil, fmt.Errorf("readStructFile: %q: %v", file, err)
	}
	fileExt := filepath.Ext(file)
	data := map[string]any{}
	switch fileExt {
	case ".env":
		env, err := godotenv.Unmarshal(contents)
		if err != nil {
			return data, fmt.Errorf("readStructFile: %q: %v", file, err)
		}
		for k, v := range env {
			data[k] = v
		}
	case ".yml":
		fallthrough
	case ".yaml":
		err = yaml.Unmarshal([]byte(contents), &data)
		if err != nil {
			return data, fmt.Errorf("readStructFile: %q: %v", file, err)
		}
	case ".json":
		err = json.Unmarshal([]byte(contents), &data)
		if err != nil {
			return data, fmt.Errorf("readStructFile: %q: %v", file, err)
		}
	}
	return data, nil
}

// probeStrings for "sops" detection
var (
	probeStringsJsonOrYamlOrBinary = []string{
		"sops", "version", "unencrypted_suffix", "lastmodified", "mac",
	}
	probeStringsEnv = []string{
		"sops_version", "sops_unencrypted_suffix", "sops_lastmodified", "sops_mac",
	}
)

// readFile reads the contents of a given file, when it looks like sops encrypted,
// try to decrypt the contents. The result is retuned as string.
func (cg Cuegen) readFile(component Component, file string) (string, error) {
	if cg.Debug {
		log.Printf("    * readFile:   %v", file)
	}

	inputFile := file
	if component.ID == localFilesystem {
		found := false
		filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
			if path == inputFile {
				found = true
				return filepath.SkipAll
			}
			return nil
		})
		if !found {
			return "", fmt.Errorf("readFile: file %q not found inside working directory", inputFile)
		}
	}

	// read file from the component's filesystem
	data, err := fs.ReadFile(component.Filesystem, file)
	if err != nil {
		log.Printf("    filesystem content:")
		return "", fmt.Errorf("readFile: %q: %q: %v", component.Path, file, err)
	}

	var format formats.Format
	var probes []string
	stringData := string(data)

	// guess file format from extension
	switch filepath.Ext(file) {
	case ".env":
		format = formats.Dotenv
		probes = probeStringsEnv
	case ".yml":
		fallthrough
	case ".yaml":
		format = formats.Yaml
		probes = probeStringsJsonOrYamlOrBinary
	case ".json":
		format = formats.Json
		probes = probeStringsJsonOrYamlOrBinary
	default:
		format = formats.Binary
		probes = probeStringsJsonOrYamlOrBinary
	}

	// detect whether contents looks like sops encrypted
	allMatched := true
	for _, probe := range probes {
		allMatched = allMatched && strings.Contains(stringData, probe)
	}
	if allMatched {
		if cg.Debug {
			log.Printf("                  (sops encrypted)")
		}
		plaintext, err := decrypt.DataWithFormat(data, format)
		if err == nil {
			return string(plaintext), nil
		}
		log.Printf("WARN: %q: looks like sops encrypted, but decrypt failed: %v", file, err)
	}

	return stringData, nil
}

func (cg Cuegen) PrintConfig() {
	log.Printf("Cuegen:")
	log.Printf("    ObjectsPath:  %v", cg.ObjectsPath)
	log.Printf("    CheckPaths:   %+v", cg.CheckPaths)
	if len(cg.Components) > 0 {
		log.Printf("    Components:")
		for id, component := range cg.Components {
			log.Printf("        %-13s%+v", id+":", component.Path)
		}
	}
	log.Printf("---")
}

func (pv pathValueAttributes) PrintAttributes() {
	log.Printf("PathValueAttributes:")
	for path, valAttr := range pv {
		log.Printf("    ---")
		log.Printf("    CuePath:    %v", path)
		log.Printf("    SubPath:    %v", valAttr.subPath)
		for _, attr := range valAttr.attrs {
			log.Printf("    Attribute:  @%v(%v)", attr.Name(), attr.Contents())
		}
	}
	log.Printf("---")
}
