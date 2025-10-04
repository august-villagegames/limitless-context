package cmd

func newDoctorCommand() command {
	return command{
		name:        "doctor",
		description: "Inspect environment readiness for optional subsystems",
		run:         stubRun("doctor"),
	}
}
