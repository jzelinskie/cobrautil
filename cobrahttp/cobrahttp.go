// Package cobrahttp implements a builder for registering flags and producing
// a Cobra RunFunc that configures an HTTP server.
package cobrahttp

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

// Option is function used to configure an HTTP server within a Cobra RunFunc.
type Option func(b *Builder)

// New creates a Cobra RunFunc Builder for an HTTP server.
func New(serviceName string, opts ...Option) *Builder {
	b := &Builder{
		serviceName:    stringz.DefaultEmpty(serviceName, "http"),
		preRunLevel:    0,
		logger:         logr.Discard(),
		defaultAddr:    ":8443",
		defaultEnabled: false,
		flagPrefix:     "http",
	}
	for _, configure := range opts {
		configure(b)
	}
	return b
}

// Builder is used to configure an HTTP server via Cobra.
type Builder struct {
	flagPrefix     string
	serviceName    string
	defaultAddr    string
	defaultEnabled bool
	logger         logr.Logger
	preRunLevel    int
	handler        http.Handler
}

func (b *Builder) prefix(s string) string {
	return cobrautil.PrefixJoiner(b.flagPrefix)(s)
}

// RegisterFlags adds flags for configuring an HTTP server.
//
// The following flags are added:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-enabled"
func (b *Builder) RegisterFlags(flags *pflag.FlagSet) {
	flags.String(b.prefix("addr"), b.defaultAddr, "address to listen on to serve "+b.serviceName)
	flags.String(b.prefix("tls-cert-path"), "", "local path to the TLS certificate used to serve "+b.serviceName)
	flags.String(b.prefix("tls-key-path"), "", "local path to the TLS key used to serve "+b.serviceName)
	flags.Bool(b.prefix("enabled"), b.defaultEnabled, "enable "+b.serviceName+" http server")
}

// ServerFromFlags creates an *http.Server as configured by the flags from
// RegisterFlags().
func (b *Builder) ServerFromFlags(cmd *cobra.Command) *http.Server {
	return &http.Server{
		Addr:    cobrautil.MustGetStringExpanded(cmd, b.prefix("addr")),
		Handler: b.handler,
	}
}

// ListenFromFlags listens on the provided HTTP server using values configured
// in the provided command.
func (b *Builder) ListenFromFlags(cmd *cobra.Command, srv *http.Server) error {
	if !cobrautil.MustGetBool(cmd, b.prefix("enabled")) {
		return nil
	}

	certPath := cobrautil.MustGetStringExpanded(cmd, b.prefix("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, b.prefix("tls-key-path"))

	switch {
	case certPath == "" && keyPath == "":
		b.logger.V(b.preRunLevel).Info(
			"http server started serving",
			"addr", srv.Addr,
			"prefix", b.flagPrefix,
			"scheme", "http",
			"insecure", "true",
		)
		if err := srv.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed while serving http: %w", err)
		}
		return nil

	case certPath != "" && keyPath != "":
		b.logger.V(b.preRunLevel).Info(
			"http server started serving",
			"addr", srv.Addr,
			"prefix", b.flagPrefix,
			"scheme", "https",
			"insecure", "false",
		)
		if err := srv.ListenAndServeTLS(certPath, keyPath); err != nil && errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed while serving https: %w", err)
		}
		return nil

	default:
		return fmt.Errorf(
			"failed to start http server: must provide both --%s-tls-cert-path and --%s-tls-key-path",
			b.flagPrefix,
			b.flagPrefix,
		)
	}
}

// WithLogger configures logging of the configured HTTP server environment.
func WithLogger(logger logr.Logger) Option {
	return func(b *Builder) { b.logger = logger }
}

// WithDefaultAddress configures the default value of the address the server
// will listen at.
//
// Defaults to ":8443"
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
// Defaults to "http".
func WithFlagPrefix(flagPrefix string) Option {
	return func(b *Builder) { b.flagPrefix = flagPrefix }
}

// WithPreRunLevel defines the logging level used for pre-run log messages.
//
// Defaults to "debug".
func WithPreRunLevel(preRunLevel int) Option {
	return func(b *Builder) { b.preRunLevel = preRunLevel }
}

// WithHandler defines the handler used by the http.Server.
//
// No handler is set by default.
func WithHandler(handler http.Handler) Option {
	return func(b *Builder) { b.handler = handler }
}
