# Security model

## Authentication

Both the CLI and the web UI ultimately trust the same thing: a real POSIX
account's own credentials.

- **CLI**: identity is the real process UID. UID 0 (root/`sudo`) is
  treated as admin scope; any other UID must belong to a registered
  tenant account (`ngxborg user create`), or the command is refused
  outright — an ordinary shell login that was never registered with
  ngxborg is not, by itself, ngxborg access.
- **Web UI**: a login POSTs a username/password to PAM
  (`internal/authpam`, a self-contained cgo binding — no third-party PAM
  module, no separate password store). Success starts a server-side
  session; admin scope is `ngxborg-admin` group membership at that
  moment, not something the client can claim.
- **Backup transport**: SSH public-key authentication against the
  tenant's own account, on the dedicated Borg port. See below.

## The dual SSH port design

`ngxborg setup` configures a second `sshd` listener (default port
`2222`), separate from the ordinary admin SSH port (default `22`), with
its own `Match LocalPort` block. Ordinary interactive SSH access is
completely unaffected by anything on this port; the Borg port accepts
**only** key-based auth, and every accepted connection is forced into
running exactly one command.

## Per-key path restriction

Registering a key (`ngxborg user key add`) writes an `authorized_keys`
line shaped like:

```
command="/usr/bin/borg serve --restrict-to-path /var/lib/ngxborg/repos/alice/websites",restrict ssh-ed25519 AAAA... alice@websites
```

`command=` is OpenSSH's forced-command mechanism: whatever the connecting
client actually asks to run is ignored, and this exact command runs
instead — the key's holder cannot choose to run something else through
this port. `--restrict-to-path` is Borg's own flag limiting `borg serve`
to one path, regardless of what repository URL the client requests. The
`restrict` keyword additionally disables X11/agent/port forwarding for
that line.

Two consequences worth being explicit about:

- **A key is scoped to one repository, not to a tenant.** Give a tenant
  two repositories and they'll have two separately-registered keys (or
  one key registered twice, restricted differently each time) — one
  key's compromise does not expose the other repository.
- **Cross-tenant reach is impossible by construction, not by policy.**
  A tenant's key only ever appears in *that tenant's own*
  `authorized_keys` file. There is no shared keyring, no lookup table
  mapping keys to repositories across accounts — `sshd` simply never
  offers another account's forced command to a key it was never told
  about.

## Reversible disable, not deletion

Two independent disable/enable controls exist, deliberately using
different underlying mechanisms because they close different doors:

**`ngxborg repo disable <repo>`** — `chmod 0000` on the repository
directory. A tenant's own `borg serve` process (running as that tenant,
not root) can no longer even `stat()` the directory, so every operation
against it fails immediately. Root-run admin operations are unaffected —
Unix permission checks don't apply to root — so an admin can still
inspect or purge a disabled repository. `enable` restores `0700`.

**`ngxborg user disable <username>`** — `usermod --expiredate 1` (an
account expiry date far in the past), not a password lock.
`passwd --lock` would only block *password* authentication; sshd's
account-expiry check fires regardless of auth method, including the
pubkey/forced-command auth the Borg port uses — so this is what actually
stops a registered SSH key from working, not just web UI login.
`enable` clears the expiry date (`usermod --expiredate ""`).

Neither disable operation touches the underlying password, keys, or
repository data — both are meant to be flipped back at any time.

## What this does *not* protect against

Being direct about the boundaries:

- **A compromised repository-scoped key can still delete or corrupt
  that repository's own archives** (unless registered `--append-only`).
  Scope keys narrowly and consider `--append-only` for clients you don't
  fully trust with pruning.
- **ngxborg does not encrypt anything itself.** Encryption is entirely
  Borg's (`repokey`/`keyfile`, your choice at `borg init`) — a repository
  initialized without encryption is not encrypted, regardless of
  anything ngxborg does.
- **Root on the ngxborg host can read/modify anything.** This is the
  ordinary Unix trust boundary, not a gap specific to ngxborg — the same
  is true of any server you don't control the root account on.
- **This is a transport and access-control layer, not a backup
  verification tool.** ngxborg does not check archive integrity beyond
  what Borg itself does; `borg check` is still your responsibility.

## MCP endpoint security

The `/mcp` endpoint is **anonymous** — it does not require PAM
authentication. This is intentional: agents connecting from external
systems (GitHub Actions, CI/CD pipelines, local LLM tools) cannot
authenticate via PAM, and the endpoint is only reachable on your internal
network or behind a reverse proxy with its own access control.

**All MCP tools are effectively admin-level operations.** The
`ngxborg-admin` group membership check that gates admin endpoints in
the web UI does **not** apply to MCP tools. Any caller with network
access to `/mcp` can create users, delete repositories, and run
diagnostics.

Security recommendations:

- **Network isolation**: Ensure the ngxborg port is only reachable from
  trusted networks. Use firewall rules or a reverse proxy with IP
  allowlisting.
- **TLS**: If running with `--tls none` (plain HTTP), all MCP traffic
  is unencrypted. Use only on trusted internal networks or behind a
  TLS-terminating reverse proxy.
- **Audit**: MCP tool calls are not logged separately from web UI access
  logs. For compliance requirements, implement reverse proxy logging or
  a dedicated audit layer.

## Reporting a vulnerability

Please open an issue at
[github.com/mevijays/ngxborg/issues](https://github.com/mevijays/ngxborg/issues).
For anything you'd rather not disclose publicly before a fix ships,
mention that in the issue and a maintainer will follow up privately.
