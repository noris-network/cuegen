// Copyright 2024 cuegen Authors
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

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
)

var (
	backupPrefix     = ".cuegen-backup-" + strconv.Itoa(os.Getpid()) + "~"
	restoreAfterRun  = map[string]string{}
	knownSopsFormats = map[string]formats.Format{
		".env":  formats.Dotenv,
		".json": formats.Json,
		".sops": formats.Binary,
		".yaml": formats.Yaml,
		".yml":  formats.Yaml,
	}
)

var wlog = slog.With("app", "wrapper")

func execWrapper() {
	dir, err := os.Getwd()
	if err != nil {
		wlog.Error("get wd", "err", err)
		os.Exit(1)
	}
	dir = filepath.Clean(dir)

	// decrypt sops files
	if err := decryptPath(dir); err != nil {
		wlog.Error("decrypt", "err", err)
		os.Exit(1)
	}

	// execute cuegen or other wrapped executable
	wexe := os.Getenv("CUEGEN_WRAPPED_EXECUTABLE")
	if wexe == "" {
		exe, err := os.Executable()
		if err != nil {
			wlog.Error("find executable", "err", err)
			os.Exit(1)
		}
		wexe = exe
	}
	cmd := exec.Command(wexe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = append(os.Environ(), "CUEGEN_SKIP_DECRYPT=true")
	if err := cmd.Run(); err != nil {
		wlog.Debug("exec", "exe", wexe, "err", err)
	}

	// restore original state
	if err := restorePath(); err != nil {
		wlog.Error("restore failed", "err", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func decryptPath(path string) error {
	slog.Debug("decrypt", "path", path)
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			} else {
				return err
			}
		}
		if !strings.Contains(path, ".sops.") && !strings.HasSuffix(path, ".sops") {
			return nil
		}
		if info.Name() == ".sops.yaml" {
			return nil
		}
		return decryptFile(path)
	})
}

func decryptFile(path string) error {
	slog.Debug("decrypt", "file", path)
	ext := filepath.Ext(path)
	if _, found := knownSopsFormats[ext]; !found {
		wlog.Warn("found unhandled extension", "ext", ext)
		return nil
	}

	nonSopsPath := toNonSopsPath(path)
	if nonSopsPath == "" {
		wlog.Warn("skip", "file", path)
		return nil
	}

	wlog.Debug("decrypt", "source", path, "target", nonSopsPath)
	cleartext, err := decrypt.File(path, ext)
	if err != nil {
		return fmt.Errorf("%v: can not open decrypt file: %v", path, err)
	}

	_, err = os.Stat(nonSopsPath)
	if err == nil {
		err := backupFile(nonSopsPath)
		if err != nil {
			return fmt.Errorf("%v: can not backup file: %v", path, err)
		}
	} else {
		restoreAfterRun[nonSopsPath] = ""
	}

	f, err := os.Create(nonSopsPath)
	if err != nil {
		return fmt.Errorf("%v: %v", path, err)
	}
	f.Write(cleartext)
	f.Close()
	return nil
}

func backupFile(path string) error {
	bak := filepath.Join(filepath.Dir(path), backupPrefix+filepath.Base(path))
	wlog.Debug("backup", "source", path, "target", bak)
	restoreAfterRun[path] = bak
	return os.Rename(path, bak)
}

func restorePath() error {
	for orig, bak := range restoreAfterRun {
		if bak == "" {
			wlog.Debug("remove", "file", orig)
			if err := os.Remove(orig); err != nil {
				return fmt.Errorf("cleanup: remove failed: %v: %v", orig, err)
			}
			continue
		}
		wlog.Debug("restore", "target", orig, "source", bak)
		if err := os.Rename(bak, orig); err != nil {
			return fmt.Errorf("cleanup: restore failed: %v: %v", orig, err)
		}
	}
	return nil
}

func toNonSopsPath(path string) string {
	if strings.HasSuffix(path, ".sops") {
		return strings.TrimSuffix(path, ".sops")
	}
	ext := filepath.Ext(path)
	path = strings.TrimSuffix(path, ext)
	sops := filepath.Ext(path)
	if sops != ".sops" {
		return ""
	}
	return strings.TrimSuffix(path, sops) + ext
}
