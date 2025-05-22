// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"fmt"
	"io"
	"os"
	"runtime"

	flag "github.com/spf13/pflag"
	apimachineryversion "k8s.io/apimachinery/pkg/version"
)

// These variables will be set during building the grove operator via LD_FLAGS
// These variables have been borrowed from k8s.io/component-base repository. We do not want
// the dependencies that k8s.io/component-base pulls in as the attempt is the keep a lean set of dependencies.
var (
	// programName is the name of the operator.
	programName = "etcd-druid"
	// gitVersion is the semantic version for grove operator.
	gitVersion = "v0.0.0-master+$Format:%H$"
	// gitCommit is the SHA1 from git, output of $(git rev-parse HEAD)
	gitCommit = "$Format:%H$"
	// gitTreeState is the state of git tree, either "clean" or "dirty"
	gitTreeState = ""
	// buildDate is the date (in ISO8601 format) at which the build was done. Output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	buildDate   = "1970-01-01T00:00:00Z"
	versionFlag bool
)

// AddFlags adds the --version flag to the flag.FlagSet.
func AddFlags(fs *flag.FlagSet) {
	fs.BoolVar(&versionFlag, "version", false, "--version prints the version information and quits")
}

// Get returns the version details for the grove operator.
func Get() apimachineryversion.Info {
	return apimachineryversion.Info{
		GitVersion:   gitVersion,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// PrintVersionAndExitIfRequested will check if --version is passed and if it is
// then it will print the version information and quit.
func PrintVersionAndExitIfRequested() {
	if versionFlag {
		_, _ = fmt.Fprintf(io.Writer(os.Stdout), "%s %v\n", programName, Get())
		os.Exit(0)
	}
}
