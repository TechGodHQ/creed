package cmd

import (
	"github.com/spf13/cobra"

	mcpserver "github.com/techgodhq/creed/internal/mcp"
	"github.com/techgodhq/creed/internal/service"
)

type mcpServeFunc func(*mcpserver.Server) error

func newMCPCommand(defaultRoot string, serve mcpServeFunc) *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run Creed MCP tooling",
		Long:  "Run Creed's Model Context Protocol (MCP) tooling for agent clients.",
	}

	root := defaultRoot
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve generated Creed MCP tools over stdio",
		Long:  "Serve generated Creed MCP tools over stdio for MCP-compatible clients such as Claude Desktop and Cursor.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(mcpserver.NewServer(service.New(root)))
		},
	}
	serveCmd.Flags().StringVar(&root, "root", defaultRoot, "project root containing .creed")

	mcpCmd.AddCommand(serveCmd)
	return mcpCmd
}

func serveMCPStdio(server *mcpserver.Server) error {
	return server.ServeStdio()
}
