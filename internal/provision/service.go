package provision

import (
	"fmt"
	"os"

	"ngxborg/internal/logx"
	"ngxborg/internal/system"
)

// webUnitPath is the systemd unit for the web UI.
const webUnitPath = "/etc/systemd/system/ngxborg-web.service"

// webUnitTemplate runs as root deliberately: PAM's pam_unix.so needs to
// read /etc/shadow to check a password (root-only, the same reason ngxsetup
// itself always requires root), and the admin view has to be able to browse
// every tenant's own repository directory regardless of which uid owns it.
// This is real privilege the web UI carries — it should sit behind a
// trusted network or a reverse proxy with its own access control, the same
// operational caveat ngxsetup gives phpMyAdmin.
const webUnitTemplate = `# Managed by ngxborg — do not edit by hand; changes are overwritten by
# ` + "`ngxborg setup`" + `.
[Unit]
Description=ngxborg web UI
After=network.target ssh.service

[Service]
Type=simple
ExecStart=/usr/local/bin/ngxborg web --addr %s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`

// writeWebUnit renders the unit but never enables or starts it — that is
// InstallService's job, a distinct step an operator opts into explicitly.
func (c *Ctx) writeWebUnit() error {
	return c.writeWebUnitAddr(":8443")
}

func (c *Ctx) writeWebUnitAddr(addr string) error {
	body := fmt.Sprintf(webUnitTemplate, addr)
	if err := writeIfChanged(webUnitPath, body, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", webUnitPath, err)
	}
	if c.Runner.DryRun {
		return nil
	}
	return system.DaemonReload(c.Context, c.Runner)
}

// InstallService enables and starts the web UI as a systemd service —
// `ngxborg install service`. addr is the listen address ("host:port" or
// ":port"); an empty string keeps whatever writeWebUnit last configured
// (":8443" after a fresh Setup) rather than resetting it.
func (c *Ctx) InstallService(addr string) error {
	if err := c.preflight(); err != nil {
		return err
	}
	if err := RequireSetup(c.Context, c.Runner); err != nil {
		return err
	}
	if addr != "" {
		if err := c.writeWebUnitAddr(addr); err != nil {
			return err
		}
	} else if _, err := os.Stat(webUnitPath); err != nil {
		if err := c.writeWebUnit(); err != nil {
			return err
		}
	}
	if err := system.EnableNow(c.Context, c.Runner, "ngxborg-web.service"); err != nil {
		return fmt.Errorf("starting the web UI service: %w", err)
	}
	logx.Change("ngxborg-web.service is running")
	return nil
}

// RemoveService disables and stops the web UI without touching anything
// else — accounts, repositories, and sshd configuration all survive, since
// "stop running the web UI" and "tear down the backup server" are very
// different operator intents.
func (c *Ctx) RemoveService() error {
	if !c.Runner.DryRun {
		c.Runner.TryRun(c.Context, "systemctl", "disable", "--now", "ngxborg-web.service")
	}
	return nil
}
