package cli

import (
	"context"
	"flag"
	"os"
	"strings"

	"ngxborg/internal/borgrepo"
	"ngxborg/internal/logx"
	"ngxborg/internal/sshaccess"
)

func cmdUserKey(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage("ngxborg user key <add|list|remove> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return cmdUserKeyAdd(ctx, rest)
	case "list":
		return cmdUserKeyList(ctx, rest)
	case "remove":
		return cmdUserKeyRemove(ctx, rest)
	default:
		return errUsage("ngxborg user key <add|list|remove> ...")
	}
}

func cmdUserKeyAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user key add", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant to add the key for (required for admin, ignored for a tenant's own session)")
	appendOnly := fs.Bool("append-only", false, "restrict the key to appending new archives — it cannot delete or prune existing ones")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errUsage("ngxborg user key add [--tenant <name>] [--append-only] <repo> <pubkey-or-@file>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	repoName := fs.Arg(0)
	pubKey, err := readKeyArg(fs.Arg(1))
	if err != nil {
		return err
	}
	if !borgrepo.Exists(tenant, repoName) {
		return errUsage("no repository named " + repoName + " for " + tenant + " — create it first with `ngxborg repo create`")
	}
	repoPath := borgrepo.Path(tenant, repoName)
	if err := sshaccess.AddKey(tenant, repoPath, pubKey, *appendOnly); err != nil {
		return err
	}
	mode := "read-write"
	if *appendOnly {
		mode = "append-only"
	}
	logx.Change("registered a %s key for %s, restricted to %s", mode, tenant, repoPath)
	return nil
}

// readKeyArg accepts either the key literally on the command line or, when
// prefixed with "@", a path to read it from — the same "@file" convention
// curl uses — since a public key line is long and copy-pasting it directly
// into a shell command is exactly the kind of thing that silently mangles a
// character.
func readKeyArg(arg string) (string, error) {
	if !strings.HasPrefix(arg, "@") {
		return arg, nil
	}
	data, err := os.ReadFile(strings.TrimPrefix(arg, "@"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func cmdUserKeyList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user key list", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant to list keys for")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	keys, err := sshaccess.ListKeys(tenant)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		logx.Info("%s has no registered backup keys yet", tenant)
		return nil
	}
	logx.Section("Keys for %s", tenant)
	for _, k := range keys {
		mode := "read-write"
		if k.AppendOnly {
			mode = "append-only"
		}
		comment := k.Comment
		if comment == "" {
			comment = "(no comment)"
		}
		logx.Info("%-10s %-40s %s", mode, k.RepoPath, comment)
	}
	return nil
}

func cmdUserKeyRemove(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user key remove", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant to remove the key from")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg user key remove [--tenant <name>] <key-material>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	if err := sshaccess.RemoveKey(tenant, fs.Arg(0)); err != nil {
		return err
	}
	logx.Change("removed key from %s", tenant)
	return nil
}
