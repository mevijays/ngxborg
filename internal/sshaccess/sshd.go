// Package sshaccess is the whole of ngxborg's backup transport: Borg has no
// daemon of its own (see internal/borg's own doc comment in ngxsetup, the
// client side of this exact relationship) — a borg client always just opens
// an SSH connection and the far end execs `borg serve`. Everything this
// package does is in service of two things: keeping that traffic on its own
// port, separate from normal administrative SSH on 22, and making sure a
// tenant's SSH key can only ever run `borg serve` against their own
// repository, never a shell.
package sshaccess

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ngxborg/internal/system"
)

// atomicWrite replaces a file's contents via write-then-rename, so a crash
// or power loss mid-write can never leave sshd reading a half-written
// config on its next start.
func atomicWrite(path, body string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DropInPath is where ngxborg's own sshd configuration lives. Modern
// OpenSSH (Ubuntu ships one) reads /etc/ssh/sshd_config.d/*.conf by
// default via an Include directive already present in the stock
// sshd_config, so this never needs to touch that file directly — the same
// drop-in pattern ngxsetup itself uses everywhere for exactly the same
// reason: an operator's own edits to the main file survive untouched.
const DropInPath = "/etc/ssh/sshd_config.d/60-ngxborg.conf"

// dropInTemplate configures sshd to answer on two ports — the standard one,
// completely unrestricted, for ordinary administrative access, and a second,
// borg-dedicated one, locked down by a Match block so that even a key this
// package never restricted could not do much on it. The real access control
// is still per-key (see EnsureKey's forced command below); this Match block
// is defense in depth, not the only thing standing between a connection and
// a shell.
const dropInTemplate = `# Managed by ngxborg — do not edit by hand; changes are overwritten by
# ` + "`ngxborg setup`" + `. Put local overrides in a separate file instead.
#
# Two ports, one sshd: administrative SSH stays on the standard port,
# completely unrestricted; borg backup traffic uses a second, dedicated
# port that this Match block hardens. There is no separate "borg server"
# process to run on its own port — borg has none; every connection here
# still goes through the very same sshd, it is just told to behave more
# defensively when it arrives on the borg port specifically.
Port %d
Port %d

Match LocalPort %d
    PasswordAuthentication no
    KbdInteractiveAuthentication no
    PermitTTY no
    X11Forwarding no
    AllowTcpForwarding no
    AllowAgentForwarding no
    AllowStreamLocalForwarding no
    Banner none
`

// EnsureDualPort writes the drop-in, validates the result with sshd's own
// syntax checker before ever touching the running daemon, and reloads it.
// adminPort is almost always 22; borgPort is whatever the operator chose at
// setup time.
//
// Validating before reloading matters more here than almost anywhere else
// this tool touches: sshd is how an operator gets back into this machine at
// all, and a config mistake here is not "the web UI is briefly down" the
// way a bad nginx reload is — it can be "nobody can SSH in until someone
// walks to the console."
//
// Ubuntu 22.04+ runs sshd under systemd socket activation by default
// (ssh.socket owns the listening socket on 22; ssh.service is handed
// already-bound file descriptors rather than binding anything itself).
// sshd's own Port directive is silently meaningless in that mode — it
// binds nothing, because it never gets the chance to — so a second Port
// line here would produce a config that validates cleanly and changes
// nothing. disableSocketActivation switches the host to the traditional
// model, where sshd binds its own ports directly and Port genuinely takes
// effect, which is what essentially every "sshd on more than one port"
// guide assumes.
func EnsureDualPort(ctx context.Context, r system.Runner, adminPort, borgPort int) error {
	if adminPort == borgPort {
		return fmt.Errorf("the borg port (%d) must differ from the admin SSH port (%d)", borgPort, adminPort)
	}
	body := fmt.Sprintf(dropInTemplate, adminPort, borgPort, borgPort)

	if r.DryRun {
		return nil
	}
	if err := EnsurePrivilegeSeparationDir(); err != nil {
		return fmt.Errorf("creating /run/sshd: %w", err)
	}
	if err := atomicWrite(DropInPath, body, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", DropInPath, err)
	}
	if err := allowFirewallPort(ctx, r, borgPort); err != nil {
		return err
	}
	if err := r.Run(ctx, "sshd", "-t"); err != nil {
		return fmt.Errorf("sshd rejected the new configuration (left un-reloaded, the old config is still active): %w", err)
	}
	if _, err := disableSocketActivation(ctx, r); err != nil {
		return err
	}
	// A reload (SIGHUP) is not enough here even once socket activation is
	// out of the way: sshd only calls bind() on its configured ports when
	// it starts, never in response to a reload — a newly added Port is
	// invisible to an already-running process no matter how it gets told
	// to re-read its config. Confirmed the hard way: a first attempt at
	// this used reload-or-restart, sshd -t validated cleanly, doctor
	// reported everything correct, and the port still never opened,
	// because the already-running process from before this config existed
	// was still the one serving every connection. Only a genuine restart
	// starts a fresh process that binds everything the current config
	// asks for. This restart is safe for whoever is already connected —
	// see disableSocketActivation's doc comment — and unconditional,
	// rather than skipped when the drop-in's content did not change,
	// because "the file matches" and "sshd is actually listening on the
	// port that file names" are two different facts, and only the second
	// one is the thing worth being correct about.
	name := "ssh.service"
	if !system.UnitExists(ctx, r, name) {
		name = "sshd.service"
	}
	if err := r.Run(ctx, "systemctl", "restart", name); err != nil {
		return fmt.Errorf("restarting %s: %w", name, err)
	}
	return nil
}

// EnsurePrivilegeSeparationDir makes sure /run/sshd exists before sshd is
// ever invoked — by this package, or by doctor's own independent `sshd -t`
// check (internal/provision/doctor.go calls this too, for the same
// reason). On a normally-booted host it already exists — systemd-tmpfiles
// creates it at boot from a rule openssh-server ships — but /run is
// tmpfs, cleared every boot, and that rule only runs at boot time. A host
// that installs openssh-server without an intervening reboot (exactly
// what `apt-get install` during `ngxborg setup` does) can reach this
// point before /run/sshd has ever been created. Confirmed live: even
// `sshd -t`, a pure syntax check with nothing to actually bind or
// privilege-separate, refuses to run at all without it
// ("Missing privilege separation directory: /run/sshd") — so this has to
// happen before the very first sshd invocation, not just before starting
// the service. Idempotent and harmless to call unconditionally.
func EnsurePrivilegeSeparationDir() error {
	return os.MkdirAll("/run/sshd", 0o755)
}

// allowFirewallPort opens the borg port through ufw, when ufw is the
// firewall in use and active — a host provisioned by ngxsetup (or hardened
// by hand the same way) allows only 22/80/443 by default, and a correctly
// bound, correctly restricted sshd listening on a port the firewall still
// drops would be a confusing "it works locally but no borg client can ever
// reach it" failure with no obvious cause. A host with no ufw, or ufw not
// active, is left entirely alone — this never enables or configures ufw
// itself, only ever adds one rule to an already-active one.
func allowFirewallPort(ctx context.Context, r system.Runner, port int) error {
	if !r.Look("ufw") {
		return nil
	}
	out, err := r.Output(ctx, "ufw", "status")
	if err != nil || !strings.Contains(out, "Status: active") {
		return nil
	}
	if strings.Contains(out, fmt.Sprintf("%d/tcp", port)) {
		return nil // already allowed
	}
	if err := r.Run(ctx, "ufw", "allow", fmt.Sprintf("%d/tcp", port), "comment", "ngxborg"); err != nil {
		return fmt.Errorf("allowing port %d through ufw: %w", port, err)
	}
	return nil
}

// disableSocketActivation switches a socket-activated sshd to bind its own
// ports directly, a one-time, idempotent change, returning whether it
// actually did anything. The already-open connection running this very
// command is unaffected either way: a restart kills sshd's master process,
// not the already-forked child handling this session (that is precisely
// why "restart sshd without dropping your current SSH session" has always
// been ordinary, routine practice) — and closing/reopening a *listening*
// socket never touches already-established TCP connections regardless,
// only the brief window for new ones during the switch.
//
// A plain `enable --now` is not enough on its own: socket activation keeps
// a single long-running ssh.service instance already active, so `--now`
// is a no-op there (the unit is already running) — that already-running
// process is still the one that was handed its listening socket back when
// systemd originally activated it, and neither a reload nor a bare
// `enable --now` makes an already-running process rebind a socket it never
// owned in the first place. Only a genuine restart, once the socket unit
// is out of the way, starts a fresh process that binds every configured
// Port itself.
func disableSocketActivation(ctx context.Context, r system.Runner) (switched bool, err error) {
	if !system.UnitExists(ctx, r, "ssh.socket") || !system.IsActive(ctx, r, "ssh.socket") {
		return false, nil // already running in the traditional, direct-bind mode
	}
	if err := r.Run(ctx, "systemctl", "disable", "--now", "ssh.socket"); err != nil {
		return false, fmt.Errorf("disabling ssh.socket: %w", err)
	}
	if err := r.Run(ctx, "systemctl", "enable", "ssh.service"); err != nil {
		return false, fmt.Errorf("enabling ssh.service: %w", err)
	}
	if err := r.Run(ctx, "systemctl", "restart", "ssh.service"); err != nil {
		return false, fmt.Errorf("restarting ssh.service to bind directly: %w", err)
	}
	return true, nil
}

// BorgPort reads back the port EnsureDualPort last configured, by parsing
// the drop-in this package itself wrote — used by doctor and by the URL
// ngxborg prints for a tenant to give their borg client.
func BorgPort() (int, error) {
	raw, err := os.ReadFile(DropInPath)
	if err != nil {
		return 0, fmt.Errorf("reading %s (has `ngxborg setup` run?): %w", DropInPath, err)
	}
	var ports []int
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Port ") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Port")))
		if err == nil {
			ports = append(ports, n)
		}
	}
	if len(ports) < 2 {
		return 0, fmt.Errorf("%s does not declare two ports; run `ngxborg setup` again", DropInPath)
	}
	// The template always writes the admin port first, the borg port second.
	return ports[1], nil
}
