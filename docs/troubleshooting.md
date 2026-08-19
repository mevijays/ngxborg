# Troubleshooting

## `Permission denied (publickey)` from a client

By far the most common issue, and almost always one of:

1. **The key hasn't been registered yet.** `ngxborg user key list --tenant <name>`
   to check. Register it with `ngxborg user key add` — see
   [Getting started](getting-started.md#4-register-the-public-key) or, if
   the client is ngxsetup, [Pairing with ngxsetup](ngxsetup-integration.md).
2. **The key is registered for a different repository than the one being
   connected to.** Keys are restricted per-repository, not per-tenant —
   see [Security model → Per-key path restriction](security.md#per-key-path-restriction).
   Double-check the path in the `ssh://` URL matches exactly what
   `ngxborg repo create` reserved.
3. **The tenant account is disabled.** `ngxborg user list` shows disabled
   accounts; `ngxborg user enable <name>` restores access, including
   every SSH key on it — see
   [Security model → Reversible disable](security.md#reversible-disable-not-deletion).
4. **The repository itself is disabled**, independent of the tenant
   account. `ngxborg repo list` shows it; `ngxborg repo enable`
   restores it.
5. **The wrong port.** The Borg port defaults to `2222`, not `22` — check
   `ngxborg doctor`'s "sshd config" line, or the exact port in the
   repository's client-commands panel (see
   [Web UI guide](web-ui.md#client-commands)).

## Connection times out, or "Connection refused"

- A firewall (commonly `ufw`) may not have an explicit rule for the Borg
  port. `ngxborg setup` opens it automatically when `ufw` is active and
  managed by this host, but a firewall managed elsewhere (a cloud
  provider's security group, an external appliance) needs its own rule
  added by hand.
- Confirm `sshd` is actually listening on both ports:
  `ss -tlnp | grep sshd` (or `sudo ngxborg doctor`, which checks this).

## `ngxborg doctor` reports a failed check

Each check names exactly what's wrong (a missing group, an absent PAM
file, an invalid `sshd` config, a missing directory). Re-running
`sudo ngxborg setup` fixes the overwhelming majority of these — it's
idempotent and safe to run again at any time. If a specific check keeps
failing after that, please open an issue with the full `doctor` output.

## "I lost the repository passphrase"

There's no recovery path — ngxborg never sees or stores it, by design
(see [Architecture → Repository lifecycle](architecture.md#repository-lifecycle)),
and neither does Borg itself store it anywhere retrievable. A repository
without its passphrase is permanently unreadable. This is exactly why
both the CLI and web UI show a generated passphrase exactly once, with an
explicit warning to write it down immediately.

## `<username> is not a registered ngxborg account`

The CLI refuses to act for any Unix account that was never created with
`ngxborg user create` — a valid shell login on the box is not, by
itself, ngxborg access. Create the account first, or run the command as
root/`sudo` for admin scope.

## Browser warns about the certificate

Expected — the web UI serves a self-signed certificate by default (see
`ngxborg install service`). Put a real certificate in front of it (a
reverse proxy, or your own cert/key pair) if that warning is a problem
for your use case; nothing about ngxborg's own protocol requires the
default self-signed cert specifically.

## Building from source fails with a PAM-related error

You're missing the PAM development headers — install `libpam0g-dev`
(Debian/Ubuntu) before running `go build`. See
[Installation → Building from source](installation.md#building-from-source).
A downloaded release binary needs nothing extra; this only affects
building from source yourself.

## Still stuck?

Please [open an issue](https://github.com/mevijays/ngxborg/issues/new)
with the output of `sudo ngxborg doctor` and the exact command/error —
see [Contributing](contributing.md) for what's useful to include.
