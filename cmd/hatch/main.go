package main

import (
	"fmt"
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/cmd/root"
)

func main() {
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
