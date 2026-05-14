package main

import (
	"os"

	"github.com/misfitdev/richmond/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
