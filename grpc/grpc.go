package grpc

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

// ConfigureFunc is a function used to configure this CobraUtil
type ConfigureFunc = func(cu *CobraUtil)

// New creates a configuration that exposes RegisterFlags and RunE
// to integrate with cobra
func New(serviceName string, configurations ...ConfigureFunc) *CobraUtil {
	cu := CobraUtil{
		serviceName:    stringz.DefaultEmpty(serviceName, "grpc"),
		preRunLevel:    1,
		logger:         logr.Discard(),
		defaultAddr:    ":50051",
		defaultEnabled: false,
		flagPrefix:     "grpc",
	}
	for _, configure := range configurations {
		configure(&cu)
	}
	return &cu
}

// CobraUtil carries the configuration for a otel CobraRunFunc
type CobraUtil struct {
	flagPrefix     string
	serviceName    string
	defaultAddr    string
	defaultEnabled bool
	logger         logr.Logger
	preRunLevel    int
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
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "grpc"))

	flags.String(prefixed("addr"), defaultAddr, "address to listen on to serve "+serviceName)
	flags.String(prefixed("network"), "tcp", "network type to serve "+serviceName+` ("tcp", "tcp4", "tcp6", "unix", "unixpacket")`)
	flags.String(prefixed("tls-cert-path"), "", "local path to the TLS certificate used to serve "+serviceName)
	flags.String(prefixed("tls-key-path"), "", "local path to the TLS key used to serve "+serviceName)
	flags.Duration(prefixed("max-conn-age"), 30*time.Second, "how long a connection serving "+serviceName+" should be able to live")
	flags.Bool(prefixed("enabled"), defaultEnabled, "enable "+serviceName+" gRPC server")
}

// RegisterGrpcServerFlags adds the following flags for use with
// GrpcServerFromFlags:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-max-conn-age"
func (cu CobraUtil) RegisterGrpcServerFlags(flags *pflag.FlagSet) {
	RegisterGrpcServerFlags(flags, cu.flagPrefix, cu.serviceName, cu.defaultAddr, cu.defaultEnabled)
}

// ServerFromFlags creates an *grpc.Server as configured by the flags from
// RegisterGrpcServerFlags().
func ServerFromFlags(cmd *cobra.Command, flagPrefix string, opts ...grpc.ServerOption) (*grpc.Server, error) {
	return New("", WithFlagPrefix(flagPrefix)).ServerFromFlags(cmd, opts...)
}

// ServerFromFlags creates an *grpc.Server as configured by the flags from
// RegisterGrpcServerFlags().
func (cu CobraUtil) ServerFromFlags(cmd *cobra.Command, opts ...grpc.ServerOption) (*grpc.Server, error) {
	prefixed := cobrautil.PrefixJoiner(cu.flagPrefix)

	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge: cobrautil.MustGetDuration(cmd, prefixed("max-conn-age")),
	}))

	certPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-key-path"))

	switch {
	case certPath == "" && keyPath == "":
		cu.logger.V(cu.preRunLevel).Info("grpc server serving plaintext", "prefix", cu.flagPrefix)
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
			cu.flagPrefix,
			cu.flagPrefix,
		)
	}
}

// ListenFromFlags listens on an gRPC server using the configuration stored
// in the cobra command that was registered with RegisterGrpcServerFlags.
func ListenFromFlags(cmd *cobra.Command, flagPrefix string, srv *grpc.Server, preRunLevel int) error {
	return New("", WithPreRunLevel(preRunLevel), WithFlagPrefix(flagPrefix)).ListenFromFlags(cmd, srv)
}

// ListenFromFlags listens on an gRPC server using the configuration stored
// in the cobra command that was registered with RegisterGrpcServerFlags.
func (cu CobraUtil) ListenFromFlags(cmd *cobra.Command, srv *grpc.Server) error {
	prefixed := cobrautil.PrefixJoiner(cu.flagPrefix)

	if !cobrautil.MustGetBool(cmd, prefixed("enabled")) {
		return nil
	}

	network := cobrautil.MustGetString(cmd, prefixed("network"))
	addr := cobrautil.MustGetStringExpanded(cmd, prefixed("addr"))

	l, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on addr for gRPC server: %w", err)
	}

	cu.logger.V(cu.preRunLevel).Info(
		"grpc server started listening",
		"addr", addr,
		"network", network,
		"prefix", cu.flagPrefix)

	if err := srv.Serve(l); err != nil {
		return fmt.Errorf("failed to serve gRPC: %w", err)
	}

	return nil
}

// WithLogger defines the logger used to log messages in this package
func WithLogger(logger logr.Logger) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.logger = logger
	}
}

// WithDefaultAddress defines the default value of the address the server will listen at.
// Defaults to ":50051"
func WithDefaultAddress(addr string) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.defaultAddr = addr
	}
}

// WithDefaultEnabled defines whether the gRPC server is enabled by default. Defaults to "false".
func WithDefaultEnabled(enabled bool) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.defaultEnabled = enabled
	}
}

// WithFlagPrefix defines prefix used with the generated flags. Defaults to "grpc".
func WithFlagPrefix(flagPrefix string) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.flagPrefix = flagPrefix
	}
}

// WithPreRunLevel defines the logging level used for pre-run log messages. Defaults to "debug".
func WithPreRunLevel(preRunLevel int) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.preRunLevel = preRunLevel
	}
}
