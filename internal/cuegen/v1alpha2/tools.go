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
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"log/slog"
	"strings"
)

func dumpPackage(pkg Package) {
	slog.Debug("    ,-------- Package dump " + pkg.Name)
	// object
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		slog.Debug("    |" + scanner.Text())
	}
	slog.Debug("    |-------- fs.FS")
	// fs.FS
	if err := fs.WalkDir(pkg.Resource.FS, ".",
		func(filename string, entry fs.DirEntry, err error) error {
			if filename == ".git" {
				return fs.SkipDir
			}
			slog.Debug("    |FS>  " + filename)
			return nil
		}); err != nil {
		panic(err)
	}
	slog.Debug("    `-------- end")
}

// removeBinaryPrefix works around cueyaml.MarshalStream adding "!!binary" in front
// of base64 encoded []byte values. k8s rejects secrets with the binary indicator.
func removeBinaryPrefix(yml string) string {
	return strings.ReplaceAll(yml, ": !!binary ", ": ")
}
