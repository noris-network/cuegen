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
	"archive/zip"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/forensicanalysis/gitfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Config struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Components     []string `yaml:"components"`
	Debug          bool     `yaml:"debug"`
	ObjectsPath    string   `yaml:"objectsPath"`
	SecretDataPath string   `yaml:"secretDataPath"`
	ChartRoot      string
	CheckPath      string   `yaml:"checkPath"`
	CheckPaths     []string `yaml:"checkPaths"`
	RootFS         *fs.FS
}

type Component struct {
	Filesystem fs.FS
	ID         string
	Path       string
	Type       string
}

type Components map[string]Component

type Cuegen struct {
	Components     Components
	Debug          bool
	ObjectsPath    string
	SecretDataPath string
	CheckPaths     []string
	ChartRoot      string
	DumpOverlaysTo string
	RootFS         *fs.FS
}

// Exec initializes the Cuegen struct and executes cuegen
func Exec(config Config) error {

	if os.Getenv("CUEGEN_DEBUG") == "true" {
		config.Debug = true
	}

	// apply preferred defaults when all required fields are empty
	if config.ObjectsPath == "" &&
		config.SecretDataPath == "" &&
		config.CheckPath == "" && len(config.CheckPaths) == 0 {
		if config.Debug {
			log.Println("applying preferred defaults for ObjectsPath, SecretDataPath, and CheckPaths")
		}
		config.ObjectsPath = "objects"
		config.SecretDataPath = "secret.*.data"
		config.CheckPaths = []string{"values", "global"}
	}

	cg := Cuegen{
		Debug:          config.Debug,
		ObjectsPath:    config.ObjectsPath,
		ChartRoot:      config.ChartRoot,
		SecretDataPath: config.SecretDataPath,
		RootFS:         config.RootFS,
	}

	if dumpDir := os.Getenv("DUMP_OVERLAYS_TO"); dumpDir != "" && strings.HasPrefix(dumpDir, "/") {
		if _, err := os.Stat(dumpDir); err != nil {
			return fmt.Errorf("Exec: DUMP_OVERLAYS_TO: %v", err)
		}
		cg.DumpOverlaysTo = dumpDir
	}

	if config.CheckPath != "" {
		cg.CheckPaths = []string{config.CheckPath}
	}
	cg.CheckPaths = append(cg.CheckPaths, config.CheckPaths...)

	if config.ObjectsPath == "" {
		return fmt.Errorf("Exec: objectsPath is not set")
	}

	components, err := cg.getComponents(config.Components)
	if err != nil {
		return fmt.Errorf("Exec: %v", err)
	}
	cg.Components = components

	if cg.RootFS != nil {
		cg.Components["remoteRootFS"] = Component{
			Filesystem: *cg.RootFS,
			Type:       "remoterootfs",
		}
	}

	if cg.Debug {
		cg.PrintConfig()
	}

	return cg.Exec()
}

// getComponents loads components from the given paths
func (cg Cuegen) getComponents(componentPaths []string) (Components, error) {
	components := Components{}

	for _, componentPath := range componentPaths {
		component := Component{Path: componentPath, ID: generateID(componentPath)}
		switch {

		case strings.HasPrefix(componentPath, "http://") || strings.HasPrefix(componentPath, "https://") || strings.HasPrefix(componentPath, "git@"):
			gitfs, err := GetGitFS(componentPath)
			if err != nil {
				return nil, fmt.Errorf("getComponents: %v", err)
			}
			component.Type = "gitfs"
			component.Filesystem = gitfs

		case strings.HasSuffix(componentPath, ".zip"):
			zipfs, err := getZipFS(componentPath)
			if err != nil {
				return nil, fmt.Errorf("getComponents: %v", err)
			}
			component.Type = "zipfs"
			component.Filesystem = zipfs

		default:
			// TODO: check if within allowed path
			componentPath = os.ExpandEnv(componentPath)

			fileInfo, err := os.Stat(componentPath)
			if !(err == nil && fileInfo.IsDir()) {
				return nil, fmt.Errorf("getComponents: could not open component %q", componentPath)
			}
			component.Type = "dirfs"
			component.Filesystem = os.DirFS(componentPath)
		}

		components[component.ID] = component
	}
	return components, nil
}

// generateID generates id from a given string
func generateID(name string) string {
	bs := sha256.Sum256([]byte(name))
	return base32.StdEncoding.EncodeToString(bs[:])[:10]
}

// GetGitFS returns a fs.FS from the given git repository URL
func GetGitFS(component string) (fs.FS, error) {
	uri, ref, subDir, err := parseGitURL(component)
	if err != nil {
		return nil, fmt.Errorf("GetGitURL: open: %v", err)
	}

	opts := git.CloneOptions{
		URL: os.ExpandEnv(uri),
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
	}(ref)
	if err != nil {
		return nil, fmt.Errorf("getGitFS: open: %v", err)
	}
	if subDir == "" {
		return fsys, nil
	}
	subfsys, err := fs.Sub(fsys, subDir)
	if err != nil {
		return nil, fmt.Errorf("getGitFS: sub: %v", err)
	}
	return subfsys, nil
}

// getZipFS returns a fs.FS from the given zip file.
func getZipFS(zipfile string) (fs.FS, error) {
	// TODO: check if within allowed path
	zipfile = os.ExpandEnv(zipfile)

	// open zip file
	reader, err := zip.OpenReader(zipfile)
	if err != nil {
		return nil, fmt.Errorf("getZipFS: open reader: %v", err)
	}
	// open root dir
	root, err := reader.Open(".")
	if err != nil {
		return nil, fmt.Errorf("getZipFS: open: %v", err)
	}
	// read root dir
	entries, err := root.(fs.ReadDirFile).ReadDir(0)
	if err != nil {
		return nil, fmt.Errorf("getZipFS: read dir: %v", err)
	}
	// just one top dir?
	subdir := "."
	if len(entries) == 1 && entries[0].IsDir() {
		subdir = entries[0].Name()
	}
	// create sub filesystem
	subfs, err := fs.Sub(reader, subdir)
	if err != nil {
		return nil, fmt.Errorf("getZipFS: sub: %v", err)
	}
	return subfs, nil
}

func parseGitURL(component string) (uri, ref, frag string, err error) {
	isSshURL := false

	if strings.HasPrefix(component, "git@") {
		component = strings.TrimPrefix(component, "git@")
		isSshURL = true
	}
	u, err := url.Parse(component)
	if err != nil {
		return "", "", "", fmt.Errorf("parse url: %v", err)
	}

	q := u.Query()
	if len(q) > 1 {
		return "", "", "", fmt.Errorf("component url: too many parameters: %v", err)
	}

	ref = q.Get("ref")
	if len(q) == 1 && ref == "" {
		return "", "", "", errors.New("getGitFS: only parameter ref supported %v")
	}
	u.RawQuery = ""

	frag = u.Fragment
	u.Fragment = ""

	uri = u.String()
	if isSshURL {
		uri = "git@" + uri
	}
	return
}
