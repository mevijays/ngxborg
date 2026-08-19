# ngxborg

**ngxborg** turns a bare Debian/Ubuntu host into a multi-tenant [BorgBackup](https://www.borgbackup.org/)
server, with no separate user database, no bespoke authentication system,
and no agent to install on the client.

```bash
sudo ngxborg setup
sudo ngxborg user create alice
sudo ngxborg repo create --tenant alice websites
```

From there, `alice` (a real POSIX account) can `borg init` and `borg backup`
straight over SSH — no manual `authorized_keys` editing, no shared secrets.

## Why

Most self-hosted Borg servers fall into one of two camps: a single shared
Unix account every client SSHes into (no isolation between clients at all),
or a heavyweight system with its own user database, its own auth, and its
own agent. ngxborg takes a third path:

- **Every tenant *is* a real POSIX account.** There is no ngxborg-specific
  user table. Web UI login is real PAM authentication
  (`internal/authpam`) against that same account's password — the same
  mechanism `login` and `sshd` use.
- **One `sshd`, configured correctly, is the whole backup transport.**
  A second, dedicated SSH port carries only forced-command,
  path-restricted `borg serve` traffic — see [Security model](security.md).
  There is no separate "borg server" daemon.
- **The server never runs `borg init` on a client's behalf.** Key
  generation and repository initialization stay client-side, exactly
  matching Borg's own architecture: your passphrase never crosses the
  network, and ngxborg's job is limited to reserving a correctly owned,
  correctly permissioned directory and enforcing which key may reach it.
- **Admin vs. tenant scope is POSIX group membership**, enforced
  identically by the CLI (via the real process UID) and the web UI (via
  the authenticated PAM session) — one rule, two surfaces.

## What you get

- A CLI (`ngxborg`) for setup, user/tenant management, repository
  lifecycle, and diagnostics.
- A small, self-hosted web UI (embedded in the binary, TLS by default)
  for the same operations, usable by tenants and admins alike.
- Per-repository disable/enable (locks out every key scoped to it,
  reversibly) and per-tenant disable/enable (locks out the web UI *and*
  every SSH key on the account at once), independent of each other.
- A one-click panel of exact, ready-to-run client commands per
  repository — see [Web UI guide](web-ui.md#client-commands).

## Where to go next

- New to ngxborg? Start with [Installation](installation.md), then
  [Getting started](getting-started.md).
- Want to understand *why* it's built this way before running it?
  Read [Architecture](architecture.md) and [Security model](security.md).
- Pointing [ngxsetup](https://github.com/mevijays/ngxsetup)'s own Borg
  backup feature at an ngxborg server? See
  [Pairing with ngxsetup](ngxsetup-integration.md).
- Something not working? Check [Troubleshooting](troubleshooting.md)
  before filing an issue — it covers the sharp edges found the hard way.

## License

ngxborg is [MIT licensed](https://github.com/mevijays/ngxborg/blob/main/LICENSE).
Contributions are welcome — see [Contributing](contributing.md).
