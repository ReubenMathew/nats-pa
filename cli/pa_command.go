package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/choria-io/fisk"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/natscli/archive"
)

func configurePaCommand(app commandHost) {
	srv := app.Command("pa", "NATS PA commands")

	configurePaGatherCommand(srv)
}

func init() {
	registerCommand("pa", 21, configurePaCommand)
}

type PaGatherCmd struct {
}

func configurePaGatherCommand(srv *fisk.CmdClause) {
	c := &PaGatherCmd{}

	srv.Command("gather", "create archive of monitoring data for all servers and accounts").Action(c.gather)
}

var endpoints = []string{
	"VARZ",
	"CONNZ",
	"ROUTEZ",
	"GATEWAYZ",
	"LEAFZ",
	"SUBSZ",
	"JSZ",
	"ACCOUNTZ",
}

var accountEndpoints = []string{
	"CONNZ",
	"LEAFZ",
	"SUBSZ",
	"JSZ",
	"INFO",
}

func (c *PaGatherCmd) gather(_ *fisk.ParseContext) error {

	// nats connection
	nc, err := newNatsConn("", natsOpts()...)
	if err != nil {
		return err
	}
	defer nc.Close()

	// archive writer
	archivePath := filepath.Join(os.TempDir(), "archive.zip")
	fmt.Printf("archivePath: %v\n", archivePath)
	aw, err := archive.NewWriter(archivePath)
	if err != nil {
		return err
	}
	defer aw.Close()

	// discover servers
	servers := []*server.ServerInfo{}
	err = doReqAsync(nil, "$SYS.REQ.SERVER.PING", 0, nc, func(b []byte) {
		var apiResponse server.ServerAPIResponse
		if err = json.Unmarshal(b, &apiResponse); err != nil {
			panic(err)
		}
		servers = append(servers, apiResponse.Server)
	})
	if err != nil {
		return err
	}

	// get server endpoint data
	for _, serverInfo := range servers {
		for _, endpoint := range endpoints {
			subject := fmt.Sprintf("$SYS.REQ.SERVER.%s.%s", serverInfo.ID, endpoint)
			err = doReqAsync(nil, subject, 1, nc, func(b []byte) {
				var apiResponse server.ServerAPIResponse
				if err = json.Unmarshal(b, &apiResponse); err != nil {
					panic(err)
				}
				archivePath := fmt.Sprintf("server_%s_%s.json", apiResponse.Server.Name, strings.ToLower(endpoint))
				err = aw.AddArtifact(archivePath, b)
				if err != nil {
					panic(err)
				}
			})
		}
	}

	// get accounts
	var accountIds []string
	err = doReqAsync(nil, "$SYS.REQ.SERVER.PING.ACCOUNTZ", 1, nc, func(b []byte) {
		var apiResponse server.ServerAPIResponse
		if err = json.Unmarshal(b, &apiResponse); err != nil {
			panic(err)
		}
		bytes, err := json.Marshal(apiResponse.Data)
		if err != nil {
			panic(err)
		}
		var accounts *server.Accountz
		if err = json.Unmarshal(bytes, &accounts); err != nil {
			panic(err)
		}
		accountIds = accounts.Accounts
	})
	if err != nil {
		return err
	}

	// get account endpoint data
	for _, accountId := range accountIds {
		for _, endpoint := range accountEndpoints {
			subject := fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.%s", accountId, endpoint)
			err = doReqAsync(nil, subject, 1, nc, func(b []byte) {
				var apiResponse server.ServerAPIResponse
				if err = json.Unmarshal(b, &apiResponse); err != nil {
					panic(err)
				}
				archivePath := fmt.Sprintf("account_%s_%s.json", accountId, strings.ToLower(endpoint))
				err = aw.AddArtifact(archivePath, b)
				if err != nil {
					panic(err)
				}
			})
		}
	}

	return nil
}
