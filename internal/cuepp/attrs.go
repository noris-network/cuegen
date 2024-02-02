package cuepp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

var cuegenAttrs = []string{"read", "readfile", "readmap"}

func (p CuePP) processAttribute(value, node cue.Value, attr cue.Attribute, fsys fs.FS) (cue.Value, error) {

	log.Printf("process attribute: %v @%v(%v)", node.Path().String(), attr.Name(), attr.Contents())

	var err error
	switch attr.Name() {
	case "readfile":
		value, err = p.attrReadFile(value, node.Path().String(), attr, fsys)
		if err != nil {
			err = fmt.Errorf("attrReadFile: %v", err)
		}
	case "readmap":
		value, err = p.attrReadMap(value, node.Path().String(), attr, fsys)
		if err != nil {
			err = fmt.Errorf("attrReadMap: %v", err)
		}
	case "read":
		value, err = p.attrRead(value, node.Path().String(), attr, fsys)
		if err != nil {
			err = fmt.Errorf("attrRead: %v", err)
		}
	default:
		err = fmt.Errorf("unknown attribute %q", attr.Name())
	}
	if err != nil {
		return cue.Value{}, err
	}

	return value, nil
}

func (p CuePP) attrReadFile(value cue.Value, path string, attr cue.Attribute, fsys fs.FS) (cue.Value, error) {

	alldata := ""
	asBytesFlag := false

	for i := 0; i < attr.NumArgs(); i++ {

		file, flag := attr.Arg(i)
		data, err := p.readFile(fsys, file)
		if err != nil {
			return value, fmt.Errorf("readFile: %v", err)
		}

		switch flag {
		case "nl":
			alldata = alldata + strings.TrimRight(data, "\n") + "\n"
		case "trim":
			alldata = alldata + strings.TrimSpace(data)
		case "bytes":
			asBytesFlag = true
			fallthrough
		default:
			alldata = alldata + data
		}
	}

	if p.asBytes(path, asBytesFlag) {
		value = value.FillPath(cue.ParsePath(path), []byte(alldata))
	} else {
		value = value.FillPath(cue.ParsePath(path), alldata)
	}

	return value, nil
}

func (p CuePP) attrReadMap(value cue.Value, cuePath string, attr cue.Attribute, fsys fs.FS) (cue.Value, error) {
	for i := 0; i < attr.NumArgs(); i++ {
		item, suffix := attr.Arg(i)
		data, err := p.readPath(fsys, item)
		if err != nil {
			return value, fmt.Errorf("attrRead: %v", err)
		}
		for k, v := range data {
			pathItems := cue.ParsePath(fmt.Sprintf("%v.%q", cuePath, k))
			switch stringValue := v.(type) {
			case string:
				if p.asBytes(cuePath, suffix == "bytes") {
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

func (p CuePP) attrRead(value cue.Value, cuePath string, attr cue.Attribute, fsys fs.FS) (cue.Value, error) {
	for i := 0; i < attr.NumArgs(); i++ {
		item, _ := attr.Arg(i)
		data, err := p.readPath(fsys, item)
		if err != nil {
			return value, fmt.Errorf("attrRead: %v", err)
		}
		value = value.FillPath(cue.ParsePath(cuePath), data)
	}
	return value, nil
}

func (p CuePP) readPath(fsys fs.FS, path string) (map[string]any, error) {
	fileinfo, err := fs.Stat(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("readPath: %v", err)
	}
	if fileinfo.IsDir() {
		entries, err := fs.ReadDir(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("readPath: %v", err)
		}
		data := map[string]any{}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			stringData, err := p.readFile(fsys, filepath.Join(path, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("readPath: %v", err)
			}
			data[entry.Name()] = stringData
		}
		return data, nil
	}
	if fileinfo.Mode().IsRegular() {
		return p.readStructFile(fsys, path)
	}
	return nil, fmt.Errorf("readPath: can't handle %q", path)
}

func (p CuePP) readStructFile(fsys fs.FS, file string) (map[string]any, error) {
	contents, err := p.readFile(fsys, file)
	if err != nil {
		return nil, fmt.Errorf("%q: %v", file, err)
	}
	fileExt := filepath.Ext(file)
	data := map[string]any{}
	switch fileExt {
	case ".env":
		env, err := godotenv.Unmarshal(contents)
		if err != nil {
			return data, fmt.Errorf("%q: %v", file, err)
		}
		for k, v := range env {
			data[k] = v
		}
	case ".yml":
		fallthrough
	case ".yaml":
		err = yaml.Unmarshal([]byte(contents), &data)
		if err != nil {
			return data, fmt.Errorf("%q: %v", file, err)
		}
	case ".json":
		err = json.Unmarshal([]byte(contents), &data)
		if err != nil {
			return data, fmt.Errorf("%q: %v", file, err)
		}
	default:
		return map[string]any{}, fmt.Errorf("%q: file type not supported", file)
	}
	return data, nil
}

var (
	probeStringsJsonOrYamlOrBinary = []string{
		"sops", "version", "unencrypted_suffix", "lastmodified", "mac",
	}
	probeStringsEnv = []string{
		"sops_version", "sops_unencrypted_suffix", "sops_lastmodified", "sops_mac",
	}
)

func (p CuePP) readFile(fsys fs.FS, file string) (string, error) {

	if p.Debug {
		log.Printf("    readFile: %v", file)
	}

	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return "", fmt.Errorf("ReadFile: %q: %v", file, err)
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
		if p.Debug {
			log.Printf("     decrypt with sops")
		}
		plaintext, err := decrypt.DataWithFormat(data, format)
		if err == nil {
			return string(plaintext), nil
		}
		log.Printf("WARN: %q: looks like sops encrypted, but decrypt failed: %v", file, err)
	}

	return stringData, nil
}

func (p CuePP) asBytes(cuePath string, asBytesFlag bool) bool {

	if p.SecretsPath == "" {
		return false
	}

	if asBytesFlag {
		return true
	}

	secretPathItems := strings.Split(p.SecretsPath, ".")
	pathItems := strings.Split(cuePath, ".")
	asBytes := true

	if len(secretPathItems) > len(pathItems) {
		asBytes = false
	}

	if asBytes {
		for n, key := range secretPathItems {
			if key != "*" && key != pathItems[n] {
				asBytes = false
				break
			}
		}
	}

	return asBytes
}
