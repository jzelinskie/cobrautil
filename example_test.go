package cobrautil_test

import (
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/jzelinskie/cobrautil/v2"
	zl "github.com/jzelinskie/cobrautil/v2/zerolog"
)

func ExampleCommandStack() {
	cmd := &cobra.Command{
		Use: "mycmd",
		PreRunE: cobrautil.CommandStack(
			cobrautil.SyncViperPreRunE("myprogram"),
			zl.ZeroLogRunE("log", zerolog.InfoLevel),
		),
	}

	zl.RegisterZeroLogFlags(cmd.PersistentFlags(), "log")
}

func ExampleRegisterZeroLogFlags() {
	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: zl.ZeroLogRunE("log", zerolog.InfoLevel),
	}

	zl.RegisterZeroLogFlags(cmd.PersistentFlags(), "log")
}

func ExampleZeroLogRunE() {
	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: zl.ZeroLogRunE("log", zerolog.InfoLevel),
	}

	zl.RegisterZeroLogFlags(cmd.PersistentFlags(), "log")
}
