# Getting started

This walks through creating a tenant, giving them a repository, and
running a real backup — the same sequence
[Web UI guide → Client commands](web-ui.md#client-commands) generates for
you automatically once a repository exists.

Everything here runs on the ngxborg **server**, except the `ssh-keygen`
and `borg` commands, which run on whatever machine you're backing up (the
**client**) — that can be the same box for a first try, or a completely
different one, as it will be in normal use.

## 1. Create a tenant

```bash
sudo ngxborg user create alice
```

This creates a real POSIX account named `alice`, locked (no password) by
default. A password is only needed for web UI login — an SSH-key-only
tenant that never touches the web UI doesn't need one at all. To set one
later:

```bash
sudo ngxborg user passwd --generate alice
```

## 2. Create a repository

```bash
sudo ngxborg repo create --tenant alice websites
```

This reserves a directory for `alice`'s exclusive use — it does **not**
run `borg init`. Repository initialization stays client-side, matching
Borg's own architecture: the passphrase never crosses the network to this
server at all.

## 3. Generate a dedicated client key

On the **client** machine:

```bash
ssh-keygen -t ed25519 -N "" -C "alice@websites" -f ~/.ssh/ngxborg_alice_websites
```

Give every repository its own key. It costs nothing and means a leaked or
retired client's access can be revoked (`ngxborg user key remove`)
without touching anything else that client does.

## 4. Register the public key

Back on the **server**:

```bash
sudo ngxborg user key add --tenant alice websites "$(cat ~/.ssh/ngxborg_alice_websites.pub)"
```

(If the key file lives on the server instead, you can pass
`@/path/to/key.pub` instead of pasting the contents.)

This key can now reach **only** this one repository — see
[Security model](security.md#per-key-path-restriction) for exactly how
that's enforced.

## 5. Initialize the repository

On the **client**, using the dedicated key:

```bash
export BORG_RSH="ssh -i ~/.ssh/ngxborg_alice_websites -o IdentitiesOnly=yes"
borg init --encryption=repokey-blake2 \
  ssh://alice@your-ngxborg-host:2222/var/lib/ngxborg/repos/alice/websites
```

Borg will ask for (or, non-interactively, read `BORG_PASSPHRASE` for) a
repository passphrase. **Write it down** — ngxborg never sees or stores
it, so there is nowhere else to recover it from.

## 6. Back up

```bash
borg create \
  ssh://alice@your-ngxborg-host:2222/var/lib/ngxborg/repos/alice/websites::'{hostname}-{now}' \
  /path/to/back/up
```

Run that again any time to add another archive — Borg deduplicates
automatically.

## Doing this without typing it yourself

The web UI's Repositories page has a **"commands"** button on every row
that generates steps 3–6 above with the real host, port, and path already
filled in — see [Web UI guide](web-ui.md#client-commands).

## Using ngxsetup instead of a manual client?

If the machine you're backing up runs
[ngxsetup](https://github.com/mevijays/ngxsetup), it manages its own
dedicated SSH key automatically — steps 3–4 above are handled for you,
and it prints the public key to register in step 4. See
[Pairing with ngxsetup](ngxsetup-integration.md).
