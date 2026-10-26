package main

import (
	"os"

	"github.com/m10rten/go-canary-cli/internal/packaging"
)

func main() {
	os.Exit(packaging.Run(os.Args[1:], os.Stdout, os.Stderr))
}
