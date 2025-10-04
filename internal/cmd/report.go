package cmd

func newReportCommand() command {
	return command{
		name:        "report",
		description: "Render the HTML report for a processed run",
		run:         stubRun("report"),
	}
}
