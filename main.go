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

package main

import (
	"os"

	v1alpha1 "github.com/noris-network/cuegen/internal/app/v1alpha1"
	v1alpha2 "github.com/noris-network/cuegen/internal/app/v1alpha2"
)

var build = "dev"

func main() {
	v1alpha1.Build = build
	v1alpha2.Build = build
	switch os.Getenv("CUEGEN_APIVERSION") {
	case "v1alpha2":
		os.Exit(v1alpha2.Main())
	default:
		os.Exit(v1alpha1.Main())
	}
}
