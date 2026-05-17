package main

import (
	"fmt"
	"os"

	"github.com/satnambhatt/phlx/internal/cli"
	"github.com/satnambhatt/phlx/internal/db"
)

func main() {
	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "phalanx: db open failed:", err)
		os.Exit(1)
	}
	if err := cli.Root().Execute(); err != nil {
		os.Exit(1)
	}
}
