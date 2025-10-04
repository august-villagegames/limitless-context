package cmd

import (
	"flag"
	"fmt"
	"io"
)

func newVersionCommand() command {
	return command{
		name:        "version",
		description: "Print the CLI version information",
		skipInit:    true,
		run: func(fs *flag.FlagSet, args []string, ctx *AppContext, stdout io.Writer, stderr io.Writer) error {
			_, err := fmt.Fprintln(stdout, versionString())
			return err
		},
	}
}
