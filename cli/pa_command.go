package cli

import (
	"fmt"

	"github.com/choria-io/fisk"
	"github.com/nats-io/nats.go"
)

func configurePaCommand(app commandHost) {
	srv := app.Command("pa", "NATS PA commands")
	// cheatsheet??
	//addCheat(, cmd *fisk.CmdClause)

	// subcommands
	configurePaHealthcheckCommand(srv)
}

func init() {
	// NOTE: what does the order arg do?
	registerCommand("pa", 21, configurePaCommand)
}

// TODO: move to it's own file when done POC
// -----------------------------------------

type PaHealthcheckCmd struct {
}

func configurePaHealthcheckCommand(srv *fisk.CmdClause) {
	c := &PaHealthcheckCmd{}

	srv.Command("healthcheck", "show health of servers").Action(c.healthcheck)
}

func (c *PaHealthcheckCmd) healthcheck(_ *fisk.ParseContext) error {
	nc, err := newNatsConn("", natsOpts()...)
	if err != nil {
		return err
	}
	defer nc.Close()

	for _, serverUrl := range nc.Servers() {
		ncCheck, err := nats.Connect(serverUrl)
		if err != nil {
			return err
		} else {
			fmt.Printf("Able to connect to server: %s, url: %s\n", ncCheck.ConnectedServerName(), serverUrl)
		}
	}

	return nil
}
