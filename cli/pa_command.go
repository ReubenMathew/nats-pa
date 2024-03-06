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
	nsys "github.com/piotrpio/nats-sys-client/pkg/sys"
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
	"HEALTHZ",
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

	// js context
	//js, err := nc.JetStream(jsOpts()...)
	//if err != nil {
	//return err
	//}

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
			// TODO: pull this out of callback
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
	// TODO: pull this out of callback
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
			// TODO: pull this out of callback
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

	sys, err := nsys.NewSysClient(nc)
	if err != nil {
		return err
	}

	for _, accountId := range accountIds {
		jszResponses, err := sys.JszPing(
			nsys.JszEventOptions{
				JszOptions: nsys.JszOptions{
					Account: accountId,
					Streams: true,
				},
			},
		)
		if err != nil {
			return err
		}
		for _, jszResp := range jszResponses {
			for _, ad := range jszResp.JSInfo.AccountDetails {
				for _, sd := range ad.Streams {
					// TODO: serialize
					// TODO: tokenize
// gather method 
					fmt.Printf("accountId: %s, streamDetail: %+v\n", accountId, sd)
				}
			}
		}
	}

	/*
		// --- per-asset info and state --- //
		// streams
		streams, streamNames, err := mgr.Streams(nil)
		if err != nil {
			return err
		}
		for _, stream := range streams {
			streamInfo, err := stream.Information()
			if err != nil {
				return err
			}
			streamInfoBytes, err := serialize(streamInfo)
			if err != nil {
				return err
			}
			archivePath := fmt.Sprintf("stream_%s_info.json", streamInfo.Config.Name)
			if err = aw.AddArtifact(archivePath, streamInfoBytes); err != nil {
				return err
			}
		}

		// consumers
		for _, streamName := range streamNames {
			consumers, _, err := mgr.Consumers(streamName)
			if err != nil {
				return err
			}
			for _, consumer := range consumers {
				consumerState, err := consumer.State()
				if err != nil {
					return err
				}
				consumerStateBytes, err := serialize(consumerState)
				if err != nil {
					return err
				}
				archivePath := fmt.Sprintf("consumer_%s_state.json", consumerState.Name)
				if err = aw.AddArtifact(archivePath, consumerStateBytes); err != nil {
					return err
				}
			}
		}
	*/

	return nil
}

func serialize(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
