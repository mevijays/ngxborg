# Architecture

## The core idea

A tenant in ngxborg is not a row in a database — it's a real POSIX account.
Everything else follows from that one decision:

- **Authentication** is PAM, against that account's real password
  (`internal/authpam`), the same mechanism `login` and `sshd` use.
- **Authorization scope** (tenant vs. admin) is POSIX group membership —
  `ngxborg` for tenants, `ngxborg-admin` for admins — checked identically
  by the CLI (via the real process UID) and the web UI (via the
  authenticated PAM session).
- **The backup transport is SSH key auth against that same account**,
  restricted per key to one repository — no separate credential store for
  that either.

There is no ngxborg-specific user table anywhere. Deleting a tenant with
`ngxborg user delete` removes a real Unix account; nothing is "soft"
about identity the way repositories are (see
[Repository lifecycle](#repository-lifecycle) below).

## Package map

| Package | Responsibility |
|---|---|
| `internal/posix` | Create/delete/disable/enable POSIX tenant and admin accounts; group membership. |
| `internal/authpam` | cgo binding to libpam — the only place this project talks to PAM directly. |
| `internal/sshaccess` | The dual-port `sshd` configuration and per-key forced-command `authorized_keys` entries. |
| `internal/borgrepo` | Repository directory lifecycle: create, list, soft-delete, purge, disable/enable. Never runs `borg init` itself. |
| `internal/provision` | Orchestrates the packages above into `setup`, `install service`, and `doctor`. |
| `internal/cli` | Command-line interface — thin wrapper over `provision`, scoped by the real process UID. |
| `internal/webui` | Embedded, self-signed-TLS web UI — same operations as the CLI, scoped by an authenticated PAM session. |
| `internal/facts` | Host detection (OS family, disk space) used by `doctor`. |
| `internal/system` | Small process/package-manager helpers (`apt`, running a command, generating a random secret) shared by everything above. |

## The dual SSH port design

`ngxborg setup` configures the host's own `sshd` — the same daemon, not a
second server — to listen on two ports:

- The **admin port** (default `22`) is completely unchanged: ordinary
  interactive/administrative SSH access, whatever was already configured.
- The **Borg port** (default `2222`) accepts only key-based auth, and
  every key registered on it carries a **forced command**
  (`command="..."` in `authorized_keys`) restricting that connection to
  `borg serve --restrict-to-path <one specific repository>` — nothing
  else that key's holder sends is ever executed.

See [Security model](security.md) for exactly what that restriction does
and does not protect against.

## Storage layout

```
/var/lib/ngxborg/
├── repos/
│   └── <tenant>/
│       └── <repo-name>/       ← one tenant-owned directory per repository
└── .trash/
    └── <tenant>/
        └── <repo-name>/       ← soft-deleted repositories land here
```

Every directory ngxborg creates between `/var/lib/ngxborg` and a leaf
repository is `0711` (traverse-only) — a tenant's own `borg serve`
process (running as that tenant, not root) needs to walk through those
ancestor directories to reach its own repository, but must not be able to
list their contents and see what other tenants exist.

`/etc/ngxborg` holds host-level configuration `setup` writes (currently
just directory scaffolding for future use); nothing tenant-identifying
lives there.

## Repository lifecycle

`ngxborg repo create` **only reserves a directory** — correctly owned,
correctly permissioned. It deliberately never runs `borg init`: that
stays entirely client-side, matching Borg's own architecture, so a
repository's encryption passphrase never crosses the network to this
server at all. The server's job is limited to *which key may reach which
path*.

- `repo delete` moves a repository to `/var/lib/ngxborg/.trash` —
  recoverable.
- `repo purge` permanently removes a trashed repository. Irreversible.
- `repo disable` / `repo enable` toggle a repository directory's
  permissions to/from `0000`, blocking (or restoring) every key scoped
  to it without touching the keys or the data underneath — see
  [Security model](security.md#reversible-disable-not-deletion).

## The web UI

`internal/webui` embeds its entire frontend (self-hosted Tailwind CSS,
Font Awesome, and Chart.js — no CDN dependencies) into the binary and
serves it over self-signed TLS by default. It exposes the same operations
as the CLI — repository and key management, tenant/admin user management,
disable/enable — scoped by whichever account a PAM login authenticated
as. An admin session can act on any tenant; a tenant session can only
ever act on its own account, enforced server-side on every request, not
just hidden in the UI.

## What ngxborg deliberately does not do

- It does not implement its own encryption, deduplication, or archive
  format — that's entirely Borg's, unmodified.
- It does not run a scheduler. Recurring backups are the client's
  responsibility (cron, systemd timers, or — if the client is
  [ngxsetup](https://github.com/mevijays/ngxsetup) — its own built-in
  scheduling).
- It does not proxy or inspect backup traffic. Once a key authenticates
  and passes its path restriction, Borg's own wire protocol runs
  end-to-end between the client and `borg serve`.
