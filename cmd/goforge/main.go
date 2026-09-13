// Command goforge is a scaffold for Go backend services.
//
// main.go is deliberately thin: it only connects the process world
// (os.Args, os.Stdout, os.Stderr, exit code) to the CLI package.
// Everything else lives in internal/cli so it can be tested without
// starting a real process.
package main

import (
	"os"

	"goforge/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
