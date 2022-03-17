package cobrautil

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Version is variable that holds program's version string.
// This should be set with the follow flags to the `go build` command:
// -ldflags '-X github.com/jzelinskie/cobrautil.Version=$YOUR_VERSION_HERE'
var Version string

// UsageVersion introspects the process debug data for Go modules to return a
// version string.
func UsageVersion(programName string, includeDeps bool) string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		panic("failed to read BuildInfo because the program was compiled with Go " + runtime.Version())
	}

	if Version == "" {
		// The version wasn't set by ldflags, so fallback to the Go module version.
		// Although, this value is pretty much guaranteed to just be "(devel)".
		Version = bi.Main.Version
	}

	if !includeDeps {
		if Version == "(devel)" {
			return fmt.Sprintf("%s development build (unknown exact version)", programName)
		}
		return fmt.Sprintf("%s %s", programName, Version)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", bi.Path, Version)
	for _, dep := range bi.Deps {
		fmt.Fprintf(&b, "\n\t%s %s", dep.Path, dep.Version)
	}
	return b.String()
}

// RegisterVersionFlags registers the flags used for the VersionRunFunc.
func RegisterVersionFlags(flags *pflag.FlagSet) {
	flags.Bool("include-deps", false, "include dependencies' versions")
}

// VersionRunFunc provides a generic implementation of a version command that
// reads its values from ldflags and the internal Go module data stored in a
// binary.
func VersionRunFunc(programName string) CobraRunFunc {
	return func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Println(UsageVersion(programName, MustGetBool(cmd, "include-deps")))
		return err
	}
}
