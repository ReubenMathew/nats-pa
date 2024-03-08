package cli

func configurePaCommand(app commandHost) {
	srv := app.Command("pa", "NATS PA commands")

	configurePaGatherCommand(srv)
}

func init() {
	registerCommand("pa", 21, configurePaCommand)
}

