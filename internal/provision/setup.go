package provision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"ngxborg/internal/authpam"
	"ngxborg/internal/borgrepo"
	"ngxborg/internal/logx"
	"ngxborg/internal/posix"
	"ngxborg/internal/sshaccess"
	"ngxborg/internal/system"
)

// basePackages are needed on every ngxborg host.
var basePackages = []string{
	"borgbackup", // provides both the client tools and `borg serve`
	"openssh-server",
	"libpam-modules", // pam_unix.so — almost always already present, listed for completeness
}

// pamStackBody is the minimal, standard "check a real POSIX password"
// stack: pam_unix.so for both the password check and the account-validity
// check (locked/expired accounts refused). Nothing more — see
// internal/authpam's package doc comment for why more would be unusual for
// what this checks.
const pamStackBody = `# Managed by ngxborg — do not edit by hand; changes are overwritten by
# ` + "`ngxborg setup`" + `.
#
# Checks a real POSIX account's password and confirms the account itself
# is currently usable (not locked, not expired) — nothing more.
auth       required     pam_unix.so
account    required     pam_unix.so
`

// SetupOptions configures a fresh install. Both ports default to their
// conventional values when zero. TLS defaults to "self-signed".
type SetupOptions struct {
	AdminPort int // default 22
	BorgPort  int // default 2222
	TLSMode   string // "self-signed", "custom", or "none"
	TLSCert   string // path to TLS certificate (used when TLSMode == "custom")
	TLSKey    string // path to TLS private key (used when TLSMode == "custom")
}

func (o SetupOptions) adminPort() int {
	if o.AdminPort != 0 {
		return o.AdminPort
	}
	return 22
}
func (o SetupOptions) borgPort() int {
	if o.BorgPort != 0 {
		return o.BorgPort
	}
	return 2222
}

// Setup brings a bare machine up to a working ngxborg backup server: the
// web UI itself is deliberately not started here — see InstallService —
// so that `ngxborg setup` is safe to re-run at any time (an idempotent
// "make the machine correct" step) without it also being the thing that
// decides whether the web UI should be publicly reachable right now.
func (c *Ctx) Setup(opts SetupOptions) error {
	if err := c.preflight(); err != nil {
		return err
	}

	logx.Section("Installing packages")
	if err := system.AptUpdate(c.Context, c.Runner); err != nil {
		return err
	}
	if err := system.AptInstall(c.Context, c.Runner, basePackages...); err != nil {
		return err
	}

	logx.Section("Configuring accounts")
	if err := posix.EnsureGroups(c.Context, c.Runner); err != nil {
		return err
	}

	logx.Section("Configuring PAM")
	pamStackPath := "/etc/pam.d/" + authpam.ServiceName
	if err := writeIfChanged(pamStackPath, pamStackBody, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", pamStackPath, err)
	}

	logx.Section("Configuring storage")
	// Base, and every directory between it and the filesystem root that
	// this tool itself owns, need their execute ("traverse") bit open to
	// everyone: a tenant's own borg serve process runs as that tenant, not
	// root, and it has to walk through Base to reach a repository two
	// levels below it. A single directory anywhere in that chain missing
	// the execute bit for "other" blocks the whole path regardless of what
	// permissions the leaf repository directory itself has — confirmed
	// live, twice, the second time one level higher than the first: both
	// /var/lib/ngxborg/repos itself and its own parent /var/lib/ngxborg
	// (created implicitly by the first MkdirAll call the very first time
	// this ran, with no awareness that anything but root would ever need
	// to reach it) needed the same fix. 0711 grants execute-only —
	// traverse, not list — so a tenant can reach their own subdirectory
	// but still cannot list either directory's contents to see what other
	// tenants exist.
	//
	// TrashBase never needs this: only root ever touches it (Delete/Purge
	// run as root, at the CLI/web UI level, not as a connecting tenant),
	// and it shares Base's own parent, which the loop below already opens.
	for _, dir := range []string{filepath.Dir(borgrepo.Base), borgrepo.Base} {
		if err := os.MkdirAll(dir, 0o711); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o711); err != nil {
			// MkdirAll only applies the mode when it creates the
			// directory; one that already existed from before this fix
			// needs its mode corrected explicitly, every time setup runs,
			// not just the first.
			return fmt.Errorf("setting permissions on %s: %w", dir, err)
		}
	}
	for _, dir := range []string{borgrepo.TrashBase, "/etc/ngxborg"} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	logx.Section("Configuring SSH")
	if err := sshaccess.EnsureDualPort(c.Context, c.Runner, opts.adminPort(), opts.borgPort()); err != nil {
		return err
	}
	logx.Change("sshd listens on %d (admin) and %d (borg, restricted)", opts.adminPort(), opts.borgPort())

	logx.Section("Installing ngxborg")
	if err := c.installSelf(); err != nil {
		logx.Warn("could not install ngxborg to /usr/local/bin: %v", err)
	}
	if err := c.writeWebUnitTLS(":8443", opts.TLSMode, opts.TLSCert, opts.TLSKey); err != nil {
		return err
	}

	logx.Section("Done")
	logx.Info("run `ngxborg install service` to start the web UI, or `ngxborg user create <name>` to add your first tenant.")
	return nil
}

// installSelf copies the currently running binary to /usr/local/bin, the
// same self-install pattern ngxsetup uses: an operator who ran `ngxborg
// setup` from a downloaded binary in /tmp gets a permanent copy on PATH
// without a separate install step.
func (c *Ctx) installSelf() error {
	if c.Runner.DryRun {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	dest := "/usr/local/bin/ngxborg"
	if self == dest {
		return nil
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest+".tmp", data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(dest+".tmp", dest); err != nil {
		return err
	}
	logx.Change("installed ngxborg to %s", dest)
	return nil
}

func writeIfChanged(path, body string, mode os.FileMode) error {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == body {
		logx.Skip("%s already up to date", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	logx.Change("wrote %s", path)
	return nil
}

// RequireSetup is a light guard other commands can call to fail with a
// clear message rather than a confusing lower-level error (a missing
// group, an absent directory) when `ngxborg setup` has never run.
func RequireSetup(ctx context.Context, r system.Runner) error {
	if _, err := os.Stat(borgrepo.Base); err != nil {
		return fmt.Errorf("ngxborg has not been set up on this host yet; run `ngxborg setup` first")
	}
	return nil
}
