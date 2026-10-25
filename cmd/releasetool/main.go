package main

import (
	"os"

	"github.com/m10rten/go-canary-cli/internal/releasetool"
)

func main() {
	os.Exit(releasetool.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
