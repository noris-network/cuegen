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
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/forensicanalysis/gitfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha2"
	"github.com/nxcc/cueconfig"
	"gopkg.in/yaml.v3"
)

const defaultYamlCuegenFile = "cuegen.yaml"
const defaultCueCuegenFile = "cuegen.cue"

var Build = "dev build"

func Main() int {

	checkForCuegenDir := false

	// this is handled in v1alpha1 code; uncomment when v1alpha1 support is removed
	//flag.BoolVar(&checkForCuegenDir, "is-cuegen-dir", false, "check current working directory for cuegen.{yaml,cue} (for cmp detection)")
	//flag.Parse()

	log.SetFlags(0)
	log.SetPrefix("# cuegen: ")

	// detect cuegen directory (for cmp-plugin)
	if checkForCuegenDir {
		if _, err := os.Stat("cuegen.cue"); err == nil {
			fmt.Println("true")
			return 0
		}
		if _, err := os.Stat("cuegen.yaml"); err == nil {
			fmt.Println("true")
			return 0
		}
		return 0
	}

	// check args
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Printf("cuegen version %v\n", Build)
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			log.Fatalln("Failed to read build info")
		}
		for _, dep := range bi.Deps {
			if dep.Path == "cuelang.org/go" {
				fmt.Printf("cue version %v\n", dep.Version)
				break
			}
		}
		return 0
	}
	if len(os.Args) != 2 {
		log.Fatalln("usage: cuegen <configfile>")
		return 1
	}

	// load root
	rootPack, arg := loadRootFS(os.Args[1])
	cuegenFile, err := findCuegenFile(rootPack.FS, arg)
	if err != nil {
		log.Fatalf("detectCuegenFile: %v", err)
	}

	// load config
	config, err := loadConfig(rootPack.FS, cuegenFile)
	if err != nil {
		log.Fatal(err)
	}

	// load packages
	packs, err := loadPackages(config.Imports)
	if err != nil {
		log.Fatal(err)
	}

	// build cuegen
	cg := cuegen.Cuegen{
		Config:   config,
		Packages: append(packs, rootPack),
	}
	if config.Debug {
		log.Printf("cuegen.Cuegen: %+v", cg)
		for _, p := range packs {
			ls(p.Name, p.FS)
		}
	}

	// run...
	err = cg.Exec()
	if err != nil {
		log.Fatalf("cuegen: %v", err)
	}
	return 0
}

// loadRootFS loads the root from the given path, this may be a local
// path, or a remote git repository
func loadRootFS(path string) (cuegen.Package, string) {
	rootPack := cuegen.Package{Name: "<root>", IsRoot: true}
	arg := ""
	switch {

	case strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://"):
		pack, err := GetGitFS(path)
		if err != nil {
			log.Fatal(err)
		}
		rootPack.FS = pack.FS
		rootPack.Type = pack.Type
		arg = "."

	default:
		rootPack.Type = "os"
		fileInfo, err := os.Stat(path)
		if err != nil {
			log.Fatal(err)
		}
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		switch {
		case strings.HasPrefix(path, "/") && fileInfo.IsDir():
			rootPack.FS = os.DirFS(path)
			arg = "."
		case strings.HasPrefix(path, "/") && fileInfo.Mode().Type().IsRegular():
			rootPack.FS = os.DirFS(filepath.Dir(path))
			arg = filepath.Base(path)
		case fileInfo.IsDir():
			rootPack.FS = os.DirFS(filepath.Join(cwd, path))
			arg = "."
		case fileInfo.Mode().Type().IsRegular():
			rootPack.FS = os.DirFS(filepath.Join(cwd, filepath.Dir(path)))
			arg = filepath.Base(path)
		default:
			log.Fatal("case not handled, please report")
		}
	}
	return rootPack, arg
}

// loadConfig loads the cuegen config from given fs.FS. When a directory is passed,
// cuegen will look for the default "cuegen.{cue,ya?ml}" in that directory.
func loadConfig(fsys fs.FS, file string) (cuegen.Config, error) {

	ext := filepath.Ext(file)
	if ext == ".cue" {
		return loadCueConfig(fsys, file)
	}
	if ext == ".yml" || ext == ".yaml" {
		return loadYamlConfig(fsys, file)
	}

	return cuegen.Config{}, errors.New("no config found")
}

// findCuegenFile ties to find the cuegen config file in given fs.FS
func findCuegenFile(rootfs fs.FS, path string) (string, error) {

	fileInfo, err := fs.Stat(rootfs, path)
	if err != nil {
		return "", err
	}

	if fileInfo.Mode().IsRegular() {
		return filepath.Base(path), nil
	}

	if fileInfo.IsDir() {
		rootfs, err := fs.Sub(rootfs, path)
		if err != nil {
			return "", err
		}
		// cuegen.cue?
		file := defaultCueCuegenFile
		fileInfo, err := fs.Stat(rootfs, file)
		if err == nil && fileInfo.Mode().IsRegular() {
			return file, nil
		}
		// cuegen.yaml?
		file = defaultYamlCuegenFile
		fileInfo, err = fs.Stat(rootfs, file)
		if err == nil && fileInfo.Mode().IsRegular() {
			return file, nil
		}
	}

	return "", fmt.Errorf("cuegen config %q not found", path)
}

func loadYamlConfig(fsys fs.FS, file string) (cuegen.Config, error) {
	fh, err := fsys.Open(file)
	if err != nil {
		return cuegen.Config{}, err
	}
	config := cuegen.CuegenConfig{}
	decoder := yaml.NewDecoder(fh)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		if errors.Is(err, io.EOF) {
			return cuegen.Config{}, nil
		}
		return cuegen.Config{}, err
	}
	return config.Spec, nil
}

