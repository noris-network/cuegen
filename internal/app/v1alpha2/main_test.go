package v1alpha2

import (
	_ "embed"
	"reflect"
	"testing"
)

func Test_parseGitURL(t *testing.T) {
	type args struct {
		component string
	}
	tests := []struct {
		name    string
		args    args
		wantGr  GitRef
		wantErr bool
	}{
		{
			name: "simple",
			args: args{component: "http://srv1.example.com/packages/mypack.git"},
			wantGr: GitRef{
				Package: "srv1.example.com/packages/mypack",
				URI:     "http://srv1.example.com/packages/mypack.git",
			},
		},
		{
			name: "with ref",
			args: args{component: "http://srv2.example.com/packages/mypack.git?ref=v1"},
			wantGr: GitRef{
				Package: "srv2.example.com/packages/mypack",
				URI:     "http://srv2.example.com/packages/mypack.git",
				Ref:     "v1",
			},
		},
		{
			name: "with path parameter",
			args: args{component: "http://srv3.example.com/packages/mypack.git?ref=v1&path=sub/a"},
			wantGr: GitRef{
				Package: "srv3.example.com/packages/mypack/sub/a",
				URI:     "http://srv3.example.com/packages/mypack.git",
				Ref:     "v1",
				Path:    "sub/a",
			},
		},
		{
			name: "with path after .git",
			args: args{component: "http://srv4.example.com/packages/mypack.git/sub/a?ref=v1"},
			wantGr: GitRef{
				Package: "srv4.example.com/packages/mypack/sub/a",
				URI:     "http://srv4.example.com/packages/mypack.git",
				Ref:     "v1",
				Path:    "sub/a",
			},
		},
		{
			name: "with empty after .git",
			args: args{component: "http://srv5.example.com/packages/mypack.git/?ref=v1"},
			wantGr: GitRef{
				Package: "srv5.example.com/packages/mypack",
				URI:     "http://srv5.example.com/packages/mypack.git",
				Ref:     "v1",
			},
		},
		{
			name:    "with path parameter and after .git",
			args:    args{component: "http://srv6.example.com/packages/mypack.git/sub/a?ref=v1&path=x"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGr, err := parseGitURL(tt.args.component)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotGr, tt.wantGr) {
				t.Errorf("parseGitURL() =\n  got  %#v,\n  want %#v", gotGr, tt.wantGr)
			}
		})
	}
}
