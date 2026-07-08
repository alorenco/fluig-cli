package main

import (
	"os"

	"github.com/alorenco/fluig-cli/internal/cli"
)

// Preenchidos pelo goreleaser via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Main(version, commit, date))
}
