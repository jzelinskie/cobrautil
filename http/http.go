package http

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RegisterHTTPServerFlags adds the following flags for use with
// HttpServerFromFlags:
// - "$PREFIX-addr"
// - "$PREFIX-tls-cert-path"
// - "$PREFIX-tls-key-path"
// - "$PREFIX-enabled"
func RegisterHTTPServerFlags(flags *pflag.FlagSet, flagPrefix, serviceName, defaultAddr string, defaultEnabled bool) {
	serviceName = stringz.DefaultEmpty(serviceName, "http")
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
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "http"))

	return &http.Server{
		Addr: cobrautil.MustGetStringExpanded(cmd, prefixed("addr")),
	}
}

// ListenFromFlags listens on an HTTP server using the configuration stored
// in the cobra command that was registered with RegisterHttpServerFlags.
func ListenFromFlags(cmd *cobra.Command, flagPrefix string, srv *http.Server, level zerolog.Level) error {
	prefixed := cobrautil.PrefixJoiner(stringz.DefaultEmpty(flagPrefix, "http"))
	if !cobrautil.MustGetBool(cmd, prefixed("enabled")) {
		return nil
	}

	certPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-cert-path"))
	keyPath := cobrautil.MustGetStringExpanded(cmd, prefixed("tls-key-path"))

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
