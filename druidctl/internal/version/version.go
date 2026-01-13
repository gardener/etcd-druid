// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"fmt"
	"runtime"
	"strings"
)

// These variables are set via -ldflags during build
// Default values are used when building without ldflags (go install, go run, etc.)
var (
	// gitVersion is the semantic version (e.g., v0.0.1 or v0.0.1-dev)
	// For dev builds: includes commit hash (e.g., v0.0.1-dev+abc1234)
	// For releases: clean version (e.g., v0.0.1)
	gitVersion = "v0.0.0-dev+unknown"

	// gitCommit is the full git commit SHA
	gitCommit = "unknown"

	// gitTreeState is "clean" if no uncommitted changes, "dirty" otherwise
	gitTreeState = "unknown"

	// buildDate in ISO8601 format (YYYY-MM-DDTHH:MM:SSZ)
	buildDate = "unknown"
)

// Info contains versioning information
type Info struct {
	GitVersion   string `json:"gitVersion" yaml:"gitVersion"`
	GitCommit    string `json:"gitCommit" yaml:"gitCommit"`
	GitTreeState string `json:"gitTreeState" yaml:"gitTreeState"`
	BuildDate    string `json:"buildDate" yaml:"buildDate"`
	GoVersion    string `json:"goVersion" yaml:"goVersion"`
	Compiler     string `json:"compiler" yaml:"compiler"`
	Platform     string `json:"platform" yaml:"platform"`
}

// Get returns the overall version information
func Get() Info {
	return Info{
		GitVersion:   gitVersion,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a human-readable version string
// For dev builds: v0.0.1-dev+abc1234
// For releases: v0.0.1
func (i Info) String() string {
	return i.GitVersion
}

// IsRelease returns true if this is a release build (no -dev suffix)
func (i Info) IsRelease() bool {
	return !strings.Contains(i.GitVersion, "-dev")
}

// IsDirty returns true if the git tree had uncommitted changes
func (i Info) IsDirty() bool {
	return i.GitTreeState == "dirty"
}
