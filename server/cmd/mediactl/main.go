package main

import (
	"fmt"
	"os"

	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/mediactl"
)

// main delegates media maintenance commands to the testable mediactl package.
func main() {
	cfg, err := wtconfig.LoadMediactlConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load mediactl config: %v\n", err)
		os.Exit(1)
	}
	os.Exit(mediactl.Run(os.Args[1:], cfg.LookupEnv, os.Stdout, os.Stderr))
}
