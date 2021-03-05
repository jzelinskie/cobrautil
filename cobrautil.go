package cobrautil

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// SyncViperPreRunE returns a Cobra run func that synchronizes Viper environment
// flags prefixed with the provided argument.
func SyncViperPreRunE(prefix string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		v := viper.New()
		viper.SetEnvPrefix(prefix)

		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			suffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			v.BindEnv(f.Name, prefix+"_"+suffix)

			if !f.Changed && v.IsSet(f.Name) {
				val := v.Get(f.Name)
				cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
			}
		})

		return nil
	}
}

// MustGetString reads a string value out of a Cobra flag and panics if that key
// was not present.
func MustGetString(cmd *cobra.Command, key string) string {
	value, err := cmd.Flags().GetString(key)
	if err != nil {
		panic("failed to find cobra flag: " + key)
	}
	return value
}

// MustGetExpandedString calls MustGetString and expands any environment
// variables present in the string.
func MustGetStringExpanded(cmd *cobra.Command, key string) string {
	return os.ExpandEnv(MustGetString(cmd, key))
}

// MustGetBool reads a boolean value out of a Cobra flag and panics if that key
// was not present.
func MustGetBool(cmd *cobra.Command, key string) bool {
	value, err := cmd.Flags().GetBool(key)
	if err != nil {
		panic("failed to find cobra flag: " + key)
	}
	return value
}
