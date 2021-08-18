package cobrautil

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// SyncViperPreRunE returns a Cobra run func that synchronizes Viper environment
// flags prefixed with the provided argument.
//
// Thanks to Carolyn Van Slyck: https://github.com/carolynvs/stingoftheviper
func SyncViperPreRunE(prefix string) func(cmd *cobra.Command, args []string) error {
	prefix = strings.ReplaceAll(strings.ToUpper(prefix), "-", "_")
	return func(cmd *cobra.Command, args []string) error {
		if cmd.Use == "help [command]" {
			return nil // No-op the help command
		}

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
	flags.String("log-level", "info", `verbosity of logging ("trace", "debug", "info", "warn", "error")`)
	flags.String("log-format", "auto", `format of logs ("auto", "human", "json")`)
}

// ZeroLogPreRunE reads the provided command's flags and configures the
// corresponding log level. The required flags can be added to a command by
// using RegisterLoggingPersistentFlags().
//
// This function exits with log.Fatal on failure.
func ZeroLogPreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Use == "help [command]" {
		return nil // No-op the help command
	}

	format := MustGetString(cmd, "log-format")
	if format == "human" || (format == "auto" && isatty.IsTerminal(os.Stdout.Fd())) {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}

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
	flags.String("otel-provider", "none", `opentelemetry provider for tracing ("none", "jaeger")`)
	flags.String("otel-jaeger-endpoint", "http://jaeger:14268/api/traces", "jaeger collector endpoint")
	flags.String("otel-jaeger-service-name", serviceName, "jaeger service name for trace data")
}

// TracingPreRun reads the provided command's flags and configures the
// corresponding tracing provider. The required flags can be added to a command
// by using RegisterTracingPersistentFlags().
func OpenTelemetryPreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Use == "help [command]" {
		return nil // No-op the help command
	}

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

// RegisterGrpcServerFlags adds the following flags for use with
// GrpcServerFromFlags:
// - "grpc-addr"
// - "grpc-no-tls"
// - "grpc-cert-path"
// - "grpc-key-path"
// - "grpc-max-conn-age"
func RegisterGrpcServerFlags(flags *pflag.FlagSet) {
	flags.String("grpc-addr", ":50051", "address to listen on for serving gRPC services")
	flags.String("grpc-cert-path", "", "local path to the TLS certificate used to serve gRPC services")
	flags.String("grpc-key-path", "", "local path to the TLS key used to serve gRPC services")
	flags.Bool("grpc-no-tls", false, "serve unencrypted gRPC services")
	flags.Duration("grpc-max-conn-age", 30*time.Second, "how long a connection should be able to live")
}

func GrpcServerFromFlags(cmd *cobra.Command, opts ...grpc.ServerOption) (*grpc.Server, error) {
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge: MustGetDuration(cmd, "grpc-max-conn-age"),
	}))

	if MustGetBool(cmd, "grpc-no-tls") {
		return grpc.NewServer(opts...), nil
	}

	certPath := MustGetStringExpanded(cmd, "grpc-cert-path")
	keyPath := MustGetStringExpanded(cmd, "grpc-key-path")
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("failed to start gRPC server: must provide either --grpc-no-tls or --grpc-cert-path and --grpc-key-path")
	}

	creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	opts = append(opts, grpc.Creds(creds))

	return grpc.NewServer(opts...), nil
}

func RegisterMetricsServerFlags(flags *pflag.FlagSet) {
	flags.String("metrics-addr", ":9090", "address on which to serve metrics and runtime profiles")
}

func MetricsServerFromFlags(cmd *cobra.Command) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:    MustGetString(cmd, "metrics-addr"),
		Handler: mux,
	}
}
