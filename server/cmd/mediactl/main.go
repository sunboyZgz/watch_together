package main

import (
	"os"

	"watch_together/server/internal/mediactl"
)

// main delegates media maintenance commands to the testable mediactl package.
func main() {
	os.Exit(mediactl.Run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
