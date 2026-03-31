package cobrautil_test

import (
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/spf13/cobra"
)

func ExampleCommandStack() {
	_ = &cobra.Command{
		Use: "mycmd",
		RunE: cobrautil.CommandStack(
			cobrautil.SyncViperPreRunE("myprogram"),
			func(cmd *cobra.Command, args []string) error {
				return nil
			},
		),
	}
}
