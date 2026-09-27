package main

import (
	"fmt"
	"os"

	"beam/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "beam:", err)
		os.Exit(1)
	}
}
