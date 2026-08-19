// Package posix manages the real system accounts that are ngxborg's tenants.
//
// This is the load-bearing design choice the whole application rests on:
// there is no separate ngxborg user database. A tenant IS a POSIX account.
// Their web UI password IS their system password, checked by PAM the same
// way `login` or `sshd` would. Their admin/tenant scope IS their membership
// in one of two POSIX groups this package creates at setup time. This
// avoids the failure mode every bolted-on auth database eventually hits —
// drifting out of sync with the accounts it is supposed to describe — by
// having only one place accounts live at all.
package posix

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"time"

	"ngxborg/internal/system"
)

// AdminGroup and TenantGroup are created once by `ngxborg setup`. Every
// ngxborg account is a member of TenantGroup; an account is additionally an
// admin — full visibility across every tenant's repos and users — iff it is
// also a member of AdminGroup. A plain `useradd` account that was never
// registered through ngxborg is not a member of either group and this
// package will not recognise it as a tenant at all, even if PAM would happily
// authenticate its password: existing shell accounts on the box (an
// operator's own login, a monitoring service account) must never accidentally
// gain backup access just by having a valid password.
const (
	TenantGroup = "ngxborg"
	AdminGroup  = "ngxborg-admin"
)

// EnsureGroups creates the two POSIX groups this package's whole
// authorization model depends on. Idempotent.
func EnsureGroups(ctx context.Context, r system.Runner) error {
	for _, g := range []string{TenantGroup, AdminGroup} {
		if _, err := user.LookupGroup(g); err == nil {
			continue
		}
		if err := r.Run(ctx, "groupadd", "--system", g); err != nil {
			return fmt.Errorf("creating group %s: %w", g, err)
		}
	}
	return nil
}

// Exists reports whether a username is a real system account.
func Exists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

// InGroup reports whether a username belongs to a named group — the primitive
// IsTenant and IsAdmin are both built from.
func InGroup(username, group string) bool {
	u, err := user.Lookup(username)
	if err != nil {
		return false
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return false
	}
	if u.Gid == g.Gid {
		return true
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, id := range gids {
		if id == g.Gid {
			return true
		}
	}
	return false
}

// IsTenant reports whether username is a registered ngxborg account —
// present on the system AND a member of TenantGroup. Presence alone is not
// enough: see the package doc comment.
func IsTenant(username string) bool { return Exists(username) && InGroup(username, TenantGroup) }

// IsAdmin reports whether username has full cross-tenant visibility. An
// admin is necessarily also a tenant (EnsureUser always adds both groups
// together for an admin account), but this is checked independently rather
// than assumed, so a group membership hand-edited by an operator is still
// honoured correctly either way.
func IsAdmin(username string) bool { return Exists(username) && InGroup(username, AdminGroup) }

// HomeDir returns a user's home directory, needed to locate
// ~/.ssh/authorized_keys.
func HomeDir(username string) (string, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("looking up %s: %w", username, err)
	}
	return u.HomeDir, nil
}

// UIDGID returns a user's numeric uid/gid, used to drop privileges before
// running `borg init` so a tenant's repository files are owned by the
// tenant, not by whatever process (root, typically) created them.
func UIDGID(username string) (uid, gid uint32, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, 0, fmt.Errorf("looking up %s: %w", username, err)
	}
	var uid64, gid64 uint64
	if _, err := fmt.Sscanf(u.Uid, "%d", &uid64); err != nil {
		return 0, 0, fmt.Errorf("parsing uid %q for %s: %w", u.Uid, username, err)
	}
	if _, err := fmt.Sscanf(u.Gid, "%d", &gid64); err != nil {
		return 0, 0, fmt.Errorf("parsing gid %q for %s: %w", u.Gid, username, err)
	}
	return uint32(uid64), uint32(gid64), nil
}

// CreateUser provisions a new tenant account: a real home directory (so
// ~/.ssh/authorized_keys has somewhere to live), a normal login shell (an
// operator may legitimately want to SSH in directly, e.g. to run `ngxborg
// repo list` themselves — the *backup* transport is separately locked down
// per SSH key via a forced command, not by taking the account's shell away;
// see internal/sshaccess), and membership in TenantGroup, plus AdminGroup
// too when admin is true. No password is set here — the account is locked
// (no valid password hash at all) until SetPassword runs, so a freshly
// created tenant cannot be authenticated as by anyone, including via an
// empty or default password, before an operator or the tenant themselves
// sets one.
func CreateUser(ctx context.Context, r system.Runner, username string, admin bool) error {
	if Exists(username) {
		return fmt.Errorf("a system account named %q already exists", username)
	}
	if err := r.Run(ctx, "useradd",
		"--create-home",
		"--shell", "/bin/bash",
		"--comment", "ngxborg tenant",
		username,
	); err != nil {
		return fmt.Errorf("creating account %s: %w", username, err)
	}
	if err := r.Run(ctx, "usermod", "-aG", TenantGroup, username); err != nil {
		return fmt.Errorf("adding %s to %s: %w", username, TenantGroup, err)
	}
	if admin {
		if err := r.Run(ctx, "usermod", "-aG", AdminGroup, username); err != nil {
			return fmt.Errorf("adding %s to %s: %w", username, AdminGroup, err)
		}
	}
	// Locked, not merely empty: an empty password field in /etc/shadow can
	// mean "no password required" to some PAM configurations, which is the
	// exact opposite of what a freshly created, not-yet-claimed account
	// should mean.
	r.TryRun(ctx, "passwd", "--lock", username)
	return nil
}

