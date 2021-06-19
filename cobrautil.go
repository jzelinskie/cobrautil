package cobrautil

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// SyncViperPreRunE returns a Cobra run func that synchronizes Viper environment
// flags prefixed with the provided argument.
//
// Thanks to Carolyn Van Slyck: https://github.com/carolynvs/stingoftheviper
func SyncViperPreRunE(prefix string) func(cmd *cobra.Command, args []string) error {
	prefix = strings.ToUpper(prefix)
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

// CobraRunFunc is the signature of cobra.Command RunFuncs.
type CobraRunFunc func(cmd *cobra.Command, args []string) error

// RunFuncStack chains together a collection of CobraCommandFuncs into one.
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

// RegisterZeroLogFlags adds a "log-level" flag for use in with ZeroLogPreRunE.
func RegisterZeroLogFlags(flags *pflag.FlagSet) {
	flags.String("log-level", "info", "verbosity of logging (trace, debug, info, warn, error, fatal, panic)")
}

// ZeroLogPreRunE reads the provided command's flags and configures the
// corresponding log level. The required flags can be added to a command by
// using RegisterLoggingPersistentFlags().
//
// This function exits with log.Fatal on failure.
func ZeroLogPreRunE(cmd *cobra.Command, args []string) error {
	level := strings.ToLower(MustGetString(cmd, "log-level"))
	switch level {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	default:
		log.Fatal().Str("level", level).Msg("unknown log level")
	}

	log.Info().Str("new level", level).Msg("set log level")
	return nil
}
