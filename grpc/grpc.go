package grpc

import (
	"fmt"
	"net"
	"time"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

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

// ServerFromFlags creates an *grpc.Server as configured by the flags from
// RegisterGrpcServerFlags().
func ServerFromFlags(cmd *cobra.Command, flagPrefix string, opts ...grpc.ServerOption) (*grpc.Server, error) {
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "grpc"))

	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge: cobrautil.MustGetDuration(cmd, prefixed("max-conn-age")),
	}))

	certPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-key-path"))

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

// ListenFromFlags listens on an gRPC server using the configuration stored
// in the cobra command that was registered with RegisterGrpcServerFlags.
func ListenFromFlags(cmd *cobra.Command, flagPrefix string, srv *grpc.Server, level zerolog.Level) error {
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "grpc"))

	if !cobrautil.MustGetBool(cmd, prefixed("enabled")) {
		return nil
	}

	network := cobrautil.MustGetString(cmd, prefixed("network"))
	addr := cobrautil.MustGetStringExpanded(cmd, prefixed("addr"))

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
