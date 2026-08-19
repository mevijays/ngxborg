# ngxborg

[![CI](https://github.com/mevijays/ngxborg/actions/workflows/ci.yml/badge.svg)](https://github.com/mevijays/ngxborg/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mevijays/ngxborg?include_prereleases)](https://github.com/mevijays/ngxborg/releases)
[![Docs](https://img.shields.io/badge/docs-mevijays.github.io%2Fngxborg-blue)](https://mevijays.github.io/ngxborg/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

A multi-tenant, PAM-authenticated [Borg](https://www.borgbackup.org/)
backup server for Debian/Ubuntu — one binary, no agent, no separate user
database.

```bash
sudo ngxborg setup
sudo ngxborg user create alice
sudo ngxborg repo create --tenant alice websites
```

Every tenant is a real POSIX account. Web UI login is real PAM
authentication against that account's password. The backup transport is
plain SSH, restricted per key to exactly one repository via OpenSSH's own
forced-command mechanism. There's no ngxborg-specific credential store
anywhere.

## Install

```bash
curl -fLO https://github.com/mevijays/ngxborg/releases/latest/download/ngxborg-linux-amd64
sudo install -m 0755 ngxborg-linux-amd64 /usr/local/bin/ngxborg
sudo ngxborg setup
```

`arm64` builds are published too. See
[Installation](https://mevijays.github.io/ngxborg/installation/) for
checksums, building from source, and starting the web UI.

## Documentation

Full docs: **[mevijays.github.io/ngxborg](https://mevijays.github.io/ngxborg/)**

- [Getting started](https://mevijays.github.io/ngxborg/getting-started/)
- [Architecture](https://mevijays.github.io/ngxborg/architecture/)
- [CLI reference](https://mevijays.github.io/ngxborg/cli-reference/)
- [Security model](https://mevijays.github.io/ngxborg/security/)
- [Pairing with ngxsetup](https://mevijays.github.io/ngxborg/ngxsetup-integration/)

## Contributing

Issues and pull requests are welcome. See
[CONTRIBUTING.md](CONTRIBUTING.md) for dev setup, running tests, and what
makes a good PR — it's a short read.

## License

[MIT](LICENSE)
