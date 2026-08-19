// Command ngxborg is a multi-tenant Borg backup server: setup/provisioning,
// tenant and repository management, and a PAM-authenticated web UI, all in
// one binary — see internal/cli for the command surface.
package main

import (
	"context"
	"os"

	"ngxborg/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:]))
}
