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
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"

	"github.com/forensicanalysis/gitfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha2"
)

func parseGitURL(importPath string) (loc cuegen.Location, err error) {
	u, err := url.Parse(importPath)
	if err != nil {
		return cuegen.Location{}, fmt.Errorf("parse url: %v", err)
	}

	q := u.Query()

	loc.Path = q.Get("path")
	delete(q, "path")

	loc.Ref = q.Get("ref")
	delete(q, "ref")

	if len(q) > 0 {
		return cuegen.Location{},
			errors.New("parseGitURL: only parameters 'ref' and 'path' supported")
	}

	u.RawQuery = ""
	loc.URI = u.String()

	if strings.Contains(loc.URI, ".git/") {
		if loc.Path != "" {
			return cuegen.Location{},
				errors.New("path paramter and path url path found")
		}
		_, loc.Path, _ = strings.Cut(u.String(), ".git/")
	}

	loc.Package = u.Host + u.Path

	if strings.HasSuffix(loc.Package, ".git") {
		loc.Package = strings.TrimSuffix(loc.Package, ".git") + "/" + loc.Path
	}

	if strings.Contains(loc.URI, ".git/") {
		uri, _, _ := strings.Cut(loc.URI, ".git/")
		loc.URI = uri + ".git"
	}

	loc.Package = strings.TrimSuffix(loc.Package, ".git")
	loc.Package = strings.Replace(loc.Package, ".git/", "/", 1)
	loc.Package = strings.TrimSuffix(loc.Package, "/")

	return
}

// getGitFS returns a cuegen.Resource from the given cuegen.Location
func getGitFS(loc cuegen.Location) (cuegen.Resource, error) {
	opts := git.CloneOptions{
		URL:          loc.URI,
		SingleBranch: true,
		Depth:        1,
	}
	if os.Getenv("CUEGEN_HTTP_USERNAME") != "" && os.Getenv("CUEGEN_HTTP_PASSWORD") != "" {
		opts.Auth = &http.BasicAuth{
			Username: os.Getenv("CUEGEN_HTTP_USERNAME"),
			Password: os.Getenv("CUEGEN_HTTP_PASSWORD"),
		}
	}

	fsys, err := func(ref string) (*gitfs.GitFS, error) {
		if ref != "" {
			// tag?
			opts.ReferenceName = plumbing.ReferenceName("refs/tags/" + ref)
			fsys, err := gitfs.NewWithOptions(&opts)
			if err == nil {
				return fsys, nil
			}
			// branch?
			opts.ReferenceName = plumbing.ReferenceName("refs/heads/" + ref)
			fsys, err = gitfs.NewWithOptions(&opts)
			if err == nil {
				return fsys, nil
			}
		}
		fsys, err := gitfs.NewWithOptions(&opts)
		if err != nil {
			return nil, err
		}
		return fsys, nil
	}(loc.Ref)
	if err != nil {
		return cuegen.Resource{}, fmt.Errorf("getGitFS: open: %v", err)
	}

	rsc := cuegen.Resource{Type: "git", Location: loc}
	if loc.Path == "" {
		rsc.FS = fsys
		return rsc, nil
	}
	subFsys, err := fs.Sub(fsys, loc.Path)
	if err != nil {
		return cuegen.Resource{}, fmt.Errorf("getGitFS: sub: %v", err)
	}
	rsc.FS = subFsys
	return rsc, nil
}
