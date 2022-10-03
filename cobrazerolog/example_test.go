package cobrazerolog_test

import (
	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"

	"github.com/spf13/cobra"
)

func ExampleBuilder_RegisterFlags() {
	zl := cobrazerolog.New()

	cmd := &cobra.Command{
		Use:     "mycmd",
		PreRunE: zl.RunE(),
	}

	zl.RegisterFlags(cmd.PersistentFlags())
}
