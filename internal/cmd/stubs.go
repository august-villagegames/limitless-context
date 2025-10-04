package cmd

import (
	"flag"
	"fmt"
	"io"
)

func stubRun(action string) func(fs *flag.FlagSet, args []string, ctx *AppContext, stdout io.Writer, stderr io.Writer) error {
	return func(fs *flag.FlagSet, args []string, ctx *AppContext, stdout io.Writer, stderr io.Writer) error {
		msg := fmt.Sprintf("%s workflow not implemented yet; see docs/ROADMAP.md for status", action)
		if ctx != nil && ctx.Logger != nil {
			ctx.Logger.Info("command stub executed", "command", fs.Name())
		}
		fmt.Fprintln(stdout, msg)
		return nil
	}
}
