// Package provision brings a bare Debian-family host up to a running
// ngxborg backup server: packages installed, tenant/admin groups created,
// PAM wired up, sshd listening on its second port, and — as a distinct,
// later, opt-in step — the web UI actually running as a service.
package provision

import (
	"context"
	"fmt"

	"ngxborg/internal/facts"
	"ngxborg/internal/system"
)

// Ctx carries the state every provisioning step needs.
type Ctx struct {
	Context context.Context
	Runner  system.Runner
	Facts   facts.Facts
}

// New gathers facts about the running machine and returns a ready-to-use
// Ctx. dryRun makes every mutating step print what it would do instead of
// doing it, the same convention ngxsetup uses throughout.
func New(ctx context.Context, dryRun bool) (*Ctx, error) {
	c := &Ctx{
		Context: ctx,
		Runner:  system.Runner{DryRun: dryRun},
	}
	c.Facts = facts.Detect("/")
	return c, nil
}

// preflight refuses to proceed on a platform this tool was never built for
// (package names, sshd_config.d, and systemd unit conventions all assume
// Debian-family) or without root, the same up-front checks ngxsetup itself
// makes before touching anything.
func (c *Ctx) preflight() error {
	if err := system.RequireRoot(); err != nil {
		return err
	}
	if !c.Facts.OS.DebianFamily() {
		return fmt.Errorf("ngxborg supports Debian/Ubuntu hosts; detected %q", c.Facts.OS.PrettyName)
	}
	return nil
}
