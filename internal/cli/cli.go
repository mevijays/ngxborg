// Package cli implements ngxborg's command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"

	"ngxborg/internal/build"
	"ngxborg/internal/logx"
)

// Run parses os.Args and dispatches to the matching subcommand, returning
// the process exit code.
func Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Print(usage())
		return 1
	}

	var err error
	switch args[0] {
	case "setup":
		err = cmdSetup(ctx, args[1:])
	case "install":
		err = cmdInstall(ctx, args[1:])
	case "uninstall":
		err = cmdUninstall(ctx, args[1:])
	case "user":
		err = cmdUser(ctx, args[1:])
	case "repo":
		err = cmdRepo(ctx, args[1:])
	case "doctor":
		err = cmdDoctor(ctx, args[1:])
	case "web":
		err = cmdWeb(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Printf("ngxborg %s\n", build.Version)
		fmt.Printf("Maintainer:  %s\n", build.Maintainer)
		fmt.Printf("Repository:  %s\n", build.RepoURL)
		return 0
	case "help", "--help", "-h":
		fmt.Print(usage())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usage())
		return 1
	}

	logx.Summary()
	if err != nil {
		logx.Error("%v", err)
		return 1
	}
	return 0
}

func usage() string {
	return `ngxborg — multi-tenant Borg backup server

SETUP
  ngxborg setup [--admin-port 22] [--borg-port 2222] [--dry-run]
      Install packages, create the tenant/admin groups, wire up PAM,
      configure sshd's dual-port listener. Safe to re-run.

  ngxborg install service [--addr :8443]
      Enable and start the web UI as a systemd service.

  ngxborg uninstall service
      Stop and disable the web UI (accounts and repositories untouched).

USERS
  ngxborg user create [--admin] <username>
  ngxborg user delete <username>
  ngxborg user list
  ngxborg user passwd [--generate] [username]
      Set/reset a login password. A tenant with no argument sets their own.
  ngxborg user disable <username>
  ngxborg user enable <username>
      Locks out the web UI and every SSH key on the account at once,
      without touching the password, keys, or repositories — reversible.
  ngxborg user key add [--tenant <name>] [--append-only] <repo> <pubkey-or-@file>
  ngxborg user key list [--tenant <name>]
  ngxborg user key remove [--tenant <name>] <key-material>

REPOSITORIES
  ngxborg repo create [--tenant <name>] <repo>
  ngxborg repo list [--tenant <name>]
      Admin with no --tenant lists every tenant's repositories.
  ngxborg repo delete [--tenant <name>] <repo>
      Soft delete — recoverable until purged.
  ngxborg repo purge [--tenant <name>] [--yes] <repo>
      Permanent, irreversible.
  ngxborg repo disable [--tenant <name>] <repo>
  ngxborg repo enable [--tenant <name>] <repo>
      Blocks every SSH key restricted to this repository, without
      removing the keys or the data — reversible.

  ngxborg doctor
  ngxborg version

Commands that touch other accounts, sshd, or PAM need root (sudo). A
tenant runs everything else as themselves — no sudo, scoped automatically
to their own account.

Flags must come before positional arguments (ngxborg repo create --tenant
alice mybackup, not the other way around) — a plain consequence of Go's
standard flag parser stopping at the first non-flag argument, the same
constraint every Go CLI built on it shares.
`
}
