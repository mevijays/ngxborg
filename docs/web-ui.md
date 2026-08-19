# Web UI guide

Start it with `sudo ngxborg install service` (see
[Installation](installation.md#start-the-web-ui)), then visit
`https://your-host:8443/`. It's self-signed by default, so your browser
will warn about the certificate on first visit — that's expected unless
you've put a real certificate in front of it yourself.

Log in with a tenant or admin account's real POSIX password (set one with
`ngxborg user passwd` if you haven't yet — see
[CLI reference](cli-reference.md#ngxborg-user-passwd)).

## Dashboard

A quick-glance summary: repository count, total storage used, and a
per-tenant storage breakdown (admin view only — a tenant sees only their
own).

## Repositories

Every repository you (or, as an admin, any tenant) can see, with its
initialization status, size, and disabled/enabled state.

- **Create a repository** — reserves a directory; does not run
  `borg init`. Admins pick a tenant from a dropdown of real accounts,
  not free text.
- **disable / enable** — reversible; see
  [Security model](security.md#reversible-disable-not-deletion).
- **delete** — moves the repository to the trash (recoverable via the
  CLI's `repo purge`/restore path; the web UI does not yet expose purge
  directly — ask an admin to run it from the CLI if you need a
  repository permanently gone).

### Client commands

Every repository row has a **commands** button (also linked right after
creating a new repository) that opens a panel with the exact,
ready-to-paste commands for *that specific repository* — no placeholders,
no assembly required:

1. Generate a dedicated key, on the client.
2. Show its public half, to paste into **SSH Keys → Register a key**.
3. `borg init`, with the real `ssh://` URL already filled in.
4. `borg create`, ready to run against a real path.

...plus a separate note for pointing [ngxsetup](ngxsetup-integration.md)
at the same repository, since it manages its own key automatically and
only needs the URL.

The host and port in these commands are read from the running server
itself (`sshaccess.BorgPort()`, parsed back from the `sshd` configuration
`ngxborg setup` wrote) and from the address your browser is currently
using to reach it — not guesses, and not something you have to fill in
by hand.

## SSH Keys

Every registered key, which repository it's restricted to, and whether
it's append-only. Registering a new key uses cascading dropdowns: pick a
tenant (admin view), then pick one of *that tenant's actual repositories*
— never free text, so there's nothing to typo.

## Users *(admin only)*

Create, delete, and list tenant/admin accounts. Per-row actions:

- **set password** — blank generates a strong one, shown once.
- **disable / enable** — locks (or restores) the web UI and every SSH key
  on the account at once. Self-disabling an admin session prompts for
  confirmation and then signs you out, since the CLI remains available as
  a recovery path (`sudo ngxborg user enable <you>`).
- **delete** — removes the underlying POSIX account.

Every signed-in user (tenant or admin) also has a **change my password**
link in the sidebar, for their own account.

## Doctor *(admin only)*

The same checklist as `ngxborg doctor` on the CLI, rendered as a table —
useful for a quick health check without shelling in.
