//go:generate go run ./internal/codegen --service internal/service/service.go --out-cli cmd/gen --out-mcp internal/mcp/gen --out-ops internal/ops/gen --out-http internal/httpapi/gen

package main

import (
	"fmt"
	"os"

	"github.com/techgodhq/creed/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "creed: %v\n", err)
		os.Exit(1)
	}
}
