package cobrautil_test

import (
	"github.com/spf13/cobra"

	"github.com/jzelinskie/cobrautil"
)

func ExampleCommandStack() {
	cmd := &cobra.Command{
		Use: "mycmd",
		PreRunE: cobrautil.CommandStack(
			cobrautil.SyncViperPreRunE("myprogram"),
			cobrautil.ZeroLogPreRunE,
		),
	}

	cobrautil.RegisterZeroLogFlags(cmd.PersistentFlags())
}

func ExampleRegisterZeroLogFlags() {
	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: cobrautil.ZeroLogPreRunE,
	}

	cobrautil.RegisterZeroLogFlags(cmd.PersistentFlags())
}

func ExampleZeroLogPreRunE() {
	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: cobrautil.ZeroLogPreRunE,
	}

	cobrautil.RegisterZeroLogFlags(cmd.PersistentFlags())
}
