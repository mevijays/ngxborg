package cli

import (
	"context"
	"flag"

	"ngxborg/internal/logx"
	"ngxborg/internal/provision"
)

func cmdSetup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	adminPort := fs.Int("admin-port", 22, "port sshd keeps serving ordinary administrative SSH on")
	borgPort := fs.Int("borg-port", 2222, "second port sshd listens on, restricted to forced-command borg traffic")
	dryRun := fs.Bool("dry-run", false, "print what would change without changing anything")
	verbose := fs.Bool("verbose", false, "show every command run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *verbose {
		logx.SetLevel(logx.LevelDebug)
	}

	c, err := provision.New(ctx, *dryRun)
	if err != nil {
		return err
	}
	return c.Setup(provision.SetupOptions{AdminPort: *adminPort, BorgPort: *borgPort})
}

func cmdInstall(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "service" {
		return errUsage("ngxborg install service [--addr :8443]")
	}
	fs := flag.NewFlagSet("install service", flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address, e.g. :8443 or 127.0.0.1:8443 (default: keep whatever setup configured)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	c, err := provision.New(ctx, false)
	if err != nil {
		return err
	}
	return c.InstallService(*addr)
}

func cmdUninstall(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "service" {
		return errUsage("ngxborg uninstall service")
	}
	c, err := provision.New(ctx, false)
	if err != nil {
		return err
	}
	return c.RemoveService()
}

func cmdDoctor(ctx context.Context, args []string) error {
	c, err := provision.New(ctx, false)
	if err != nil {
		return err
	}
	checks := c.Diagnose()
	fails, warns := 0, 0
	logx.Section("Diagnostics")
	for _, ck := range checks {
		switch ck.Status {
		case provision.StatusOK:
			logx.Change("%-24s %s", ck.Name, ck.Detail)
		case provision.StatusWarn:
			warns++
			logx.Warn("%-24s %s%s", ck.Name, ck.Detail, fixSuffix(ck.Fix))
		case provision.StatusFail:
			fails++
			logx.Error("%-24s %s%s", ck.Name, ck.Detail, fixSuffix(ck.Fix))
		}
	}
	logx.Section("Summary")
	logx.Info("%d checks, %d failures, %d warnings", len(checks), fails, warns)
	return nil
}

func fixSuffix(fix string) string {
	if fix == "" {
		return ""
	}
	return " — " + fix
}

func errUsage(usage string) error {
	return &usageError{usage}
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return "usage: " + e.msg }
