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
	configurePaConnCheckCommand(srv)
}

func init() {
	// NOTE: what does the order arg do?
	registerCommand("pa", 21, configurePaCommand)
}

type PaConnCheckCmd struct {
}

func configurePaConnCheckCommand(srv *fisk.CmdClause) {
	c := &PaConnCheckCmd{}

	srv.Command("check", "show connection health").Action(c.connectionCheck)
}

func (c *PaConnCheckCmd) connectionCheck(_ *fisk.ParseContext) error {
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
