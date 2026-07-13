package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMajorVersion(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"v2", 2, true},
		{"v2.0.0", 2, true},
		{"v1beta1", 1, true},
		{"v0.16.6", 0, true},
		{"v3", 3, true},
		{"2alpha1", 2, true}, // leading v optional
		{"v", 0, false},
		{"alpha1", 0, false},
		{"", 0, false},
		{"vx", 0, false},
	}
	for _, tc := range tests {
		got, ok := majorVersion(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("majorVersion(%q) = (%d,%v) want (%d,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsV2(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"v2", true},
		{"v2.0.0", true},
		{"v1beta1", false},
		{"v0.16.6", false},
		{"v3", false},
		{"", false},
		{"garbage", false},
	}
	for _, tc := range tests {
		if got := isV2(tc.in); got != tc.want {
			t.Errorf("isV2(%q) = %v want %v", tc.in, got, tc.want)
		}
	}
}

// readAPIVersion must extract the literal from cuegen.cue without evaluating
// CUE, so unresolved imports in sibling fields do not block the parse. Both
// the nested struct-literal and the chained-label shorthand forms are valid.
func TestReadAPIVersion(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    string
		wantErr bool
	}{
		{
			name: "nested struct",
			src: `package control

cuegen: {
	apiVersion: "v2"
	spec: export: "export.objects"
}`,
			want: "v2",
		},
		{
			name: "chained shorthand",
			src: `package control

cuegen: apiVersion: "v1beta1"
`,
			want: "v1beta1",
		},
		{
			name: "unrelated imports ignored",
			src: `package control

import "noris.net/mcs/libmcs@v2"

cuegen: {
	apiVersion: "v2"
	spec: import: [libmcs]
}`,
			want: "v2",
		},
		{
			name: "no cuegen field",
			src: `package control

foo: "bar"
`,
			wantErr: true,
		},
		{
			name: "apiVersion not a string",
			src: `package control

cuegen: { apiVersion: 42 }
`,
			wantErr: true,
		},
		{
			name: "empty file",
			src: `package control
`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "cuegen.cue"), []byte(tc.src), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := readAPIVersion(filepath.Join(dir, "cuegen.cue"))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestReadAPIVersionMissingFile(t *testing.T) {
	if _, err := readAPIVersion(filepath.Join(t.TempDir(), "nope.cue")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
