package cmd

func newProcessCommand() command {
	return command{
		name:        "process",
		description: "Validate LLM outputs and update run artifacts",
		run:         stubRun("process"),
	}
}
