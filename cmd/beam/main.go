package main

import (
	"fmt"
	"os"

	"beam/internal/cli"
)

func main() {
	err := cli.NewRoot().Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "beam:", err)
	}
	flushOutputs()
	if err != nil {
		os.Exit(1)
	}
}

func flushOutputs() {
	syncFile(os.Stdout)
	syncFile(os.Stderr)
}

func syncFile(file *os.File) {
	if file != nil {
		_ = file.Sync()
	}
}
