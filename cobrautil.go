package cobrautil

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/joho/godotenv"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// IsBuiltinCommand checks against a hard-coded list of the names of commands
// that cobra provides out-of-the-box.
func IsBuiltinCommand(cmd *cobra.Command) bool {
	return stringz.SliceContains([]string{
		"help [command]",
		"completion [command]",
	},
		cmd.Use,
	)
}

// SyncEnvPreRunE returns a CobraRunFunc that synchronizes environment
// variables flags with the provided prefix.
func SyncEnvPreRunE(prefix string) CobraRunFunc {
	return func(cmd *cobra.Command, args []string) error {
		if IsBuiltinCommand(cmd) {
			return nil // No-op for builtins
		}

		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			varName := strings.ToUpper(strings.ReplaceAll(prefix+"_"+f.Name, "-", "_"))
			val, envIsSet := os.LookupEnv(varName)
			if !f.Changed && envIsSet {
				_ = cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
			}
		})

		return nil
	}
}

// SyncDotEnvPreRunE returns a CobraRunFunc that loads a .dotenv file
// before calling SyncDotEnvPreRunE.
//
// If empty, envfilePath defaults to ".env".
func SyncDotEnvPreRunE(prefix, envfilePath string, l logr.Logger) CobraRunFunc {
	if err := godotenv.Load(stringz.DefaultEmpty(envfilePath, ".env")); err != nil {
		l.V(2).Info(
			"skipped loading dotenv",
			"path", envfilePath,
			"err", err,
		)
	}
	return SyncEnvPreRunE(prefix)
}

// CobraRunFunc is the signature of cobra.Command RunFuncs.
type CobraRunFunc func(cmd *cobra.Command, args []string) error

// CommandStack chains together a collection of CobraCommandFuncs into one.
func CommandStack(cmdfns ...CobraRunFunc) CobraRunFunc {
	return func(cmd *cobra.Command, args []string) error {
		for _, cmdfn := range cmdfns {
			if err := cmdfn(cmd, args); err != nil {
				return err
			}
		}
		return nil
	}
}

// PrefixJoiner joins a list of strings with the "-" separator, including the provided prefix string
//
// example: PrefixJoiner("hi")("how", "are", "you") = "hi-how-are-you"
func PrefixJoiner(prefix string) func(...string) string {
	return func(xs ...string) string {
		return stringz.Join("-", append([]string{prefix}, xs...)...)
	}
}
