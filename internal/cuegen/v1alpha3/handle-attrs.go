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
			//for _, attr := range attrs {
			//	attrsContents = attrsContents + attr.Contents()
			//}
			seenKey := fmt.Sprintf("%v:%v:%v", subPath, v.Pos().Offset(), attrsContents)
			if seen[seenKey] {
				fmt.Printf("**SEEN** %v\n", seenKey)
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

	// ///////////
	// for cuePath, valAttr := range paths {
	// 	fmt.Printf(">>>>>>>>>>>> %v %v\n", cuePath, valAttr)
	// 	value = value.FillPath(cue.ParsePath(cuePath), "XXXXX!")
	// }
	// ///////////
	// printJSON(paths)
	return paths
}

var cg_Debug = !true

// //////////////////
// processAttributes processes all attributes that are known to cuegen (see cuegenAttrs)
func processAttributes(value cue.Value, attrs pathValueAttributes) (cue.Value, error) {
	//if cg_Debug {
	//log.Printf("ProcessAttributes:")
	//}
	for cuePath, valAttr := range attrs {
		if cg_Debug {
			log.Printf("    ---")
			log.Printf("    CuePath:    %v", cuePath)
			log.Printf("    SubPath:    %v", valAttr.subPath)
		}

		// for name, component := range cg.Components {
		// 	prefix := fmt.Sprintf(overlayFmt, name, "")
		// 	if strings.Contains(valAttr.subPath, prefix) {
		// 		if cg.Debug {
		// 			log.Printf("    load from:  %v", name)
		// 		}
		// 		selectedFs = component
		// 		break
		// 	}
		// }
		bpath := filepath.Dir(valAttr.subPath)
		for _, attr := range valAttr.attrs {
			if cg_Debug {
				log.Printf("    Attribute:  @%v(%v)", attr.Name(), attr.Contents())
			}
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
	if cg_Debug {
		log.Printf("---")
	}
	return value, nil
}

///

func attrReadFile(value cue.Value, path string, attr cue.Attribute, bpath string) (cue.Value, error) {
	alldata := ""
	bytesFlag := ""
	//attr.Pos
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
	if asBytes(path, bytesFlag) {
		value = value.FillPath(cue.ParsePath(path), []byte(alldata))
	} else {
		value = value.FillPath(cue.ParsePath(path), alldata)
	}
	return value, nil
}

// /
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
	if cg_Debug {
		log.Printf("    * readFile:   %v", file)
	}

	// read file from the component's filesystem
	data, err := os.ReadFile(file)
	if err != nil {
		log.Printf("    filesystem content: %v", err)
		return "", fmt.Errorf("readFile: %q: %v", file, err)
	}

	var format formats.Format
	var probes []string
	stringData := string(data)

	// // guess file format from extension
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
		if cg_Debug {
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

// /
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
				if asBytes(cuePath, suffix) {
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

//

func asBytes(cuePath, suffix string) bool {
	if suffix == "bytes" {
		return true
	}
	//secretPathItems := strings.Split(cg.SecretDataPath, ".")
	//pathItems := strings.Split(cuePath, ".")
	//asBytes := true
	//if len(secretPathItems) > len(pathItems) {
	//	asBytes = false
	//}
	//if asBytes && suffix != "bytes" {
	//	for n, key := range secretPathItems {
	//		if key != "*" && key != pathItems[n] {
	//			asBytes = false
	//			break
	//		}
	//	}
	//}
	//return asBytes
	return false
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

////////////////////

//lint:ignore U1000 (dev-only)
func printJSON(i interface{}, prefix ...string) {
	pre := fmt.Sprintf("%T>>>", i)
	if len(prefix) == 1 {
		pre = prefix[0] + ">"
	}
	data, err := json.MarshalIndent(i, pre+"  ", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(pre + "  " + string(data))
}
