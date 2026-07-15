package cmd

import (
	"errors"
	"testing"

	mcpserver "github.com/techgodhq/creed/internal/mcp"
)

func TestMCPCommandRegistersServeSubcommand(t *testing.T) {
	matches := 0
	for _, command := range rootCmd.Commands() {
		if command.Name() == "mcp" {
			matches++
			if _, _, err := command.Find([]string{"serve"}); err != nil {
				t.Fatalf("mcp command should register serve subcommand: %v", err)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("expected exactly one mcp command, got %d", matches)
	}
}

func TestMCPServeCommandStartsGeneratedServer(t *testing.T) {
	called := false
	cmd := newMCPCommand(".", func(server *mcpserver.Server) error {
		called = true
		for _, want := range []string{"list_targets", "sync"} {
			if !containsString(server.ToolNames(), want) {
				t.Fatalf("serve command constructed MCP server without %q tool: %v", want, server.ToolNames())
			}
		}
		return nil
	})
	cmd.SetArgs([]string{"serve"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("mcp serve command failed: %v", err)
	}
	if !called {
		t.Fatal("mcp serve command did not start server")
	}
}

func TestMCPServeCommandReturnsServeError(t *testing.T) {
	wantErr := errors.New("stdio closed")
	cmd := newMCPCommand(".", func(server *mcpserver.Server) error {
		return wantErr
	})
	cmd.SetArgs([]string{"serve"})

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("mcp serve error = %v, want %v", err, wantErr)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
