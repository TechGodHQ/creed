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
