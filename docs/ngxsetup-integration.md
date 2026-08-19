# Pairing with ngxsetup

[ngxsetup](https://github.com/mevijays/ngxsetup) is a companion project —
a single-binary WordPress hosting tool with its own built-in Borg backup
feature. This page covers pointing ngxsetup's Borg feature at an ngxborg
server; if your client is anything else, see
[Getting started](getting-started.md) instead.

## What ngxsetup handles automatically

Unlike a fully manual client, ngxsetup manages its own dedicated SSH
keypair for reaching a remote Borg repository — it never depends on
whatever identity an interactive shell happens to resolve, which is the
difference between working "by accident" in a terminal and working
reliably from an unattended `cron`/`systemd` run.

- Leave its key field blank and it **generates** a fresh dedicated
  `ed25519` key the first time you set up a remote repository.
- Or give it an existing private key to **import** instead, if you'd
  rather bring your own.
- Either way, it shows you the **public key** afterward — that's the one
  half you need to hand to ngxborg.

## Step by step

### 1. Create the tenant and repository on ngxborg

```bash
sudo ngxborg user create backupdemo
sudo ngxborg repo create --tenant backupdemo websites
```

### 2. Point ngxsetup at it

Web UI (ngxsetup's own Backups page):

- **Repository**: `ssh://backupdemo@your-ngxborg-host:2222/var/lib/ngxborg/repos/backupdemo/websites`
- **Encryption**: `repokey-blake2` (recommended)
- **Compression**: `zstd` (recommended)
- **Passphrase**: leave blank to generate one — it's shown once, write it
  down immediately
- **SSH private key**: leave blank to have ngxsetup generate one

Or the CLI equivalent:

```bash
ngxsetup borg setup \
  --repo ssh://backupdemo@your-ngxborg-host:2222/var/lib/ngxborg/repos/backupdemo/websites \
  --encryption repokey-blake2 --compression zstd --generate
```

### 3. Register the public key ngxsetup shows you

The first attempt will fail with `Permission denied (publickey)` — that's
expected: ngxborg doesn't know this new key yet. ngxsetup prints exactly
the command you need, host and repository already filled in:

```bash
ngxborg user key add --tenant backupdemo websites 'ssh-ed25519 AAAA...'
```

Run that on the **ngxborg** host (or paste the printed public key into
its web UI's **SSH Keys → Register a key** — see
[Web UI guide](web-ui.md)).

### 4. Re-run setup

```bash
ngxsetup borg setup --repo ssh://backupdemo@your-ngxborg-host:2222/... --generate
```

This time the key is already registered, `borg init` succeeds, and the
repository is ready. `ngxsetup borg status` confirms `reachable: yes`.

## The URL, piece by piece

```
ssh://backupdemo@your-ngxborg-host:2222/var/lib/ngxborg/repos/backupdemo/websites
       └────┬───┘ └────────┬────────┘ └┬┘ └──────────────────┬──────────────────┘
          tenant     ngxborg host    Borg port          the repository's

```

- **tenant** must match a real account created with `ngxborg user create`.
- **port** is ngxborg's dedicated Borg port (`2222` unless you customized
  it — see `ngxborg doctor` or the web UI's Repositories → commands panel
  to confirm the real value on your host).
- **path** is exactly what `ngxborg repo create` reserved — the web UI's
  per-repository **commands** panel (see
  [Web UI guide → Client commands](web-ui.md#client-commands)) shows this
  whole URL ready to copy, so you never have to assemble it by hand.

## Restoring

ngxborg has no role in restoring — that stays entirely in ngxsetup, which
talks to the same repository with `borg extract`/`borg mount` under the
hood (`ngxsetup borg restore`). ngxborg's job ends at "let the right key
reach the right repository."
