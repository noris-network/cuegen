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
	"context"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/mod/modconfig"
	"cuelang.org/go/mod/module"
)

type DevRegistry struct {
	registry modconfig.Registry
	root     string
}

func NewDevRegistry(root string) modconfig.Registry {
	if debugLog {
		fmt.Printf("#@@@ NewDevRegistry:  %v\n", root)
	}
	registry, err := modconfig.NewRegistry(nil)
	if err != nil {
		panic(err)
	}
	return DevRegistry{registry: registry, root: root}
}

func (r DevRegistry) Fetch(ctx context.Context, m module.Version) (module.SourceLoc, error) {
	loc, err := r.getDevLoc(m)
	if err != nil {
		if debugLog {
			fmt.Printf("#@@@ getLoc %v: %v\n", m, err)
		}
		loc, err = r.registry.Fetch(ctx, m)
	}
	return loc, err
}

func (r DevRegistry) getDevLoc(m module.Version) (module.SourceLoc, error) {
	path := filepath.Join(r.root, m.BasePath())
	fileInfo, err := os.Stat(path)
	if err != nil {
		return module.SourceLoc{}, err
	}
	if !fileInfo.IsDir() {
		return module.SourceLoc{}, fmt.Errorf("%v is not a dir", path)
	}
	if debugLog {
		fmt.Printf("#@@@ %v  ==>  %v\n", m.String(), path)
	}
	return module.SourceLoc{FS: module.OSDirFS(path), Dir: "."}, nil
}

func (r DevRegistry) Requirements(ctx context.Context, m module.Version) ([]module.Version, error) {
	panic("not handled: Requirements")
	return r.registry.Requirements(ctx, m)
}

func (r DevRegistry) ModuleVersions(ctx context.Context, mpath string) ([]string, error) {
	panic("not handled: ModuleVersions")
	return r.registry.ModuleVersions(ctx, mpath)
}
