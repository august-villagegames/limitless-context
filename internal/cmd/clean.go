package cmd

func newCleanCommand() command {
	return command{
		name:        "clean",
		description: "Remove generated artifacts (requires confirmation)",
		run:         stubRun("cleanup"),
	}
}
