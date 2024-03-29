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

cuegen: close({
	apiVersion: string
	kind:       "Cuegen" | "CuegenPackage" | "CuegenLibrary"
	metadata: {
		if kind == "Cuegen" || kind == "CuegenPackage" {
			appVersion: *"" | string
			pkgVersion: *"" | string
			version:    *(appVersion + "~" + pkgVersion) | string
		}
		if kind == "CuegenLibrary" {
			version: string
		}
		name: string
	}
	spec: close({
		debug: *false | bool
		if kind == "Cuegen" || kind == "CuegenPackage" {
			imports: [...#pkgloc]
		}
		if kind == "Cuegen" {
			packages: [...#pkgloc]
			postProcess: *"" | string
		}
		if kind != "CuegenLibrary" {
			objectsPath:    *"objects" | string
			secretDataPath: *"secret" | string
		}
	})
})

#pkgloc:
	{uri: *"" | string, path: *"" | string, ref: *"" | string} |
	{package: *"" | string, path: *"" | string}
