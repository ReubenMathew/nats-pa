package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

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

type Endpoint struct {
	name           string
	expectedStruct any
	typeTag        *archive.Tag
}

var serverEndpoints = []Endpoint{
	{
		"VARZ",
		server.Varz{},
		archive.TagServerVars(),
	},
	{
		"CONNZ",
		server.Connz{},
		archive.TagConnections(),
	},
	{
		"ROUTEZ",
		server.Routez{},
		archive.TagRoutes(),
	},
	{
		"GATEWAYZ",
		server.Gatewayz{},
		archive.TagGateways(),
	},
	{
		"LEAFZ",
		server.Leafz{},
		archive.TagLeafs(),
	},
	{
		"SUBSZ",
		server.Subsz{},
		archive.TagSubs(),
	},
	{
		"JSZ",
		server.JSInfo{},
		archive.TagJetStream(),
	},
	{
		"ACCOUNTZ",
		server.Accountz{},
		archive.TagAccounts(),
	},
	{
		"HEALTHZ",
		server.HealthStatus{},
		archive.TagHealth(),
	},
}

var accountEndpoints = []Endpoint{
	{
		"CONNZ",
		server.Connz{},
		archive.TagConnections(),
	},
	{
		"LEAFZ",
		server.Leafz{},
		archive.TagLeafs(),
	},
	{
		"SUBSZ",
		server.Subsz{},
		archive.TagSubs(),
	},
	{
		"INFO",
		server.AccountInfo{},
		// TODO: find a better tag
		archive.TagAccounts(),
	},
	{
		"JSZ",
		server.JetStreamStats{},
		archive.TagJetStream(),
	},
}

type ServerAPIResponse struct {
	Server *server.ServerInfo `json:"server"`
	Data   json.RawMessage    `json:"data,omitempty"`
	Error  *server.ApiError   `json:"error,omitempty"`
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
	if err = doReqAsync(nil, "$SYS.REQ.SERVER.PING", 0, nc, func(b []byte) {
		var apiResponse ServerAPIResponse
		if err = json.Unmarshal(b, &apiResponse); err != nil {
			panic(err)
		}
		servers = append(servers, apiResponse.Server)
	}); err != nil {
		return err
	}

	// get data from all endpoints for all servers
	for _, serverInfo := range servers {
		for _, endpoint := range serverEndpoints {

			subject := fmt.Sprintf("$SYS.REQ.SERVER.%s.%s", serverInfo.ID, endpoint.name)

			var apiResponseBytes []byte
			err = doReqAsync(nil, subject, 1, nc, func(b []byte) {
				apiResponseBytes = b
			})

			var apiResponse ServerAPIResponse
			if err = json.Unmarshal(apiResponseBytes, &apiResponse); err != nil {
				return err
			}

			// remarshal to get the api response payload struct
			var apiResponseDataBytes []byte
			apiResponseDataBytes, err = json.Marshal(apiResponse.Data)
			if err != nil {
				return err
			}

			resp := reflect.New(reflect.TypeOf(endpoint.expectedStruct)).Interface()
			if err = json.Unmarshal(apiResponseDataBytes, resp); err != nil {
				return err
			}
			aw.Add(resp, archive.TagServer(serverInfo.Name), archive.TagCluster(serverInfo.Cluster), endpoint.typeTag)
		}
	}

	// retrieve all accounts
	var (
		accountIds       []string
		apiResponseBytes []byte
	)
	{
		if err = doReqAsync(nil, "$SYS.REQ.SERVER.PING.ACCOUNTZ", 1, nc, func(b []byte) {
			apiResponseBytes = b
		}); err != nil {
			return err
		}

		var apiResponse ServerAPIResponse
		if err = json.Unmarshal(apiResponseBytes, &apiResponse); err != nil {
			panic(err)
		}

		var bytes []byte
		bytes, err = json.Marshal(apiResponse.Data)
		if err != nil {
			panic(err)
		}

		var accounts *server.Accountz
		if err = json.Unmarshal(bytes, &accounts); err != nil {
			panic(err)
		}
		accountIds = accounts.Accounts
	}

	// get account endpoint data
	for _, accountId := range accountIds {
		for _, endpoint := range accountEndpoints {
			subject := fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.%s", accountId, endpoint.name)

			var apiResponseBytes []byte
			if err = doReqAsync(nil, subject, 1, nc, func(b []byte) {
				apiResponseBytes = b
			}); err != nil {
				return err
			}

			var apiResponse ServerAPIResponse
			if err = json.Unmarshal(apiResponseBytes, &apiResponse); err != nil {
				return err
			}
			// handle api response error
			if apiResponse.Error != nil {
				fmt.Printf("%s cannot be collected for account %s, error: %s\n", endpoint.name, accountId, apiResponse.Error)
				continue
			}

			var apiResponseDataBytes []byte
			apiResponseDataBytes, err = json.Marshal(apiResponse.Data)
			if err != nil {
				return err
			}

			resp := reflect.New(reflect.TypeOf(endpoint.expectedStruct)).Interface()
			if err = json.Unmarshal(apiResponseDataBytes, resp); err != nil {
				fmt.Printf("failed to unmarshal %s api response from account %s: %s\n", endpoint.name, accountId, err.Error())
				continue
			}

			// add to archive
			if err = aw.Add(resp, archive.TagAccount(accountId), endpoint.typeTag); err != nil {
				return err
			}
		}
	}

	// TODO: remove this dependency
	sys, err := nsys.NewSysClient(nc)
	if err != nil {
		return err
	}

	// HACK: refactor to use wire protocol
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
					if err = aw.Add(sd, archive.TagAccount(accountId), archive.TagStream(sd.Name)); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}