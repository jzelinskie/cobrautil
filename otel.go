package cobrautil

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

// RegisterOpenTelemetryFlags adds the following flags for use with
// OpenTelemetryPreRunE:
// - "$PREFIX-provider"
// - "$PREFIX-endpoint"
// - "$PREFIX-service-name"
func RegisterOpenTelemetryFlags(flags *pflag.FlagSet, flagPrefix, serviceName string) {
	bi, _ := debug.ReadBuildInfo()
	serviceName = stringz.DefaultEmpty(serviceName, bi.Main.Path)
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "otel"))

	flags.String(prefixed("provider"), "none", `OpenTelemetry provider for tracing ("none", "jaeger, otlphttp", "otlpgrpc")`)
	flags.String(prefixed("endpoint"), "", "OpenTelemetry collector endpoint - the endpoint can also be set by using enviroment variables")
	flags.String(prefixed("service-name"), serviceName, "service name for trace data")
	flags.String(prefixed("trace-propagator"), "w3c", `OpenTelemetry trace propagation format ("b3", "w3c", "ottrace"). Add multiple propagators separated by comma.`)

	// Legacy flags! Will eventually be dropped!
	flags.String("otel-jaeger-endpoint", "", "OpenTelemetry collector endpoint - the endpoint can also be set by using enviroment variables")
	if err := flags.MarkHidden("otel-jaeger-endpoint"); err != nil {
		panic("failed to mark flag hidden: " + err.Error())
	}
	flags.String("otel-jaeger-service-name", serviceName, "service name for trace data")
	if err := flags.MarkHidden("otel-jaeger-service-name"); err != nil {
		panic("failed to mark flag hidden: " + err.Error())
	}
}

// OpenTelemetryRunE returns a Cobra run func that configures the
// corresponding otel provider from a command.
//
// The required flags can be added to a command by using
// RegisterOpenTelemetryFlags().
func OpenTelemetryRunE(flagPrefix string, prerunLevel zerolog.Level) CobraRunFunc {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "otel"))
	return func(cmd *cobra.Command, args []string) error {
		if IsBuiltinCommand(cmd) {
			return nil // No-op for builtins
		}

		provider := strings.ToLower(MustGetString(cmd, prefixed("provider")))
		serviceName := MustGetString(cmd, prefixed("service-name"))
		endpoint := MustGetString(cmd, prefixed("endpoint"))
		propagators := strings.Split(MustGetString(cmd, prefixed("trace-propagator")), ",")

		var exporter trace.SpanExporter
		var err error

		// If endpoint is not set, the clients are configured via the OpenTelemetry environment variables or
		// default values.
		// See: https://github.com/open-telemetry/opentelemetry-go/tree/main/exporters/otlp/otlptrace#environment-variables
		// or https://github.com/open-telemetry/opentelemetry-go/tree/main/exporters/jaeger#environment-variables
		switch provider {
		case "none":
			// Nothing.
		case "jaeger":
			// Legacy flags! Will eventually be dropped!
			endpoint = stringz.DefaultEmpty(endpoint, MustGetString(cmd, "otel-jaeger-endpoint"))
			serviceName = stringz.Default(serviceName, MustGetString(cmd, "otel-jaeger-service-name"), "", cmd.Flags().Lookup(prefixed("service-name")).DefValue)

			var opts []jaeger.CollectorEndpointOption
			if endpoint != "" {
				opts = append(opts, jaeger.WithEndpoint(endpoint))
			}

			exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(opts...))
			if err != nil {
				return err
			}
			return initOtelTracer(exporter, serviceName, propagators)
		case "otlphttp":
			var opts []otlptracehttp.Option
			if endpoint != "" {
				opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
			}

			exporter, err = otlptrace.New(context.Background(), otlptracehttp.NewClient(opts...))
			if err != nil {
				return err
			}
			return initOtelTracer(exporter, serviceName, propagators)
		case "otlpgrpc":
			var opts []otlptracegrpc.Option
			if endpoint != "" {
				opts = append(opts, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
			}

			exporter, err = otlptrace.New(context.Background(), otlptracegrpc.NewClient(opts...))
			if err != nil {
				return err
			}
			return initOtelTracer(exporter, serviceName, propagators)
		default:
			return fmt.Errorf("unknown tracing provider: %s", provider)
		}

		log.WithLevel(prerunLevel).Str("new provider", provider).Msg("set tracing provider")
		return nil
	}
}

func initOtelTracer(exporter trace.SpanExporter, serviceName string, propagators []string) error {
	res, err := setResource(serviceName)
	if err != nil {
		return err
	}

	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	setTracePropagators(propagators)

	return nil
}

func setResource(serviceName string) (*resource.Resource, error) {
	return resource.New(
		context.Background(),
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
}

// setTextMapPropagator sets the OpenTelemetry trace propagation format.
// Currently it supports b3, ot-trace and w3c.
func setTracePropagators(propagators []string) {
	var tmPropagators []propagation.TextMapPropagator

	for _, p := range propagators {
		switch p {
		case "b3":
			tmPropagators = append(tmPropagators, b3.New())
		case "ottrace":
			tmPropagators = append(tmPropagators, ot.OT{})
		case "w3c":
			fallthrough
		default:
			tmPropagators = append(tmPropagators, propagation.Baggage{})      // W3C baggage support
			tmPropagators = append(tmPropagators, propagation.TraceContext{}) // W3C for compatibility with other tracing system
		}
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(tmPropagators...))
}
