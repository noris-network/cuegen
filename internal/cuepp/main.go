package cuepp

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
)

type CuePP struct {
	Tempdir     string
	Debug       bool
	SecretsPath string
}

var removeErrorString = regexp.MustCompile(`// explicit error \(_\|_ literal\) in source\s*`)
var stripPkgName = regexp.MustCompile(`:.*$`)

func (p CuePP) Process(src string, fsys fs.FS) (string, error) {

	// workaround disapearing-package-issue
	packageName := getPackage(src)

	// save working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %v", err)
	}
	defer os.Chdir(cwd)
	os.Chdir(p.Tempdir)

	// overlay
	overlay := map[string]load.Source{
		filepath.Join(p.Tempdir, "gen.cue"): load.FromString(src),
	}

	// load, will probably fail on first try because of missing packages
	bi := load.Instances([]string{}, &load.Config{Overlay: overlay})[0]
	if bi.Err != nil && strings.Contains(bi.Err.Error(), "cannot find package") {

		// fake missing packages
		for _, pack := range bi.ImportPaths {
			pkgName := stripPkgName.ReplaceAllString(pack, "")
			if err != nil {
				return "", fmt.Errorf("rx: %v", bi.Err)
			}
			file := filepath.Join(p.Tempdir, "cue.mod", "gen", pkgName, "gen.cue")
			overlay[file] = load.FromString("package " + filepath.Base(pkgName))
		}

		// try again
		bi = load.Instances([]string{}, &load.Config{Overlay: overlay})[0]
	}
	if bi.Err != nil {
		return "", fmt.Errorf("load: %v", bi.Err)
	}

	// build value
	ctx := cuecontext.New()
	value := ctx.BuildInstance(bi)
	if value.Err() != nil {
		return "", fmt.Errorf("build: %v", value.Err())
	}

	// find attributes
	value.Walk(func(v cue.Value) bool {
		for _, attr := range v.Attributes(cue.ValueAttr) {
			if slices.Contains(cuegenAttrs, attr.Name()) {
				value, err = p.processAttribute(value, v, attr, fsys)
				if err != nil {
					return false
				}
			}
		}
		return true
	}, func(v cue.Value) {})
	if err != nil {
		return "", fmt.Errorf("processAttribute: %v", err)
	}
	if value.Err() != nil {
		return "", fmt.Errorf("processAttribute: value:  %v", value.Err())
	}

	// re-create cue code
	rawcode, err := format.Node(value.Syntax())
	if err != nil {
		return "", fmt.Errorf("format: %v", err)
	}
	output := string(rawcode)

	// workaround bug...
	// probably related to https://github.com/cue-lang/cue/issues/2646
	output = removeErrorString.ReplaceAllString(output, "")

	// output
	out := &strings.Builder{}

	// workaround: restore package name
	// TODO: build minimal test case and file bug
	packageName2 := getPackage(output)
	if packageName2 == "" {
		fmt.Fprintf(out, "package %v\n\n", packageName)
	}

	fmt.Fprintf(out, "%v", output)
	return out.String(), nil
}

func getPackage(src string) string {
	scanner := bufio.NewScanner(strings.NewReader(src))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "package ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "package "))
		}
	}
	return ""
}
