package cuepp_test

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"github.com/noris-network/cuegen/internal/cuepp"
)

func TestReadAttributes(t *testing.T) {

	// development-only
	writeOutputToFile := os.Getenv("WRITE_OUTPUT_TO_FILE") == "true"

	// new empty directory
	emptyDir, err := os.MkdirTemp("", "cuegen-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(emptyDir)

	// this key is just for testing, do not use it for anything else
	os.Setenv("SOPS_AGE_KEY", "AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ")

	p := cuepp.CuePP{
		Tempdir:     emptyDir,
		Debug:       true,
		SecretsPath: "secret",
	}

	paths, err := filepath.Glob(filepath.Join("testdata", "*", "*-source.cue"))
	if err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal("Getwd: ", err)
	}

	for _, path := range paths {
		testdir := filepath.Base(filepath.Dir(path))
		testname := filepath.Base(strings.TrimSuffix(path, "-source.cue"))
		t.Run(testdir+"-"+testname, func(t *testing.T) {

			// read cue source
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal("error reading source file:", err)
			}

			// process attributes
			dir := filepath.Join(cwd, filepath.Dir(path))
			output, err := p.Process(string(source), "", os.DirFS(dir))
			if err != nil {
				t.Fatal("Process: ", err)
			}

			// dump output
			if writeOutputToFile {
				outfile := filepath.Join(filepath.Dir(path), testname+"-output.cue")
				log.Println("write outfile", outfile)
				err := os.WriteFile(outfile, []byte(output), 0o644)
				if err != nil {
					panic(err)
				}
			}

			// read test
			testFile := filepath.Join(filepath.Dir(path), testname+"-test.cue")
			test, err := os.ReadFile(testFile)
			if err != nil {
				t.Logf("error: %v, skip test", err)
			} else {

				// prepare validation
				ctx := cuecontext.New()
				value := ctx.CompileString(output)
				if value.Err() != nil {
					t.Fatalf("output:\n%s\n", errors.Details(value.Err(), nil))
				}
				check := ctx.CompileBytes(test, cue.Scope(value))
				if check.Err() != nil {
					t.Fatalf("Schema: Compile Error:\n%s\n", errors.Details(check.Err(), nil))
				}

				// validate
				err = check.Validate()
				if err != nil {
					t.Fatalf("Validate Error:\n%s\n", errors.Details(err, nil))
				}
			}

			// read golden file
			goldenFile := filepath.Join(filepath.Dir(path), testname+".golden")
			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Logf("error: %v, skip test", err)
			} else {
				if !bytes.Equal([]byte(output), want) {
					t.Errorf("\n=== got:\n%s\n=== want:\n%s\n", output, want)
				}
			}

		})
	}
}
