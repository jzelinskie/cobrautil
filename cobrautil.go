package cobrautil

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// SyncViperPreRunE returns a Cobra run func that synchronizes Viper environment
// flags prefixed with the provided argument.
//
// Thanks to Carolyn Van Slyck: https://github.com/carolynvs/stingoftheviper
func SyncViperPreRunE(prefix string) func(cmd *cobra.Command, args []string) error {
	prefix = strings.ReplaceAll(strings.ToUpper(prefix), "-", "_")
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
		return fmt.Errorf("unknown log level: %s", level)
	}

	log.Info().Str("new level", level).Msg("set log level")
	return nil
}

// RegisterOpenTelemetryFlags adds the following flags for use with
// OpenTelemetryPreRunE:
// - "otel-provider"
// - "otel-jaeger-endpoint"
// - "otel-jaeger-service-name"
func RegisterOpenTelemetryFlags(flags *pflag.FlagSet, serviceName string) {
	flags.String("otel-provider", "none", "opentelemetry provider for tracing (none, jaeger)")
	flags.String("otel-jaeger-endpoint", "http://jaeger:14268/api/traces", "jaeger collector endpoint")
	flags.String("otel-jaeger-service-name", serviceName, "jaeger service name for trace data")
}

// TracingPreRun reads the provided command's flags and configures the
// corresponding tracing provider. The required flags can be added to a command
// by using RegisterTracingPersistentFlags().
func OpenTelemetryPreRunE(cmd *cobra.Command, args []string) error {
	provider := strings.ToLower(MustGetString(cmd, "otel-provider"))
	switch provider {
	case "none":
		// Nothing.
	case "jaeger":
		return initJaegerTracer(
			MustGetString(cmd, "otel-jaeger-endpoint"),
			MustGetString(cmd, "otel-jaeger-service-name"),
		)
	default:
		return fmt.Errorf("unknown tracing provider: %s", provider)
	}

	log.Info().Str("new provider", provider).Msg("set tracing provider")
	return nil
}

func initJaegerTracer(endpoint, serviceName string) error {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)))
	if err != nil {
		return err
	}

	// Configure the global tracer as a batched, always sampling Jaeger exporter.
	otel.SetTracerProvider(trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewBatchSpanProcessor(exp)),
		trace.WithResource(resource.NewSchemaless(semconv.ServiceNameKey.String(serviceName))),
	))

	// Configure the global tracer to use the W3C method for propagating contexts
	// across services.
	//
	// For low-level details see:
	// https://www.w3.org/TR/trace-context/
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return nil
}
