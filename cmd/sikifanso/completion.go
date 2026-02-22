package main

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func clusterNameShellComplete(_ context.Context, cmd *cli.Command) {
	sessions, err := session.ListAll()
	if err != nil {
		return
	}
	for _, s := range sessions {
		_, _ = fmt.Fprintln(cmd.Root().Writer, s.ClusterName)
	}
}
