package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cue-exp/cueconfig"
	v1alpha4 "github.com/noris-network/cuegen/internal/app/v1alpha4"
	v1beta1 "github.com/noris-network/cuegen/internal/cuegen/v1beta1"
)

const (
	cuegenCue = "cuegen.cue"
	cueMod    = "cue.mod"
)

func main() {
	// argocd cmp check
	cmpPluginCheck()

	// apiVersion below v1alpha5?
	if !cuegenNgCheck() {
		os.Exit(v1alpha4.Main())
	}

	flag.Parse()
	path := "."
	if flag.NArg() == 1 {
		path = flag.Args()[0]
	}

	if os.Getenv("CUEGEN_DEBUG") == "true" {
		fmt.Printf("# cuegen apiVersion >= v1alpha5 detected\n")
	}

	if err := v1beta1.Exec(filepath.Join(".", cuegenCue), path); err != nil {
		log.Fatalln(err)
	}
}

var cuegenSchema = []byte(`cuegen: apiVersion: *"" | string`)

func cuegenNgCheck() bool {
	if !(hasCueModDir(".") && hasCuegenCue(".")) {
		return false
	}

	cfg := struct{ Cuegen struct{ ApiVersion string } }{}
	if err := cueconfig.Load(cuegenCue, cuegenSchema, nil, nil, &cfg); err != nil {
		log.Printf("pre-check: load %v: %v", cuegenCue, err)
		return false
	}

	switch cfg.Cuegen.ApiVersion {
	case "v1alpha5", "v1beta1":
		return true
	default:
		return false
	}
}

func hasCueModDir(path string) bool {
	finfo, err := os.Stat(filepath.Join(path, cueMod))
	return err == nil && finfo.IsDir()
}

func hasCuegenCue(path string) bool {
	finfo, err := os.Stat(filepath.Join(path, cuegenCue))
	return err == nil && finfo.Mode().IsRegular()
}

func cmpPluginCheck() {
	if !(len(os.Args) == 2 && os.Args[1] == "-is-cuegen-dir") {
		return
	}
	if _, err := os.Stat(cuegenCue); err == nil {
		fmt.Println(true)
	}
	os.Exit(0)
}
