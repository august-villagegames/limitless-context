package main

import (
	"os"

	"github.com/offlinefirst/limitless-context/internal/cmd"
)

func main() {
	root := cmd.NewRootCommand()
	if err := root.Execute(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
