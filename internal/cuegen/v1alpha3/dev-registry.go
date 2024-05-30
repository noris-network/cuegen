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
	registry, err := modconfig.NewRegistry(nil)
	if err != nil {
		panic(err)
	}
	return DevRegistry{registry: registry, root: root}
}

func (r DevRegistry) Fetch(ctx context.Context, m module.Version) (module.SourceLoc, error) {
	loc, err := r.getDevLoc(m)
	if err != nil {
		//fmt.Printf("getLoc: %v\n", err)
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
	//fmt.Printf("@@@ %v    ==> %v\n", m.String(), path)
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
