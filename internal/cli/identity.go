package cli

import (
	"fmt"
	"os/user"
	"strconv"

	"ngxborg/internal/posix"
)

// identity is who the CLI is acting as, resolved from the real process uid
// — not PAM, not a session, not a flag. This is deliberately a different
// mechanism from the web UI's own scoping (internal/webui checks whichever
// account a PAM login authenticated as): here, the question is "who can
// execute code as this uid on this box right now", the same trust boundary
// every other root-requiring Unix tool relies on. Running via sudo makes
// this "root" regardless of who typed the sudo command, which is exactly
// the traditional Unix answer to "who is this" for a privileged operation
// — the CLI does not attempt to peel back sudo to find the original account
// (SUDO_USER can be spoofed by anything that can set environment variables,
// so it is not a trust boundary either).
type identity struct {
	Admin    bool
	Username string // "" when Admin, since root is not itself a tenant
}

// resolveIdentity determines whether the invoking process may act for
// every tenant (root — admin scope) or only for itself (a specific,
// registered tenant account — its own scope, no `--tenant` override
// possible). Anything else — an ordinary shell account that was never
// registered with `ngxborg user create` — is refused outright: a valid
// login on the box is not, by itself, ngxborg access.
func resolveIdentity() (identity, error) {
	u, err := user.Current()
	if err != nil {
		return identity{}, fmt.Errorf("determining the current user: %w", err)
	}
	if u.Uid == "0" {
		return identity{Admin: true}, nil
	}
	if !posix.IsTenant(u.Username) {
		return identity{}, fmt.Errorf("%s is not a registered ngxborg account (see `ngxborg user create`); admin operations need sudo", u.Username)
	}
	return identity{Username: u.Username}, nil
}

// scopeTenant resolves which tenant a command should act on, given the
// invoking identity and an optional `--tenant` flag value. A tenant's own
// CLI session ignores (and, if it disagrees, rejects) any attempt to name
// someone else — the whole point of self-scoping is that it cannot be
// overridden by a flag.
func scopeTenant(id identity, flagValue string) (string, error) {
	if !id.Admin {
		if flagValue != "" && flagValue != id.Username {
			return "", fmt.Errorf("you are logged in as %s and can only manage your own account/repositories", id.Username)
		}
		return id.Username, nil
	}
	if flagValue == "" {
		return "", fmt.Errorf("--tenant is required when running as admin")
	}
	return flagValue, nil
}

// mustAtoi parses a required integer flag/argument, returning a clear error
// rather than Atoi's bare "invalid syntax" for the common case of an
// operator fat-fingering a port number.
func mustAtoi(field, s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", field, s)
	}
	return n, nil
}
