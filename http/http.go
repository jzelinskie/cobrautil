package http

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ConfigureFunc is a function used to configure this CobraUtil
type ConfigureFunc = func(cu *CobraUtil)

// New creates a configuration that exposes RegisterFlags and RunE
// to integrate with cobra
func New(serviceName string, configurations ...ConfigureFunc) *CobraUtil {
	cu := CobraUtil{
		serviceName:    stringz.DefaultEmpty(serviceName, "http"),
		preRunLevel:    1,
		logger:         logr.Discard(),
		defaultAddr:    ":8443",
		defaultEnabled: false,
		flagPrefix:     "http",
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
	handler        http.Handler
}

// RegisterHTTPServerFlags adds the following flags for use with
// HttpServerFromFlags:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-enabled"
func RegisterHTTPServerFlags(flags *pflag.FlagSet, flagPrefix, serviceName, defaultAddr string, defaultEnabled bool) {
	defaultAddr = stringz.DefaultEmpty(defaultAddr, ":8443")
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "http"))

	flags.String(prefixed("addr"), defaultAddr, "address to listen on to serve "+serviceName)
	flags.String(prefixed("tls-cert-path"), "", "local path to the TLS certificate used to serve "+serviceName)
	flags.String(prefixed("tls-key-path"), "", "local path to the TLS key used to serve "+serviceName)
	flags.Bool(prefixed("enabled"), defaultEnabled, "enable "+serviceName+" http server")
}

// ServerFromFlags creates an *http.Server as configured by the flags from
// RegisterHttpServerFlags().
func ServerFromFlags(cmd *cobra.Command, flagPrefix string) *http.Server {
	return New("", WithFlagPrefix(flagPrefix)).ServerFromFlags(cmd)
}

// ListenFromFlags listens on an HTTP server using the configuration stored
// in the cobra command that was registered with RegisterHttpServerFlags.
func ListenFromFlags(cmd *cobra.Command, flagPrefix string, srv *http.Server, preRunLevel int) error {
	return New("", WithFlagPrefix(flagPrefix), WithPreRunLevel(preRunLevel)).ListenWithServerFromFlags(cmd, srv)
}

// RegisterHTTPServerFlags adds the following flags for use with
// HttpServerFromFlags:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-enabled"
func (cu CobraUtil) RegisterHTTPServerFlags(flags *pflag.FlagSet) {
	RegisterHTTPServerFlags(flags, cu.flagPrefix, cu.serviceName, cu.defaultAddr, cu.defaultEnabled)
}

// ServerFromFlags creates an *http.Server as configured by the flags from
// RegisterHttpServerFlags().
func (cu CobraUtil) ServerFromFlags(cmd *cobra.Command) *http.Server {
	prefixed := cobrautil.PrefixJoiner(cu.flagPrefix)

	return &http.Server{
		Addr:    cobrautil.MustGetStringExpanded(cmd, prefixed("addr")),
		Handler: cu.handler,
	}
}

// ListenWithServerFromFlags listens on the provided HTTP server using the configuration stored
// in the cobra command that was registered with RegisterHttpServerFlags.
func (cu CobraUtil) ListenWithServerFromFlags(cmd *cobra.Command, srv *http.Server) error {
	prefixed := cobrautil.PrefixJoiner(cu.flagPrefix)
	if !cobrautil.MustGetBool(cmd, prefixed("enabled")) {
		return nil
	}

	certPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-key-path"))

	switch {
	case certPath == "" && keyPath == "":
		cu.logger.V(1).Info("http server serving plaintext", "addr", srv.Addr, "prefix", cu.flagPrefix)
		if err := srv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed while serving http: %w", err)
		}
		return nil

	case certPath != "" && keyPath != "":
		cu.logger.V(cu.preRunLevel).Info("https server started serving", "addr", srv.Addr, "prefix", cu.flagPrefix)
		if err := srv.ListenAndServeTLS(certPath, keyPath); err != nil && errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed while serving https: %w", err)
		}
		return nil

	default:
		return fmt.Errorf(
			"failed to start http server: must provide both --%s-tls-cert-path and --%s-tls-key-path",
			cu.flagPrefix,
			cu.flagPrefix,
		)
	}
}

// WithLogger defines the logger used to log messages in this package
func WithLogger(logger logr.Logger) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.logger = logger
	}
}

// WithDefaultAddress defines the default value of the address the server will listen at.
// Defaults to ":8443"
func WithDefaultAddress(addr string) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.defaultAddr = addr
	}
}

// WithDefaultEnabled defines whether the http server is enabled by default. Defaults to "false".
func WithDefaultEnabled(enabled bool) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.defaultEnabled = enabled
	}
}

// WithFlagPrefix defines prefix used with the generated flags. Defaults to "http".
func WithFlagPrefix(flagPrefix string) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.flagPrefix = flagPrefix
	}
}

// WithPreRunLevel defines the logging level used for pre-run log messages. Defaults to "debug"..
func WithPreRunLevel(preRunLevel int) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.preRunLevel = preRunLevel
	}
}

// WithHandler defines the HTTP server handler to inject in the http.Server in ServerFromFlags method.
// No handler is set by default. The value will be ignored in ListenFromFlags.
func WithHandler(handler http.Handler) ConfigureFunc {
	return func(cu *CobraUtil) {
		cu.handler = handler
	}
}
