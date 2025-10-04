package cmd

func newBundleCommand() command {
	return command{
		name:        "bundle",
		description: "Generate task bundles from a completed capture run",
		run:         stubRun("bundle"),
	}
}