//go:embed schema.cue
var cuegenConfigSchema []byte

func loadCueConfig(fsys fs.FS, file string) (cuegen.Config, error) {
	config := struct{ Cuegen cuegen.CuegenConfig }{}
	if err := cueconfig.LoadFS(fsys, file, cuegenConfigSchema, nil, nil, &config); err != nil {
		return cuegen.Config{}, fmt.Errorf("load cue: %v", err)
	}
	return config.Cuegen.Spec, nil
}

// loadPackages loads packages from the given import strings
func loadPackages(imports []string) ([]cuegen.Package, error) {
	packs := []cuegen.Package{}
	for _, imp := range imports {
		pack, err := loadPackage(imp)
		if err != nil {
			return []cuegen.Package{}, err
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

// loadPackage loads a package from the given import string
func loadPackage(importPath string) (cuegen.Package, error) {

	importPath = os.ExpandEnv(importPath)

	switch {

	case strings.HasPrefix(importPath, "http://") ||
		strings.HasPrefix(importPath, "https://") ||
		strings.HasPrefix(importPath, "git@"):
		return GetGitFS(importPath)

	// e.g. example.com/packages/name=/abs/local/path
	case strings.Contains(importPath, "=/"):
		name, directory, _ := strings.Cut(importPath, "=")
		directory = filepath.Clean(directory)
		fileInfo, err := os.Stat(directory)
		if !(err == nil && fileInfo.IsDir()) {
			return cuegen.Package{}, fmt.Errorf("could not open import %q as directory", directory)
		}
		return cuegen.Package{Name: name, Type: "os", FS: os.DirFS(directory)}, nil

	default:
		return cuegen.Package{}, fmt.Errorf("can not handle import path %q", importPath)

	}
}

// GetGitFS returns a fs.FS from the given git repository URL
func GetGitFS(component string) (cuegen.Package, error) {

	gitref, err := parseGitURL(component)
	if err != nil {
		return cuegen.Package{}, fmt.Errorf("GetGitURL: open: %v", err)
	}

	opts := git.CloneOptions{
		URL:          gitref.URI,
		SingleBranch: true,
		Depth:        1,
	}
	if os.Getenv("CUEGEN_HTTP_USERNAME") != "" && os.Getenv("CUEGEN_HTTP_PASSWORD") != "" {
		opts.Auth = &http.BasicAuth{
			Username: os.Getenv("CUEGEN_HTTP_USERNAME"),
			Password: os.Getenv("CUEGEN_HTTP_PASSWORD"),
		}
	}

	fsys, err := func(ref string) (*gitfs.GitFS, error) {
		if ref != "" {
			// tag?
			opts.ReferenceName = plumbing.ReferenceName("refs/tags/" + ref)
			fsys, err := gitfs.NewWithOptions(&opts)
			if err == nil {
				return fsys, nil
			}
			// branch?
			opts.ReferenceName = plumbing.ReferenceName("refs/heads/" + ref)
			fsys, err = gitfs.NewWithOptions(&opts)
			if err == nil {
				return fsys, nil
			}
		}
		fsys, err := gitfs.NewWithOptions(&opts)
		if err != nil {
			return nil, err
		}
		return fsys, nil
	}(gitref.Ref)
	if err != nil {
		return cuegen.Package{}, fmt.Errorf("getGitFS: open: %v", err)
	}

	pack := cuegen.Package{Name: gitref.Package, Type: "git"}
	if gitref.Path == "" {
		pack.FS = fsys
		return pack, nil
	}
	subFsys, err := fs.Sub(fsys, gitref.Path)
	if err != nil {
		return cuegen.Package{}, fmt.Errorf("getGitFS: sub: %v", err)
	}
	pack.FS = subFsys
	return pack, nil
}

type GitRef struct {
	Package string
	Ref     string
	URI     string
	Path    string
}

func parseGitURL(importPath string) (gr GitRef, err error) {

	u, err := url.Parse(importPath)
	if err != nil {
		return GitRef{}, fmt.Errorf("parse url: %v", err)
	}

	q := u.Query()

	gr.Path = q.Get("path")
	delete(q, "path")

	gr.Ref = q.Get("ref")
	delete(q, "ref")

	if len(q) > 0 {
		return GitRef{}, errors.New("getGitFS: only parameters 'ref' and 'path' supported")
	}

	u.RawQuery = ""
	gr.URI = u.String()

	if strings.Contains(gr.URI, ".git/") {
		if gr.Path != "" {
			return GitRef{}, errors.New("path paramter and path url path found")
		}
		_, gr.Path, _ = strings.Cut(u.String(), ".git/")
	}

	gr.Package = u.Host + u.Path

	if strings.HasSuffix(gr.Package, ".git") {
		gr.Package = strings.TrimSuffix(gr.Package, ".git") + "/" + gr.Path
	}

	if strings.Contains(gr.URI, ".git/") {
		uri, _, _ := strings.Cut(gr.URI, ".git/")
		gr.URI = uri + ".git"
	}

	gr.Package = strings.TrimSuffix(gr.Package, ".git")
	gr.Package = strings.Replace(gr.Package, ".git/", "/", 1)
	gr.Package = strings.TrimSuffix(gr.Package, "/")

	return
}

// ls lists the content of a passed fs
func ls(name string, fsys fs.FS) {
	if err := fs.WalkDir(fsys, ".",
		func(filename string, entry fs.DirEntry, err error) error {
			if filename == ".git" {
				return fs.SkipDir
			}
			log.Printf("[%v]  %v\n", name, filename)
			return nil
		}); err != nil {
		panic(err)
	}
}
