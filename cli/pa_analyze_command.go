package cli

import "github.com/choria-io/fisk"

type PaAnalyzeCmd struct {
	archivePath string
}

func configurePaAnalyzeCommand(srv *fisk.CmdClause) {
	c := &PaAnalyzeCmd{}

	analyze := srv.Command("analyze", "create archive of monitoring data for all servers and accounts").Action(c.analyze)
	analyze.Arg("archivePath", "path of archive to extract information from and analyze").Required().StringVar(&c.archivePath)
}

func (c *PaAnalyzeCmd) analyze(_ *fisk.ParseContext) error {

	return nil
}
