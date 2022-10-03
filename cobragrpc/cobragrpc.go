// Package cobragrpc implements a builder for registering flags and producing
// a Cobra RunFunc that configures a gRPC server.
package cobragrpc

import (
	"fmt"
	"net"
	"time"

	"github.com/jzelinskie/cobrautil/v2"

	"github.com/go-logr/logr"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Option is function used to configure a gRPC server within a Cobra RunFunc.
type Option func(*Builder)

// New creates a Cobra RunFunc Builder for a gRPC server.
func New(serviceName string, opts ...Option) *Builder {
	b := &Builder{
		serviceName:    stringz.DefaultEmpty(serviceName, "grpc"),
		preRunLevel:    0,
		logger:         logr.Discard(),
		defaultAddr:    ":50051",
		defaultEnabled: false,
		flagPrefix:     "grpc",
	}
	for _, configure := range opts {
		configure(b)
	}
	return b
}

// Builder is used to configure a gRPC server via Cobra.
type Builder struct {
	flagPrefix     string
	serviceName    string
	defaultAddr    string
	defaultEnabled bool
	logger         logr.Logger
	preRunLevel    int
}

func (b *Builder) prefix(s string) string {
	return cobrautil.PrefixJoiner(b.flagPrefix)(s)
}

// RegisterFlags adds flags for configuring a gRPC server.
//
// The following flags are added:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-max-conn-age"
func (b *Builder) RegisterFlags(flags *pflag.FlagSet) {
	flags.String(b.prefix("addr"), b.defaultAddr, "address to listen on to serve "+b.serviceName)
	flags.String(b.prefix("network"), "tcp", "network type to serve "+b.serviceName+` ("tcp", "tcp4", "tcp6", "unix", "unixpacket")`)
	flags.String(b.prefix("tls-cert-path"), "", "local path to the TLS certificate used to serve "+b.serviceName)
	flags.String(b.prefix("tls-key-path"), "", "local path to the TLS key used to serve "+b.serviceName)
	flags.Duration(b.prefix("max-conn-age"), 30*time.Second, "how long a connection serving "+b.serviceName+" should be able to live")
	flags.Bool(b.prefix("enabled"), b.defaultEnabled, "enable "+b.serviceName+" gRPC server")
}

// ServerFromFlags creates an *grpc.Server as configured by the flags from
// RegisterFlags().
func (b *Builder) ServerFromFlags(cmd *cobra.Command, opts ...grpc.ServerOption) (*grpc.Server, error) {
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge: cobrautil.MustGetDuration(cmd, b.prefix("max-conn-age")),
	}))

	certPath := cobrautil.MustGetStringExpanded(cmd, b.prefix("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, b.prefix("tls-key-path"))

	switch {
	case isInsecure(certPath, keyPath):
		return grpc.NewServer(opts...), nil

	case isSecure(certPath, keyPath):
		creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(creds))
		return grpc.NewServer(opts...), nil

	default:
		return nil, fmt.Errorf(
			"failed to start gRPC server: must provide both --%s-tls-cert-path and --%s-tls-key-path",
			b.flagPrefix,
			b.flagPrefix,
		)
	}
}

// ListenFromFlags listens on the provided gRPC server using values configured
// in the provided command.
func (b *Builder) ListenFromFlags(cmd *cobra.Command, srv *grpc.Server) error {
	if !cobrautil.MustGetBool(cmd, b.prefix("enabled")) {
		return nil
	}

	network := cobrautil.MustGetString(cmd, b.prefix("network"))
	addr := cobrautil.MustGetStringExpanded(cmd, b.prefix("addr"))

	l, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on addr for gRPC server: %w", err)
	}

	certPath := cobrautil.MustGetStringExpanded(cmd, b.prefix("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, b.prefix("tls-key-path"))
	b.logger.V(b.preRunLevel).Info(
		"grpc server started listening",
		"addr", addr,
		"network", network,
		"prefix", b.flagPrefix,
		"insecure", isInsecure(certPath, keyPath),
	)

	if err := srv.Serve(l); err != nil {
		return fmt.Errorf("failed to serve gRPC: %w", err)
	}

	return nil
}

func isInsecure(certPath, keyPath string) bool {
	return certPath == "" && keyPath == ""
}

func isSecure(certPath, keyPath string) bool {
	return certPath != "" && keyPath != ""
}

// WithLogger configures logging of the configured gRPC server environment.
func WithLogger(logger logr.Logger) Option {
	return func(b *Builder) { b.logger = logger }
}

// WithDefaultAddress configures the default value of the address the server
// will listen at.
//
// Defaults to ":50051"
func WithDefaultAddress(addr string) Option {
	return func(b *Builder) { b.defaultAddr = addr }
}

// WithDefaultEnabled defines whether the server is enabled by default.
//
// Defaults to "false".
func WithDefaultEnabled(enabled bool) Option {
	return func(b *Builder) { b.defaultEnabled = enabled }
}

// WithFlagPrefix defines prefix used with the generated flags.
//
// Defaults to "grpc".
func WithFlagPrefix(flagPrefix string) Option {
	return func(b *Builder) { b.flagPrefix = flagPrefix }
}

// WithPreRunLevel defines the logging level used for pre-run log messages.
//
// Defaults to "debug".
func WithPreRunLevel(preRunLevel int) Option {
	return func(b *Builder) { b.preRunLevel = preRunLevel }
}
