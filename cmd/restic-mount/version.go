package main

import (
	"fmt"
	"runtime"
)

var (
	// Version represents the release version of restic-mount.
	// Can be injected at build time via -ldflags "-X main.Version=x.y.z".
	Version = "0.1.0"

	// ResticCommit represents the commit hash of the upstream restic submodule.
	// Can be injected at build time via -ldflags "-X main.ResticCommit=...".
	ResticCommit = "ba802d42b"
)

// formatVersion returns a formatted string detailing the version, upstream restic commit, and Go runtime environment.
func formatVersion() string {
	return fmt.Sprintf("restic-mount %s (based on restic %s, %s, %s/%s)",
		Version, ResticCommit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
