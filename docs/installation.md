# Installation

## Requirements

- A Debian- or Ubuntu-family Linux host (ngxborg drives `apt`, `sshd`,
  `systemd`, and PAM directly — it has no other supported platform).
- Root (or `sudo`) access, for `ngxborg setup` and anything that touches
  accounts, `sshd`, or PAM.
- An outbound internet connection the first time you run `setup`, so it
  can install `borgbackup`, `openssh-server`, and PAM runtime libraries
  via `apt`.

ngxborg's web UI and CLI both authenticate against PAM
(`internal/authpam`), which links against `libpam` via cgo. The **runtime**
library (`libpam0g` or equivalent) is already present on essentially every
Debian/Ubuntu system. You only need the **development headers**
(`libpam0g-dev`) if you're building from source — see
[Building from source](#building-from-source) below. A downloaded release
binary needs nothing extra.

## Download a release

Grab the binary for your architecture from the
[releases page](https://github.com/mevijays/ngxborg/releases):

```bash
# amd64
curl -fLO https://github.com/mevijays/ngxborg/releases/latest/download/ngxborg-linux-amd64
# arm64
curl -fLO https://github.com/mevijays/ngxborg/releases/latest/download/ngxborg-linux-arm64
```

Verify against the published checksums, then install:

```bash
curl -fLO https://github.com/mevijays/ngxborg/releases/latest/download/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing

sudo install -m 0755 ngxborg-linux-amd64 /usr/local/bin/ngxborg
```

## Building from source

```bash
sudo apt-get update
sudo apt-get install -y libpam0g-dev golang-go   # or install Go from go.dev

git clone https://github.com/mevijays/ngxborg.git
cd ngxborg
go build -o ngxborg ./cmd/ngxborg
sudo install -m 0755 ngxborg /usr/local/bin/ngxborg
```

`ngxborg setup` (below) also self-installs the binary it was invoked from
to `/usr/local/bin/ngxborg`, so running it once from wherever you built or
downloaded it is enough — a separate install step is a convenience, not a
requirement.

!!! note "macOS and other platforms"
    ngxborg cannot be built for or run on macOS: PAM is a Linux/BSD
    concept with no equivalent there, and `internal/authpam` needs the
    real thing. Build and run it on Linux.

## Set up the host

```bash
sudo ngxborg setup
```

This is idempotent — safe to re-run any time. It:

1. Installs `borgbackup`, `openssh-server`, and PAM runtime packages.
2. Creates the `ngxborg` (tenant) and `ngxborg-admin` (admin) POSIX
   groups.
3. Writes `/etc/pam.d/ngxborg`, the minimal PAM stack the web UI and CLI
   authenticate tenants against.
4. Configures storage under `/var/lib/ngxborg/repos`, with the traverse
   permissions each tenant's own `borg serve` process needs to reach its
   own repository (see [Architecture](architecture.md#storage-layout)).
5. Configures `sshd` to listen on two ports — see
   [Security model](security.md#the-dual-ssh-port-design) for why.

By default, the ordinary admin SSH port stays `22` and the dedicated Borg
port is `2222`. Override either if you need to:

```bash
sudo ngxborg setup --admin-port 22 --borg-port 2222
```

Use `--dry-run` to see what setup *would* do without changing anything.

## Start the web UI

```bash
sudo ngxborg install service
```

Installs and starts a systemd unit (`ngxborg-web.service`) serving the web
UI over TLS (self-signed by default) on `:8443`. `sudo ngxborg uninstall
service` stops and disables it again — accounts and repositories are
never touched by either command.

Confirm everything is healthy:

```bash
sudo ngxborg doctor
```

## Next steps

Continue with [Getting started](getting-started.md) to create your first
tenant and repository.