// DeleteUser removes a tenant account. Their repositories are deliberately
// untouched — deleting the account that manages a repository must never be
// how the backups it protects get destroyed; use internal/borgrepo's own
// Delete/Purge for that, as an explicit, separate, irreversible step.
func DeleteUser(ctx context.Context, r system.Runner, username string) error {
	if !Exists(username) {
		return fmt.Errorf("no such account %q", username)
	}
	return r.Run(ctx, "userdel", "-r", username)
}

// SetPassword sets or resets a tenant's login password — the same password
// PAM will check on the web UI, and, if PasswordAuthentication is enabled,
// on SSH's standard port too (never on the borg-dedicated port; see
// internal/sshaccess). Piped over stdin in chpasswd's own "user:password"
// form, never as a command-line argument, so it never appears in `ps` output
// or this process's own argv.
func SetPassword(ctx context.Context, r system.Runner, username, password string) error {
	if !Exists(username) {
		return fmt.Errorf("no such account %q", username)
	}
	if strings.ContainsAny(password, "\n:") {
		return fmt.Errorf("password must not contain a newline or colon")
	}
	if _, err := r.RunStdin(ctx, username+":"+password+"\n", "chpasswd"); err != nil {
		return fmt.Errorf("setting password for %s: %w", username, err)
	}
	return nil
}

// Disable locks a tenant out of everything — the web UI and SSH/borg
// access alike — without touching their password, keys, or repositories,
// so re-enabling is instant and lossless. This is deliberately not the
// same as SetPassword locking a fresh account's password hash (which only
// blocks password authentication): a disabled tenant's SSH key would still
// work fine against a locked-password account, since key-based auth never
// touches the password hash at all. Setting the account's shadow(5)
// expiry into the past is what actually closes both doors — sshd itself
// refuses an expired account before running any command (a forced borg
// serve included), regardless of auth method, and PAM's own account phase
// (pam_acct_mgmt, which authpam.Authenticate calls) rejects it the same
// way for the web UI.
func Disable(ctx context.Context, r system.Runner, username string) error {
	if !Exists(username) {
		return fmt.Errorf("no such account %q", username)
	}
	// Epoch day 1 (1970-01-02): any date already in the past works, this
	// one is simply unambiguous and never plausible as a real expiry an
	// operator meant to set some other way.
	if err := r.Run(ctx, "usermod", "--expiredate", "1", username); err != nil {
		return fmt.Errorf("disabling %s: %w", username, err)
	}
	return nil
}

// Enable reverses Disable by clearing the account's expiry date.
func Enable(ctx context.Context, r system.Runner, username string) error {
	if !Exists(username) {
		return fmt.Errorf("no such account %q", username)
	}
	if err := r.Run(ctx, "usermod", "--expiredate", "", username); err != nil {
		return fmt.Errorf("enabling %s: %w", username, err)
	}
	return nil
}

// IsDisabled reports whether Disable has been applied — the account's
// shadow(5) expiry date is set and in the past.
func IsDisabled(username string) (bool, error) {
	out, err := readShadowField(username, 7) // expire is the 8th colon-separated field
	if err != nil {
		return false, err
	}
	if out == "" {
		return false, nil
	}
	days, err := strconv.Atoi(out)
	if err != nil {
		return false, nil // an unparseable field is not this package's concern to flag as disabled
	}
	return int64(days) <= time.Now().Unix()/86400, nil
}

// ListTenants returns every registered ngxborg account, admins included.
func ListTenants() ([]string, error) {
	g, err := user.LookupGroup(TenantGroup)
	if err != nil {
		return nil, fmt.Errorf("looking up group %s (has `ngxborg setup` run?): %w", TenantGroup, err)
	}
	return groupMembers(g.Gid)
}

// groupMembers lists a group's members by scanning /etc/group directly.
// os/user has no group-membership-listing API (GroupIds only goes the other
// direction, user -> groups), and getent's output is exactly /etc/group's
// own format, so parsing that file is the standard, dependency-free way to
// answer "who is in this group" on Linux.
func groupMembers(gid string) ([]string, error) {
	groups, err := readGroupFile("/etc/group")
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.gid == gid {
			return g.members, nil
		}
	}
	return nil, nil
}
