package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"ngxborg/internal/logx"
)

// ---- systemd ---------------------------------------------------------------

// UnitExists reports whether systemd knows about a unit.
func UnitExists(ctx context.Context, r Runner, unit string) bool {
	out, err := r.Output(ctx, "systemctl", "list-unit-files", "--no-legend", unit)
	return err == nil && strings.Contains(out, unit)
}

// IsActive reports whether a unit is currently running.
func IsActive(ctx context.Context, r Runner, unit string) bool {
	out, _ := r.Output(ctx, "systemctl", "is-active", unit)
	return strings.TrimSpace(out) == "active"
}

// IsEnabled reports whether a unit starts at boot.
func IsEnabled(ctx context.Context, r Runner, unit string) bool {
	out, _ := r.Output(ctx, "systemctl", "is-enabled", unit)
	return strings.TrimSpace(out) == "enabled"
}

// DaemonReload makes systemd re-read unit files after a drop-in changes.
func DaemonReload(ctx context.Context, r Runner) error {
	return r.Run(ctx, "systemctl", "daemon-reload")
}

// EnableNow enables a unit and starts it if it is not already running.
func EnableNow(ctx context.Context, r Runner, unit string) error {
	if !UnitExists(ctx, r, unit) {
		return fmt.Errorf("unit %s does not exist", unit)
	}
	if !IsEnabled(ctx, r, unit) {
		if err := r.Run(ctx, "systemctl", "enable", unit); err != nil {
			return err
		}
	}
	if IsActive(ctx, r, unit) {
		return nil
	}
	return r.Run(ctx, "systemctl", "start", unit)
}

// Reload asks a unit to re-read its configuration without dropping traffic.
// Falls back to a restart for units that cannot reload.
func Reload(ctx context.Context, r Runner, unit string) error {
	if !UnitExists(ctx, r, unit) {
		return nil
	}
	if err := r.Run(ctx, "systemctl", "reload-or-restart", unit); err != nil {
		return fmt.Errorf("reloading %s: %w", unit, err)
	}
	logx.Change("reloaded %s", unit)
	return nil
}

// Restart bounces a unit.
func Restart(ctx context.Context, r Runner, unit string) error {
	if err := r.Run(ctx, "systemctl", "restart", unit); err != nil {
		return fmt.Errorf("restarting %s: %w", unit, err)
	}
	logx.Change("restarted %s", unit)
	return nil
}

// ---- privilege ---------------------------------------------------------

// RequireRoot returns an error unless the process is running as root. Nearly
// every ngxborg command needs it: creating a POSIX account, writing sshd
// config, authenticating another user's password via PAM (which needs
// read access to /etc/shadow) are all root-only operations.
func RequireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command changes system configuration and must run as root (try: sudo ngxborg ...)")
	}
	return nil
}

// IsRoot reports whether the process has root privileges.
func IsRoot() bool { return os.Geteuid() == 0 }
