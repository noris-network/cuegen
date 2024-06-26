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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"cuelang.org/go/cue"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/joho/godotenv"
)

type pathValueAttributes map[string]valueAttributes

type valueAttributes struct {
	attrs   []cue.Attribute
	subPath string
}

var cuegenAttrs = []string{"read", "readfile", "readmap"}

func findAttributes(value cue.Value) pathValueAttributes {

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
			subPath, _ := strings.CutPrefix(filename, "cg.ChartRoot"+"/")
			cuePath := v.Path().String()
			attrsContents := ""
			seenKey := fmt.Sprintf("%v:%v:%v", subPath, v.Pos().Offset(), attrsContents)
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
	return paths
}

func processAttributes(value cue.Value, attrs pathValueAttributes) (cue.Value, error) {
	for cuePath, valAttr := range attrs {
		bpath := filepath.Dir(valAttr.subPath)
		for _, attr := range valAttr.attrs {
			if attr.Contents() == "" {
				return cue.Value{}, fmt.Errorf("empty attribute: @%v() at <%s>", attr.Name(), cuePath)
			}
			var err error
			switch attr.Name() {
			case "readfile":
				value, err = attrReadFile(value, cuePath, attr, bpath)
			case "readmap":
				value, err = attrReadMap(value, cuePath, attr, bpath)
			case "read":
				value, err = attrRead(value, cuePath, attr, bpath)
			default:
				panic("unknown attribute")
			}
			if err != nil {
				return cue.Value{}, fmt.Errorf("@%v at <%s>: %v", attr.Name(), cuePath, err)
			}
		}
	}
	return value, nil
}

func attrReadFile(value cue.Value, path string, attr cue.Attribute, bpath string) (cue.Value, error) {
	alldata := ""
	bytesFlag := ""
	for i := 0; i < attr.NumArgs(); i++ {
		file, flag := attr.Arg(i)
		data, err := readFile(filepath.Join(bpath, file))
		if err != nil {
			return value, fmt.Errorf("attrReadFile: %v", err)
		}
		switch flag {
		case "nl":
			alldata = alldata + strings.TrimRight(data, "\n") + "\n"
		case "trim":
			alldata = alldata + strings.TrimSpace(data)
		case "bytes":
			bytesFlag = flag
			fallthrough
		default:
			alldata = alldata + data
		}
	}
	if asBytes(bytesFlag) {
		value = value.FillPath(cue.ParsePath(path), []byte(alldata))
	} else {
		value = value.FillPath(cue.ParsePath(path), alldata)
	}
	return value, nil
}

func attrRead(value cue.Value, cuePath string, attr cue.Attribute, bpath string) (cue.Value, error) {
	for i := 0; i < attr.NumArgs(); i++ {
		item, _ := attr.Arg(i)
		data, err := readPath(filepath.Join(bpath, item))
		if err != nil {
			return value, fmt.Errorf("attrRead: %v", err)
		}
		value = value.FillPath(cue.ParsePath(cuePath), data)
	}
	return value, nil
}

func readFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("readFile: %q: %v", file, err)
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
		plaintext, err := decrypt.DataWithFormat(data, format)
		if err == nil {
			return string(plaintext), nil
		}
		log.Printf("WARN: %q: looks like sops encrypted, but decrypt failed: %v", file, err)
	}

	return stringData, nil
}

func attrReadMap(value cue.Value, cuePath string, attr cue.Attribute, bpath string) (cue.Value, error) {
	for i := 0; i < attr.NumArgs(); i++ {
		item, suffix := attr.Arg(i)
		data, err := readPath(filepath.Join(bpath, item))
		if err != nil {
			return value, fmt.Errorf("attrRead: %v", err)
		}
		for k, v := range data {
			pathItems := cue.ParsePath(fmt.Sprintf("%v.%q", cuePath, k))
			switch stringValue := v.(type) {
			case string:
				if asBytes(suffix) {
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

func readPath(path string) (map[string]any, error) {
	fileinfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("readPath: %v", err)
	}
	if fileinfo.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("readPath: %v", err)
		}
		data := map[string]any{}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			stringData, err := readFile(filepath.Join(path, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("readPath: %v", err)
			}
			data[entry.Name()] = stringData
		}
		return data, nil
	}
	if fileinfo.Mode().IsRegular() {
		return readStructFile(path)
	}
	return nil, fmt.Errorf("readPath: can't handle %q", path)
}

func asBytes(suffix string) bool {
	return suffix == "bytes"
}

func readStructFile(file string) (map[string]any, error) {
	contents, err := readFile(file)
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
	case ".yml", ".yaml":
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
