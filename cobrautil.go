package cobrautil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jzelinskie/stringz"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
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

// SyncViperPreRunE returns a Cobra run func that synchronizes Viper environment
// flags prefixed with the provided argument.
//
// Thanks to Carolyn Van Slyck: https://github.com/carolynvs/stingoftheviper
func SyncViperPreRunE(prefix string) func(cmd *cobra.Command, args []string) error {
	prefix = strings.ReplaceAll(strings.ToUpper(prefix), "-", "_")
	return func(cmd *cobra.Command, args []string) error {
		if IsBuiltinCommand(cmd) {
			return nil // No-op for builtins
		}

		v := viper.New()
		viper.SetEnvPrefix(prefix)

		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			suffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			_ = v.BindEnv(f.Name, prefix+"_"+suffix)

			if !f.Changed && v.IsSet(f.Name) {
				val := v.Get(f.Name)
				_ = cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
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

func prefixJoiner(prefix string) func(...string) string {
	return func(xs ...string) string {
		return stringz.Join("-", append([]string{prefix}, xs...)...)
	}
}

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
				opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
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
	res, _ := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(semconv.ServiceNameKey.String(serviceName)),
	)

	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	setTracePropagators(propagators)

	return nil
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

// RegisterGrpcServerFlags adds the following flags for use with
// GrpcServerFromFlags:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-max-conn-age"
func RegisterGrpcServerFlags(flags *pflag.FlagSet, flagPrefix, serviceName, defaultAddr string, defaultEnabled bool) {
	serviceName = stringz.DefaultEmpty(serviceName, "grpc")
	defaultAddr = stringz.DefaultEmpty(defaultAddr, ":50051")
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "grpc"))

	flags.String(prefixed("addr"), defaultAddr, "address to listen on to serve "+serviceName)
	flags.String(prefixed("network"), "tcp", "network type to serve "+serviceName+` ("tcp", "tcp4", "tcp6", "unix", "unixpacket")`)
	flags.String(prefixed("tls-cert-path"), "", "local path to the TLS certificate used to serve "+serviceName)
	flags.String(prefixed("tls-key-path"), "", "local path to the TLS key used to serve "+serviceName)
	flags.Duration(prefixed("max-conn-age"), 30*time.Second, "how long a connection serving "+serviceName+" should be able to live")
	flags.Bool(prefixed("enabled"), defaultEnabled, "enable "+serviceName+" gRPC server")
}

// GrpcServerFromFlags creates an *grpc.Server as configured by the flags from
// RegisterGrpcServerFlags().
func GrpcServerFromFlags(cmd *cobra.Command, flagPrefix string, opts ...grpc.ServerOption) (*grpc.Server, error) {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "grpc"))

	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge: MustGetDuration(cmd, prefixed("max-conn-age")),
	}))

	certPath := MustGetStringExpanded(cmd, prefixed("tls-cert-path"))
	keyPath := MustGetStringExpanded(cmd, prefixed("tls-key-path"))

	switch {
	case certPath == "" && keyPath == "":
		log.Warn().Str("prefix", flagPrefix).Msg("grpc server serving plaintext")
		return grpc.NewServer(opts...), nil

	case certPath != "" && keyPath != "":
		creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(creds))
		return grpc.NewServer(opts...), nil

	default:
		return nil, fmt.Errorf(
			"failed to start gRPC server: must provide both --%s-tls-cert-path and --%s-tls-key-path",
			flagPrefix,
			flagPrefix,
		)
	}
}

// GrpcListenFromFlags listens on an gRPC server using the configuration stored
// in the cobra command that was registered with RegisterGrpcServerFlags.
func GrpcListenFromFlags(cmd *cobra.Command, flagPrefix string, srv *grpc.Server, level zerolog.Level) error {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "grpc"))

	if !MustGetBool(cmd, prefixed("enabled")) {
		return nil
	}

	network := MustGetString(cmd, prefixed("network"))
	addr := MustGetStringExpanded(cmd, prefixed("addr"))

	l, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on addr for gRPC server: %w", err)
	}

	log.WithLevel(level).
		Str("addr", addr).
		Str("network", network).
		Str("prefix", flagPrefix).
		Msg("grpc server started listening")

	if err := srv.Serve(l); err != nil {
		return fmt.Errorf("failed to serve gRPC: %w", err)
	}

	return nil
}

// RegisterHTTPServerFlags adds the following flags for use with
// HttpServerFromFlags:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-enabled"
func RegisterHTTPServerFlags(flags *pflag.FlagSet, flagPrefix, serviceName, defaultAddr string, defaultEnabled bool) {
	serviceName = stringz.DefaultEmpty(serviceName, "http")
	defaultAddr = stringz.DefaultEmpty(defaultAddr, ":8443")
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "http"))

	flags.String(prefixed("addr"), defaultAddr, "address to listen on to serve "+serviceName)
	flags.String(prefixed("tls-cert-path"), "", "local path to the TLS certificate used to serve "+serviceName)
	flags.String(prefixed("tls-key-path"), "", "local path to the TLS key used to serve "+serviceName)
	flags.Bool(prefixed("enabled"), defaultEnabled, "enable "+serviceName+" http server")
}

// HTTPServerFromFlags creates an *http.Server as configured by the flags from
// RegisterHttpServerFlags().
func HTTPServerFromFlags(cmd *cobra.Command, flagPrefix string) *http.Server {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "http"))

	return &http.Server{
		Addr: MustGetStringExpanded(cmd, prefixed("addr")),
	}
}

// HTTPListenFromFlags listens on an HTTP server using the configuration stored
// in the cobra command that was registered with RegisterHttpServerFlags.
func HTTPListenFromFlags(cmd *cobra.Command, flagPrefix string, srv *http.Server, level zerolog.Level) error {
	prefixed := prefixJoiner(stringz.DefaultEmpty(flagPrefix, "http"))
	if !MustGetBool(cmd, prefixed("enabled")) {
		return nil
	}

	certPath := MustGetStringExpanded(cmd, prefixed("tls-cert-path"))
	keyPath := MustGetStringExpanded(cmd, prefixed("tls-key-path"))

	switch {
	case certPath == "" && keyPath == "":
		log.Warn().Str("addr", srv.Addr).Str("prefix", flagPrefix).Msg("http server serving plaintext")
		if err := srv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed while serving http: %w", err)
		}
		return nil

	case certPath != "" && keyPath != "":
		log.WithLevel(level).Str("addr", srv.Addr).Str("prefix", flagPrefix).Msg("https server started serving")
		if err := srv.ListenAndServeTLS(certPath, keyPath); err != nil && errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed while serving https: %w", err)
		}
		return nil

	default:
		return fmt.Errorf(
			"failed to start http server: must provide both --%s-tls-cert-path and --%s-tls-key-path",
			flagPrefix,
			flagPrefix,
		)
	}
}
