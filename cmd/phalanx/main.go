package main

import (
	"fmt"
	"os"

	"github.com/satnambhatt/phlx/internal/cli"
	"github.com/satnambhatt/phlx/internal/db"
)

// Injected at build time by goreleaser via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "phalanx: db open failed:", err)
		os.Exit(1)
	}
	_, _ = commit, date
	if err := cli.Root(version).Execute(); err != nil {
		os.Exit(1)
	}
}
