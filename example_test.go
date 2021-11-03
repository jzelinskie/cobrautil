package cobrautil_test

import (
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/jzelinskie/cobrautil"
)

func ExampleCommandStack() {
	cmd := &cobra.Command{
		Use: "mycmd",
		PreRunE: cobrautil.CommandStack(
			cobrautil.SyncViperPreRunE("myprogram"),
			cobrautil.ZeroLogPreRunE("log", zerolog.InfoLevel),
		),
	}

	cobrautil.RegisterZeroLogFlags(cmd.PersistentFlags(), "log")
}

func ExampleRegisterZeroLogFlags() {
	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: cobrautil.ZeroLogPreRunE("log", zerolog.InfoLevel),
	}

	cobrautil.RegisterZeroLogFlags(cmd.PersistentFlags(), "log")
}

func ExampleZeroLogPreRunE() {
	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: cobrautil.ZeroLogPreRunE("log", zerolog.InfoLevel),
	}

	cobrautil.RegisterZeroLogFlags(cmd.PersistentFlags(), "log")
}
