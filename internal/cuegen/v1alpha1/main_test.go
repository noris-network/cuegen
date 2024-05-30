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

import "testing"

func Test_parseGitURL(t *testing.T) {
	type args struct {
		component string
	}
	tests := []struct {
		name     string
		args     args
		wantUrl  string
		wantRef  string
		wantFrag string
		wantErr  bool
	}{
		{
			name:     "http url",
			args:     args{component: "https://github.com/nxcc/cuegen-example-component-d"},
			wantUrl:  "https://github.com/nxcc/cuegen-example-component-d",
			wantRef:  "",
			wantFrag: "",
			wantErr:  false,
		},
		{
			name:     "http url with tag ref",
			args:     args{component: "https://github.com/nxcc/cuegen-example-component-d?ref=v1"},
			wantUrl:  "https://github.com/nxcc/cuegen-example-component-d",
			wantRef:  "v1",
			wantFrag: "",
			wantErr:  false,
		},
		{
			name:     "http url with tag ref and fragment",
			args:     args{component: "https://github.com/nxcc/cuegen-example-component-d?ref=v1#subdir"},
			wantUrl:  "https://github.com/nxcc/cuegen-example-component-d",
			wantRef:  "v1",
			wantFrag: "subdir",
			wantErr:  false,
		},
		{
			name:     "ssh url",
			args:     args{component: "git@github.com:noris-network/cuegen.git"},
			wantUrl:  "git@github.com:noris-network/cuegen.git",
			wantRef:  "",
			wantFrag: "",
			wantErr:  false,
		},
		{
			name:     "ssh url with tag ref",
			args:     args{component: "git@github.com:noris-network/cuegen.git?ref=v1"},
			wantUrl:  "git@github.com:noris-network/cuegen.git",
			wantRef:  "v1",
			wantFrag: "",
			wantErr:  false,
		},
		{
			name:     "ssh url with tag ref and fragment",
			args:     args{component: "git@github.com:noris-network/cuegen.git?ref=v1#subdir"},
			wantUrl:  "git@github.com:noris-network/cuegen.git",
			wantRef:  "v1",
			wantFrag: "subdir",
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUrl, gotRef, gotFrag, err := parseGitURL(tt.args.component)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotUrl != tt.wantUrl {
				t.Errorf("parseGitURL() gotUrl = %v, want %v", gotUrl, tt.wantUrl)
			}
			if gotRef != tt.wantRef {
				t.Errorf("parseGitURL() gotRef = %v, want %v", gotRef, tt.wantRef)
			}
			if gotFrag != tt.wantFrag {
				t.Errorf("parseGitURL() gotFrag = %v, want %v", gotFrag, tt.wantFrag)
			}
		})
	}
}
