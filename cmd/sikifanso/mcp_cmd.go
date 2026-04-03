package main

import (
	"context"

	mcpserver "github.com/alicanalbayrak/sikifanso/internal/mcp"
	"github.com/urfave/cli/v3"
)

func mcpCmd() *cli.Command {
	return &cli.Command{
		Name:   "mcp",
		Usage:  "Model Context Protocol server",
		Hidden: true,
		Commands: []*cli.Command{
			mcpServeCmd(),
		},
	}
}

func mcpServeCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the MCP server (stdio transport)",
		Action: wrapAction(func(ctx context.Context, _ *cli.Command) error {
			return mcpserver.Run(ctx, &mcpserver.Deps{
				Logger: zapLogger,
			})
		}),
	}
}
