package zerolog

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ConfigureFunc is a function used to configure this CobraUtil
type ConfigureFunc = func(cu *CobraUtil)

// New creates a configuration that exposes RegisterZeroLogFlags and ZeroLogRunE
// to integrate with cobra
func New(configurations ...ConfigureFunc) *CobraUtil {
	cu := CobraUtil{
		preRunLevel: zerolog.InfoLevel,
	}
	for _, configure := range configurations {
		configure(&cu)
	}
	return &cu
}

// CobraUtil carries the configuration for a zerolog CobraRunFunc
type CobraUtil struct {
	flagPrefix        string
	target            func(zerolog.Logger)
	async             bool
	asyncSize         int
	asyncPollInterval time.Duration
	preRunLevel       zerolog.Level
}

// RegisterZeroLogFlags adds flags for use in with ZeroLogPreRunE:
// - "$PREFIX-level"
// - "$PREFIX-format"
func RegisterZeroLogFlags(flags *pflag.FlagSet, flagPrefix string) {
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "log"))
	flags.String(prefixed("level"), "info", `verbosity of logging ("trace", "debug", "info", "warn", "error")`)
	flags.String(prefixed("format"), "auto", `format of logs ("auto", "console", "json")`)
}

// RunE returns a Cobra run func that configures the corresponding
// log level from a command.
//
// The required flags can be added to a command by using
// RegisterLoggingPersistentFlags().
func RunE(flagPrefix string, prerunLevel zerolog.Level) cobrautil.CobraRunFunc {
	return New(WithFlagPrefix(flagPrefix), WithPreRunLevel(prerunLevel)).RunE()
}

// RegisterFlags adds flags for use in with ZeroLogPreRunE:
// - "$PREFIX-level"
// - "$PREFIX-format"
func (cu CobraUtil) RegisterFlags(flags *pflag.FlagSet) {
	RegisterZeroLogFlags(flags, cu.flagPrefix)
}

// RunE returns a Cobra run func that configures the corresponding
// log level from a command.
//
// The required flags can be added to a command by using
// RegisterLoggingPersistentFlags().
func (cu CobraUtil) RunE() cobrautil.CobraRunFunc {
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(cu.flagPrefix, "log"))
	return func(cmd *cobra.Command, args []string) error {
		if cobrautil.IsBuiltinCommand(cmd) {
			return nil // No-op for builtins
		}

		var output io.Writer

		format := cobrautil.MustGetString(cmd, prefixed("format"))
		if format == "console" || format == "auto" && isatty.IsTerminal(os.Stdout.Fd()) {
			output = zerolog.ConsoleWriter{Out: os.Stderr}
		} else {
			output = os.Stderr
		}

		if cu.async {
			output = diode.NewWriter(output, 1000, 10*time.Millisecond, func(missed int) {
				fmt.Printf("Logger Dropped %d messages", missed)
			})
		}

		l := zerolog.New(output).With().Timestamp().Logger()

		level := strings.ToLower(cobrautil.MustGetString(cmd, prefixed("level")))
		switch level {
		case "trace":
			l = l.Level(zerolog.TraceLevel)
		case "debug":
			l = l.Level(zerolog.DebugLevel)
		case "info":
			l = l.Level(zerolog.InfoLevel)
		case "warn":
			l = l.Level(zerolog.WarnLevel)
		case "error":
			l = l.Level(zerolog.ErrorLevel)
		case "fatal":
			l = l.Level(zerolog.FatalLevel)
		case "panic":
			l = l.Level(zerolog.PanicLevel)
		default:
			return fmt.Errorf("unknown log level: %s", level)
		}

		if cu.target != nil {
			cu.target(l)
		} else {
			log.Logger = l
		}

		l.WithLevel(cu.preRunLevel).
			Str("format", format).
			Str("log_level", level).
			Str("provider", "zerolog").
			Bool("async", cu.async).
			Msg("configured logging")
		return nil
	}
}

// WithFlagPrefix defines prefix used with the generated flags. Defaults to "log".
func WithFlagPrefix(flagPrefix string) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.flagPrefix = flagPrefix
	}
}

// WithPreRunLevel defines the logging level used for pre-run log messages. Debug by default.
func WithPreRunLevel(preRunLevel zerolog.Level) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.preRunLevel = preRunLevel
	}
}

// WithAsync enables non-blocking logging. Size of the buffer and polling interval can be configured.
// Disabled by default.
func WithAsync(size int, pollInterval time.Duration) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.async = true
		cu.asyncSize = size
		cu.asyncPollInterval = pollInterval
	}
}

// WithTarget callback that forwards the configured logger. Useful when we want to keep it in a global variable.
func WithTarget(fn func(zerolog.Logger)) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.target = fn
	}
}
