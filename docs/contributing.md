# Contributing

Contributions are welcome — bug reports, documentation fixes, and pull
requests alike.

## Development setup

```bash
sudo apt-get update
sudo apt-get install -y libpam0g-dev   # authpam needs the PAM headers to build
git clone https://github.com/mevijays/ngxborg.git
cd ngxborg
go build ./...
```

ngxborg only builds and runs on Linux (Debian/Ubuntu-family) — see
[Installation → Building from source](installation.md#building-from-source).
If your day-to-day machine is a Mac or Windows box, a disposable Ubuntu VM
or container is the easiest way to get a real target to test setup/doctor
against; the unit tests themselves (below) don't need one.

## Running the tests

```bash
gofmt -l .              # should print nothing
go vet ./...
go test -race -count=1 ./...
```

All three run in CI on every pull request (`amd64` and `arm64`), along
with an end-to-end job that stands up a real host, creates tenants and
repositories, and adversarially confirms one key can't reach another
tenant's or another repository's data — see `.github/workflows/ci.yml`
if you want to run the same steps locally.

## Code style

- Run `gofmt` before committing; CI fails otherwise.
- This codebase favors comments that explain *why* a piece of code looks
  the way it does — especially around a real bug that shaped it — over
  comments that restate *what* the next line obviously does. If you're
  fixing something non-obvious, a sentence on what broke and why the fix
  works this way is more valuable than the diff alone.
- No CDN dependencies in the web UI. Tailwind CSS, Font Awesome, and
  Chart.js are vendored under `internal/webui/static/vendor/` —
  keep it that way; a browser loading this UI should never need internet
  access beyond the ngxborg server itself.
- Prefer a real, disposable Linux host (or VM/container) for testing
  anything that touches `sshd`, PAM, or POSIX accounts — mocking these
  convincingly is harder than standing up the real thing, and this
  project's own history has caught more than one bug that only a real
  target surfaced (see `internal/sshaccess` and `internal/borgrepo`'s
  package docs for two examples).

## Making a pull request

1. Fork the repository and create a branch off `main`.
2. Keep the change focused — a pull request that does one thing is much
   easier to review than one that reorganizes unrelated code along the
   way.
3. Add or update tests for anything behavioral. A change with no test
   coverage for what it fixes is much likelier to regress silently later.
4. Make sure `gofmt`, `go vet`, and `go test -race ./...` all pass
   locally before opening the PR — CI runs the same checks, but catching
   it locally is faster for everyone.
5. Describe *why*, not just *what*, in the PR description — especially
   for a bug fix: what was the actual failure, and how did you confirm
   the fix addresses it?

## Reporting bugs

Please include:

- The exact command (or web UI action) and what happened, verbatim
  (error text, not a paraphrase).
- `sudo ngxborg doctor`'s output.
- Your OS/distribution and `ngxborg version`'s output.

## Reporting a security issue

See [Security model → Reporting a vulnerability](security.md#reporting-a-vulnerability).
