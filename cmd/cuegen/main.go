package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	cuegen "github.com/noris-network/cuegen/internal/cuegen/v1alpha3"
	"github.com/nxcc/cueconfig"
)

const (
	cuegenCue  = "cuegen.cue"
	cuegenYaml = "cuegen.yaml"
)

//go:embed schema.cue
var configSchema []byte

var Build = "dev-build"

func main() {

	flagIsCuegenDir := false

	flag.BoolVar(&flagIsCuegenDir, "is-cuegen-dir", false,
		"check current working directory for cuegen.{yaml,cue} (for cmp detection)")

	flag.Parse()

	switch {

	// cmp detection
	case flagIsCuegenDir:
		isCuegenDir()
		os.Exit(0)

	// version
	case len(os.Args) == 2 && os.Args[1] == "version":
		printVersion()
		os.Exit(0)

	// no chdir
	case len(os.Args) == 1,
		len(os.Args) == 2 && (os.Args[1] == "." || os.Args[1] == "./"):

	// chdir
	case len(os.Args) == 2 && (strings.HasPrefix(os.Args[1], "./") || strings.HasPrefix(os.Args[1], "../") || strings.HasPrefix(os.Args[1], "/")):
		if err := os.Chdir(os.Args[1]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

	// fallback
	default:
		fallbackToV1Alpha1()
	}

	// read cuegen.cue
	config, err := configure()
	if err != nil {
		fmt.Printf("configure: %v\n", err)
		os.Exit(1)
	}
	if config.ApiVersion != cuegen.V1Alpha3 {
		fallbackToV1Alpha1()
	}

	// exec
	if err := cuegen.Exec(config); err != nil {
		fmt.Printf("exec: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func isCuegenDir() {
	if _, err := os.Stat(cuegenCue); err == nil {
		fmt.Println(true)
	}
	if _, err := os.Stat(cuegenYaml); err == nil {
		fmt.Println(true)
	}
}

func printVersion() {
	fmt.Printf("cuegen version %v\n", Build)
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.Fatalln("Failed to read build info")
	}
	for _, dep := range bi.Deps {
		if dep.Path == "cuelang.org/go" {
			fmt.Printf("cue version %v\n", dep.Version)
			break
		}
	}
}

func configure() (cuegen.Cuegen, error) {
	cfg := struct{ Cuegen cuegen.Cuegen }{}
	if err := cueconfig.Load(cuegenCue, configSchema, nil, nil, &cfg); err != nil {
		if os.IsNotExist(err) {
			return cuegen.Default, nil
		}
		return cuegen.Cuegen{}, err
	}
	return cfg.Cuegen, nil
}

func fallbackToV1Alpha1() {
	fmt.Println("@@@@@@@@@@@@ TODO: fallback to v1alpha1 @@@@@@@@@@@@")
	os.Exit(0)
}
