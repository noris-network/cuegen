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
	"reflect"
	"testing"

	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha2"
)

func Test_parseGitURL(t *testing.T) {
	type args struct {
		importPath string
	}
	tests := []struct {
		name    string
		args    args
		wantLoc cuegen.Location
		wantErr bool
	}{
		{
			name: "simple",
			args: args{importPath: "http://srv1.example.com/packages/mypack.git"},
			wantLoc: cuegen.Location{
				URI:     "http://srv1.example.com/packages/mypack.git",
				Package: "srv1.example.com/packages/mypack",
			},
		},
		{
			name: "with ref",
			args: args{importPath: "http://srv2.example.com/packages/mypack.git?ref=v1"},
			wantLoc: cuegen.Location{
				URI:     "http://srv2.example.com/packages/mypack.git",
				Ref:     "v1",
				Package: "srv2.example.com/packages/mypack",
			},
		},
		{
			name: "with path parameter",
			args: args{importPath: "http://srv3.example.com/packages/mypack.git?ref=v1&path=sub/a"},
			wantLoc: cuegen.Location{
				URI:     "http://srv3.example.com/packages/mypack.git",
				Ref:     "v1",
				Path:    "sub/a",
				Package: "srv3.example.com/packages/mypack/sub/a",
			},
		},
		{
			name: "with path after .git",
			args: args{importPath: "http://srv4.example.com/packages/mypack.git/sub/a?ref=v1"},
			wantLoc: cuegen.Location{
				URI:     "http://srv4.example.com/packages/mypack.git",
				Ref:     "v1",
				Path:    "sub/a",
				Package: "srv4.example.com/packages/mypack/sub/a",
			},
		},
		{
			name: "with empty after .git",
			args: args{importPath: "http://srv5.example.com/packages/mypack.git/?ref=v1"},
			wantLoc: cuegen.Location{
				URI:     "http://srv5.example.com/packages/mypack.git",
				Ref:     "v1",
				Package: "srv5.example.com/packages/mypack",
			},
		},
		{
			name:    "with path parameter and after .git",
			args:    args{importPath: "http://srv6.example.com/packages/mypack.git/sub/a?ref=v1&path=x"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLoc, err := parseGitURL(tt.args.importPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotLoc, tt.wantLoc) {
				t.Errorf("parseGitURL() =\n  got  %#v,\n  want %#v", gotLoc, tt.wantLoc)
			}
		})
	}
}
