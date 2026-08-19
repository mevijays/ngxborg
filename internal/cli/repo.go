package cli

import (
	"context"
	"flag"
	"time"

	"ngxborg/internal/borgrepo"
	"ngxborg/internal/facts"
	"ngxborg/internal/logx"
)

func cmdRepo(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage("ngxborg repo <create|list|delete|purge|disable|enable> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return cmdRepoCreate(ctx, rest)
	case "list":
		return cmdRepoList(ctx, rest)
	case "delete":
		return cmdRepoDelete(ctx, rest)
	case "purge":
		return cmdRepoPurge(ctx, rest)
	case "disable":
		return cmdRepoDisable(ctx, rest)
	case "enable":
		return cmdRepoEnable(ctx, rest)
	default:
		return errUsage("ngxborg repo <create|list|delete|purge|disable|enable> ...")
	}
}

func cmdRepoCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo create", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant to create the repository for (required for admin)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg repo create [--tenant <name>] <repo>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	r, err := borgrepo.CreateRepo(tenant, fs.Arg(0))
	if err != nil {
		return err
	}
	logx.Change("reserved repository at %s", r.Path)
	logx.Info("register a key for it with: ngxborg user key add --tenant %s %s <pubkey-or-@file>", tenant, r.Name)
	logx.Info("then, from the tenant's own machine: borg init --encryption=repokey-blake2 ssh://%s@<this-host>:<borg-port>%s", tenant, r.Path)
	return nil
}

func cmdRepoList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo list", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "list one tenant's repositories; admin with no --tenant lists everyone's")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}

	var repos []borgrepo.Repo
	if !id.Admin {
		tenant, err := scopeTenant(id, *tenantFlag)
		if err != nil {
			return err
		}
		repos, err = borgrepo.ListRepos(tenant)
		if err != nil {
			return err
		}
	} else if *tenantFlag != "" {
		repos, err = borgrepo.ListRepos(*tenantFlag)
		if err != nil {
			return err
		}
	} else {
		repos, err = borgrepo.ListAllRepos()
		if err != nil {
			return err
		}
	}

	if len(repos) == 0 {
		logx.Info("no repositories yet — see `ngxborg repo create`")
		return nil
	}
	logx.Section("Repositories")
	for _, r := range repos {
		state := "empty (not yet initialized by a borg client)"
		if r.Initialized {
			state = facts.FormatMB(r.SizeMB)
		}
		if r.Disabled {
			state += " [disabled]"
		}
		logx.Info("%-14s %-20s %-40s %s", r.Tenant, r.Name, state, r.CreatedAt.Format(time.DateOnly))
	}
	return nil
}

func cmdRepoDelete(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo delete", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant's repository to delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg repo delete [--tenant <name>] <repo>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	if err := borgrepo.Delete(tenant, fs.Arg(0)); err != nil {
		return err
	}
	logx.Change("moved %s/%s to the trash — recoverable until `ngxborg repo purge`", tenant, fs.Arg(0))
	return nil
}

func cmdRepoDisable(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo disable", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant's repository to disable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg repo disable [--tenant <name>] <repo>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	if err := borgrepo.Disable(tenant, fs.Arg(0)); err != nil {
		return err
	}
	logx.Change("disabled %s/%s — every SSH key restricted to it is locked out until `ngxborg repo enable`", tenant, fs.Arg(0))
	return nil
}

func cmdRepoEnable(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo enable", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant's repository to enable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg repo enable [--tenant <name>] <repo>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	if err := borgrepo.Enable(tenant, fs.Arg(0)); err != nil {
		return err
	}
	logx.Change("enabled %s/%s", tenant, fs.Arg(0))
	return nil
}

func cmdRepoPurge(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repo purge", flag.ContinueOnError)
	tenantFlag := fs.String("tenant", "", "which tenant's repository to purge")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage("ngxborg repo purge [--tenant <name>] [--yes] <repo>")
	}
	id, err := resolveIdentity()
	if err != nil {
		return err
	}
	tenant, err := scopeTenant(id, *tenantFlag)
	if err != nil {
		return err
	}
	repo := fs.Arg(0)
	if !*yes {
		ok, err := confirm("This permanently and irreversibly deletes " + tenant + "/" + repo + ". Continue?")
		if err != nil {
			return err
		}
		if !ok {
			logx.Info("cancelled")
			return nil
		}
	}
	if err := borgrepo.Purge(tenant, repo); err != nil {
		return err
	}
	logx.Change("permanently deleted %s/%s", tenant, repo)
	return nil
}
