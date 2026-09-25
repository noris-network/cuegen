package main

import "runtime"

// legacyBinaryHashes maps "GOOS/GOARCH" to the SHA256 of the canonical
// cuegen_v0.16.8 binary as shipped in the v0.16.8 GitHub Release archives.
// These are the hashes of the extracted cuegen binary (not the archive) and
// were curated once by downloading each archive, extracting, and running
// sha256sum. v0.16.8 is an immutable tag, so these values never change.
//
// Platforms outside this map (linux/386, linux/armv6, windows/*) were part of
// the v0.16.8 release but are not in cuegen's current build matrix; operators
// on those platforms must set CUEGEN_LEGACY_SHA256 (see legacy_verify.go).
var legacyBinaryHashes = map[string]string{
	"linux/amd64":  "66cb1260bec2574e1fb861462356bd3013bd11f63b9bfaa4c0904614811aae5d",
	"linux/arm64":  "bce772eee3127e3213e16da29f94e21db8a9dff80f9a8f30e589d8d022270209",
	"darwin/amd64": "2897dcad5435ab3d5a85d9e726b80828c741453f1137b511084734f2ecada36d",
	"darwin/arm64": "b7e7f6ceebb3baaed632b52b6ffda1bf15c4a44fdda503ec3217d5a067455131",
}

// platformKey returns the "GOOS/GOARCH" key for the running build. Because
// runLegacy uses syscall.Exec, the on-PATH binary must match the host
// architecture, so the compile-time runtime constants are the right key.
func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }
