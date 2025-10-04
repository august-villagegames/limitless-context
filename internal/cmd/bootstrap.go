package cmd

func newBootstrapCommand() command {
	return command{
		name:        "bootstrap",
		description: "Verify local prerequisites and prepare the workspace",
		run:         stubRun("bootstrap"),
	}
}
