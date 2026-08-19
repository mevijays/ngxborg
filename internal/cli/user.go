package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"ngxborg/internal/logx"
	"ngxborg/internal/posix"
	"ngxborg/internal/sshaccess"
	"ngxborg/internal/system"
)

func cmdUser(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage("ngxborg user <create|delete|list|passwd|disable|enable|key> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return cmdUserCreate(ctx, rest)
	case "delete":
		return cmdUserDelete(ctx, rest)
	case "list":
		return cmdUserList(ctx, rest)
	case "passwd":
		return cmdUserPasswd(ctx, rest)
	case "disable":
		return cmdUserDisable(ctx, rest)
	case "enable":
		return cmdUserEnable(ctx, rest)
	case "key":
		return cmdUserKey(ctx, rest)
	default:
		return errUsage("ngxborg user <create|delete|list|passwd|disable|enable|key> ...")
	}
}

func cmdUserCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user create", flag.ContinueOnError)
	admin := fs.Bool("admin", false, "grant this account full cross-tenant visibility")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg user create [--admin] <username>")
	}
	if err := system.RequireRoot(); err != nil {
		return err
	}
	username := fs.Arg(0)
	r := system.Runner{}
	if err := posix.CreateUser(ctx, r, username, *admin); err != nil {
		return err
	}
	role := "tenant"
	if *admin {
		role = "admin"
	}
	logx.Change("created %s account %s (locked — run `ngxborg user passwd %s` to set a password)", role, username, username)
	return nil
}

func cmdUserDelete(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errUsage("ngxborg user delete <username>")
	}
	if err := system.RequireRoot(); err != nil {
		return err
	}
	r := system.Runner{}
	if err := posix.DeleteUser(ctx, r, args[0]); err != nil {
		return err
	}
	logx.Change("deleted account %s (its repositories were not touched — see `ngxborg repo delete/purge`)", args[0])
	return nil
}

func cmdUserDisable(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errUsage("ngxborg user disable <username>")
	}
	if err := system.RequireRoot(); err != nil {
		return err
	}
	if err := posix.Disable(ctx, system.Runner{}, args[0]); err != nil {
		return err
	}
	logx.Change("disabled %s — the web UI and every SSH key on their account are locked out until `ngxborg user enable %s`", args[0], args[0])
	return nil
}

func cmdUserEnable(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errUsage("ngxborg user enable <username>")
	}
	if err := system.RequireRoot(); err != nil {
		return err
	}
	if err := posix.Enable(ctx, system.Runner{}, args[0]); err != nil {
		return err
	}
	logx.Change("enabled %s", args[0])
	return nil
}

func cmdUserList(ctx context.Context, args []string) error {
	if err := system.RequireRoot(); err != nil {
		return err
	}
	names, err := posix.ListTenants()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		logx.Info("no ngxborg accounts yet — see `ngxborg user create`")
		return nil
	}
	logx.Section("Accounts")
	for _, name := range names {
		role := "tenant"
		if posix.IsAdmin(name) {
			role = "admin"
		}
		keys, _ := sshaccess.ListKeys(name)
		status := ""
		if disabled, err := posix.IsDisabled(name); err == nil && disabled {
			status = " [disabled]"
		}
		logx.Info("%-20s %-8s %-10s%s", name, role, sshaccess.FormatCount(len(keys)), status)
	}
	return nil
}

func cmdUserPasswd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user passwd", flag.ContinueOnError)
	generate := fs.Bool("generate", false, "generate a random password instead of prompting")
	if err := fs.Parse(args); err != nil {
		return err
	}

	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	username := id.Username
	if fs.NArg() >= 1 {
		username = fs.Arg(0)
	}
	if username == "" {
		return errUsage("ngxborg user passwd [--generate] <username> (admin), or ngxborg user passwd [--generate] (tenant, sets your own)")
	}
	if !id.Admin && username != id.Username {
		return fmt.Errorf("you are logged in as %s and can only change your own password", id.Username)
	}

	r := system.Runner{}
	var password string
	if *generate {
		password, err = system.Password(20)
		if err != nil {
			return err
		}
	} else {
		password, err = readPassword(fmt.Sprintf("New password for %s: ", username))
		if err != nil {
			return err
		}
	}
	if err := posix.SetPassword(ctx, r, username, password); err != nil {
		return err
	}
	// A generated password can never be re-shown; SetPassword itself does
	// not print it, so this call site is the one place it is ever surfaced
	// — deliberately not through logx (whose output can end up in a log
	// file or terminal scrollback an operator revisits later), but a
	// direct, unambiguous print.
	if *generate {
		fmt.Fprintf(os.Stderr, "Generated password for %s: %s\n(shown once — save it now)\n", username, password)
	}
	logx.Change("password set for %s", username)
	return nil
}
