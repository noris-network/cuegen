package main

import "runtime"

// legacyBinaryHashes maps "GOOS/GOARCH" to the SHA256 of the canonical
// cuegen_v0.16.9 binary as shipped in the v0.16.9 GitHub Release archives.
// These are the hashes of the extracted cuegen binary (not the archive) and
// were curated once by downloading each archive, extracting, and running
// sha256sum. v0.16.9 is an immutable tag, so these values never change.
//
// Platforms outside this map (linux/386, linux/armv6, windows/*) were part of
// the v0.16.9 release but are not in cuegen's current build matrix; operators
// on those platforms must set CUEGEN_LEGACY_SHA256 (see legacy_verify.go).
var legacyBinaryHashes = map[string]string{
	"linux/amd64":  "56219e128d6feac92a212d3bcb35aa8df26c8168eb3b620bd9142714264bf8ef",
	"linux/arm64":  "aa233a204a54d758497cdcc4be6a55bc12b5dc553066f5ef288d888a18c321b3",
	"darwin/amd64": "22671d8fae25fa846d2ed3c77754fd9fc59728469549f497ccb3415a03ba000c",
	"darwin/arm64": "3382453016a631e144f2d610ebcdae0c1b385d8cdbddab383b90533c3b701b39",
}

// platformKey returns the "GOOS/GOARCH" key for the running build. Because
// runLegacy uses syscall.Exec, the on-PATH binary must match the host
// architecture, so the compile-time runtime constants are the right key.
func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }
