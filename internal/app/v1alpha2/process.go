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

package v1alpha2

import (
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"

	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha2"
	"github.com/nxcc/cueconfig"
)

func process(rootPkg cuegen.Package) error {
	slog.Info("==> prepare...")
	rootPkg.IsMain = true

	pkgs := []cuegen.Package{rootPkg}
	imps, err := packagesFromLocations(rootPkg.Imports)
	if err != nil {
		return fmt.Errorf("pkgsFromLocs: %v", err)
	}
	pkgs = append(pkgs, imps...)

	var output io.Writer = os.Stdout

	slog.Info("--> process root package", "package", rootPkg.Name)
	if err := runCuegen(pkgs, output, false); err != nil {
		return fmt.Errorf("runCuegen: %v: %v", rootPkg.Name, err)
	}

	rootPkg.IsMain = false

	for n, loc := range rootPkg.Packages {
		slog.Info("==> prepare....")
		mainPkg, err := packageFromLocation(loc)
		if err != nil {
			return fmt.Errorf("pkgFromLoc: %v", err)
		}
		mainPkg.IsMain = true

		imports, err := packagesFromLocations(mainPkg.Imports)
		if err != nil {
			return err
		}
		packages := append([]cuegen.Package{rootPkg, mainPkg}, imports...)

		slog.Info("--> process package", "package", mainPkg.Name)
		if err := runCuegen(packages, output, n == len(rootPkg.Packages)-1); err != nil {
			return fmt.Errorf("runCuegen: %v: %v", loc.Package, err)
		}
	}

	return nil
}

func runCuegen(packages []cuegen.Package, output io.Writer, isLast bool) error {
	for _, pkg := range packages {
		log.Printf("  - %v (%v) (%v)", pkg.Name, pkg.Kind, pkg.Version)
	}
	slog.Info("--> run cuegen")
	cg := cuegen.Cuegen{
		Packages: packages,
		Output:   output,
	}
	err := cg.Exec()
	if err != nil {
		return fmt.Errorf("cuegen: %v", err)
	}
	if !isLast {
		fmt.Fprintln(output, "---")
	}
	return nil
}

func packagesFromLocations(imports []cuegen.Location) ([]cuegen.Package, error) {
	pkgs := []cuegen.Package{}
	for _, loc := range imports {
		pkg, err := packageFromLocation(loc)
		if err != nil {
			return []cuegen.Package{}, fmt.Errorf("pkgFromLoc: %v", err)
		}
		if len(pkg.Imports) > 0 {
			return []cuegen.Package{}, fmt.Errorf(
				"imported package %v may not have further imports", pkg.Name,
			)
		}
		if len(pkg.Packages) > 0 {
			return []cuegen.Package{}, fmt.Errorf(
				"imported package %v may not pull further packages", pkg.Name,
			)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

var pkgCache = map[string]cuegen.Package{}

func packageFromLocation(loc cuegen.Location) (cuegen.Package, error) {
	hash := loc.Hash()
	if cached, found := pkgCache[hash]; found {
		slog.Info("    (cache get)", "loc", loc.Package, "uri", loc.URI)
		return cached, nil
	}
	res, err := loadResource(loc)
	if err != nil {
		return cuegen.Package{}, fmt.Errorf("loadResource: %v", err)
	}
	pkg, err := loadPackage(res)
	if err != nil {
		return cuegen.Package{}, fmt.Errorf("loadPackage: %v", err)
	}
	pkgCache[hash] = pkg
	slog.Info("    (cache add)", "package", loc.Package, "uri", loc.URI)
	return pkg, nil
}

func loadPackage(rsc cuegen.Resource) (cuegen.Package, error) {
	obj := struct{ Cuegen cuegen.CuegenConfig }{}
	err := cueconfig.LoadFS(rsc.FS, cuegenFile, CuegenSchema, nil, nil, &obj)
	if err != nil {
		return cuegen.Package{}, fmt.Errorf("cueconfig: %v (%+v)", err, rsc.Location)
	}

	return cuegen.Package{
		ObjectsPath:    obj.Cuegen.Spec.ObjectsPath,
		SecretDataPath: obj.Cuegen.Spec.SecretDataPath,
		Name:           obj.Cuegen.Metadata.Name,
		Version:        obj.Cuegen.Metadata.Version,
		Resource:       rsc,
		Packages:       obj.Cuegen.Spec.Packages,
		Imports:        obj.Cuegen.Spec.Imports,
		Debug:          obj.Cuegen.Spec.Debug,
		Kind:           cuegen.Parse(obj.Cuegen.Kind),
	}, nil
}

func loadResource(loc cuegen.Location) (cuegen.Resource, error) {
	switch {

	case loc.URI == "" && loc.Path != "":
		return cuegen.Resource{
			Type:     "os",
			Location: loc,
			FS:       os.DirFS(os.ExpandEnv(loc.Path)),
		}, nil

	case strings.HasPrefix(loc.URI, "http://") || strings.HasPrefix(loc.URI, "https://"):
		loc, err := parseGitURL(loc.URI)
		if err != nil {
			return cuegen.Resource{}, fmt.Errorf("parseGitURL: %v", err)
		}
		return getGitFS(loc)

	default:
		return cuegen.Resource{},
			errors.New("loadResource: case not handled, please report")
	}
}
