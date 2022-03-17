package cobrautil

import (
	"fmt"
	"os"
	"strings"

	"github.com/jzelinskie/stringz"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RegisterZeroLogFlags adds flags for use in with ZeroLogPreRunE:
// - "$PREFIX-level"
// - "$PREFIX-format"
func RegisterZeroLogFlags(flags *pflag.FlagSet, flagPrefix string) {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "log"))
	flags.String(prefixed("level"), "info", `verbosity of logging ("trace", "debug", "info", "warn", "error")`)
	flags.String(prefixed("format"), "auto", `format of logs ("auto", "console", "json")`)
}

// ZeroLogRunE returns a Cobra run func that configures the corresponding
// log level from a command.
//
// The required flags can be added to a command by using
// RegisterLoggingPersistentFlags().
func ZeroLogRunE(flagPrefix string, prerunLevel zerolog.Level) CobraRunFunc {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "log"))
	return func(cmd *cobra.Command, args []string) error {
		if IsBuiltinCommand(cmd) {
			return nil // No-op for builtins
		}

		format := MustGetString(cmd, prefixed("format"))
		if format == "console" || format == "auto" && isatty.IsTerminal(os.Stdout.Fd()) {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		}

		level := strings.ToLower(MustGetString(cmd, prefixed("level")))
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
			return fmt.Errorf("unknown log level: %s", level)
		}

		log.WithLevel(prerunLevel).Str("new level", level).Msg("set log level")

		return nil
	}
}
