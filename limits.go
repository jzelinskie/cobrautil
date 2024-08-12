package cobrautil

import (
	"log/slog"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"
)

// SetLimitsRunE wraps the RunFunc with setup logic for memory limits and maxprocs
// limits for the go process.
// NOTE: this assumes that there is already a zerolog instance configured for the process
// by the time this RunE is invoked.
func SetLimitsRunE() CobraRunFunc {
	return func(cmd *cobra.Command, args []string) error {
		// Need to invert the slog => zerolog map so that we can get the correct
		// slog loglevel for memlimit logging
		logLevelMap := make(map[zerolog.Level]slog.Level, len(slogzerolog.LogLevels))
		for sLevel, zLevel := range slogzerolog.LogLevels {
			logLevelMap[zLevel] = sLevel
		}

		logger := zerolog.DefaultContextLogger

		logLevel := logLevelMap[logger.GetLevel()]

		slogger := slog.New(slogzerolog.Option{Level: logLevel, Logger: logger}.NewZerologHandler())

		undo, err := maxprocs.Set(maxprocs.Logger(zerolog.DefaultContextLogger.Printf))
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to set maxprocs")
		}
		defer undo()

		_, _ = memlimit.SetGoMemLimitWithOpts(
			memlimit.WithRatio(0.9),
			memlimit.WithProvider(
				memlimit.ApplyFallback(
					memlimit.FromCgroup,
					memlimit.FromSystem,
				),
			),
			memlimit.WithLogger(slogger),
		)

		return nil
	}
}
