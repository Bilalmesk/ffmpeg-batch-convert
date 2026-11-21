// Package version exposes build metadata injected at link time.
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// These values are overridden by -ldflags in release builds.
var (
	Version = "0.2.1-dev"
	Commit  = "none"
	Date    = "unknown"
)

// Short returns the semantic version without any build metadata.
func Short() string {
	if i := strings.IndexByte(Version, '+'); i >= 0 {
		return Version[:i]
	}
	return Version
}

// Full returns a single-line description of the build.
func Full() string {
	return fmt.Sprintf("ffbatch %s (%s, %s) %s/%s %s",
		Version, shortCommit(), Date, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func shortCommit() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}
	if Commit == "" {
		return "none"
	}
	return Commit
}
