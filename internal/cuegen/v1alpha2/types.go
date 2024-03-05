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
	"crypto/sha1"
	"fmt"
	"io"
	"io/fs"
)

type CuegenConfig struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   Meta   `yaml:"metadata"`
	Spec       Spec   `yaml:"spec"`
}

type Meta struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	AppVersion string `yaml:"appVersion"`
	PkgVersion string `yaml:"pkgVersion"`
}

type Spec struct {
	ObjectsPath    string     `yaml:"objectsPath"`
	SecretDataPath string     `yaml:"secretDataPath"`
	Imports        []Location `yaml:"imports"`
	Packages       []Location `yaml:"packages"`
	Debug          bool       `yaml:"debug"`
}

type Package struct {
	Imports        []Location `yaml:"imports"`
	Packages       []Location `yaml:"packages"`
	ObjectsPath    string
	SecretDataPath string
	Name           string
	Version        string
	Debug          bool
	Kind           Kind
	IsMain         bool
	Resource       Resource
}

type Resource struct {
	FS       fs.FS
	Type     string
	Location Location
}

type Detect struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

type Location struct {
	URI     string
	Path    string
	Ref     string
	Package string
}

func (l Location) Hash() string {
	hash := sha1.New()
	hash.Write([]byte(fmt.Sprintf("%#v", l)))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

type Cuegen struct {
	Packages []Package
	Output   io.Writer
}
