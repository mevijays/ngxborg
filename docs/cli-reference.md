# CLI reference

`ngxborg help` (or running with no arguments) always prints the current,
authoritative version of this reference from the binary itself. What
follows is the same content with worked examples added.

!!! warning "Flags come before positional arguments"
    This is a consequence of Go's standard `flag` package, which stops
    scanning for flags at the first non-flag argument:
    `ngxborg repo create --tenant alice mybackup` works;
    `ngxborg repo create mybackup --tenant alice` silently ignores
    `--tenant` instead of erroring.

Commands that touch other accounts, `sshd`, or PAM need root (`sudo`). A
tenant runs everything else as themselves, scoped automatically to their
own account — no `--tenant` override is possible for a non-admin caller.

## Setup

### `ngxborg setup`

```
ngxborg setup [--admin-port 22] [--borg-port 2222] [--dry-run]
```

Installs packages, creates the tenant/admin groups, wires up PAM,
configures `sshd`'s dual-port listener. Safe to re-run at any time — see
[Installation](installation.md#set-up-the-host).

### `ngxborg install service`

```
ngxborg install service [--addr :8443]
```

Enables and starts the web UI as a systemd service (`ngxborg-web.service`).

### `ngxborg uninstall service`

```
ngxborg uninstall service
```

Stops and disables the web UI. Accounts and repositories are untouched.

## Users

### `ngxborg user create`

```
ngxborg user create [--admin] <username>
```

Creates a real POSIX account, locked (no password) by default. Add
`--admin` for cross-tenant visibility.

```bash
sudo ngxborg user create alice
sudo ngxborg user create --admin ops
```

### `ngxborg user delete`

```
ngxborg user delete <username>
```

### `ngxborg user list`

```
ngxborg user list
```

### `ngxborg user passwd`

```
ngxborg user passwd [--generate] [username]
```

Sets or resets a login password. A tenant running this with no argument
sets their own; an admin must name whose password to change. `--generate`
produces a strong random password instead of prompting, printed once.

### `ngxborg user disable` / `ngxborg user enable`

```
ngxborg user disable <username>
ngxborg user enable <username>
```

Locks out the web UI *and* every SSH key on the account at once, without
touching the password, keys, or repositories. Reversible. See
[Security model](security.md#reversible-disable-not-deletion) for how.

### `ngxborg user key add`

```
ngxborg user key add [--tenant <name>] [--append-only] <repo> <pubkey-or-@file>
```

Registers a public key, restricted to exactly one repository. Accepts the
key material directly or `@/path/to/file.pub`. `--append-only` restricts
the key to append-only Borg operations (no archive deletion), useful for
a client you don't fully trust with pruning.

```bash
sudo ngxborg user key add --tenant alice websites "$(cat alice.pub)"
sudo ngxborg user key add --tenant alice websites @/tmp/alice.pub
```

### `ngxborg user key list`

```
ngxborg user key list [--tenant <name>]
```

### `ngxborg user key remove`

```
ngxborg user key remove [--tenant <name>] <key-material>
```

## Repositories

### `ngxborg repo create`

```
ngxborg repo create [--tenant <name>] <repo>
```

Reserves a repository directory. Does **not** run `borg init` — see
[Architecture → Repository lifecycle](architecture.md#repository-lifecycle).

### `ngxborg repo list`

```
ngxborg repo list [--tenant <name>]
```

An admin with no `--tenant` lists every tenant's repositories.

### `ngxborg repo delete`

```
ngxborg repo delete [--tenant <name>] <repo>
```

Soft delete — recoverable until purged.

### `ngxborg repo purge`

```
ngxborg repo purge [--tenant <name>] [--yes] <repo>
```

Permanent, irreversible removal of an already-deleted repository.

### `ngxborg repo disable` / `ngxborg repo enable`

```
ngxborg repo disable [--tenant <name>] <repo>
ngxborg repo enable [--tenant <name>] <repo>
```

Blocks every SSH key restricted to this repository, without removing the
keys or the data. Reversible.

## Diagnostics

### `ngxborg doctor`

```
ngxborg doctor
```

Runs a checklist against the live host — groups, PAM stack, package
installation, `sshd` configuration, storage, the web UI service, and disk
space — and reports failures/warnings alongside passes.

### `ngxborg version`

```
ngxborg version
```

Prints the running binary's version, maintainer, and repository URL.
