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

// running go test with -coverprofile=... will fail (e.g. package tests in vscode)

package app_test

import (
	"os"
	"testing"

	app "github.com/noris-network/cuegen/internal/app/v1alpha1"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"cuegen":               app.Main,
		"started_from_go_test": func() int { return 0 },
	}))
}

func TestCuegenLocalV1alpha1(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "v1alpha1/local",
	})
}

func TestCuegenLocalV1alpha2(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "v1alpha3/local",
	})
}

func TestCuegenRemoteV1alpha1(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "v1alpha1/remote",
	})
}

func TestCuegenRemoteV1alpha2(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "v1alpha1/remote",
	})
}
