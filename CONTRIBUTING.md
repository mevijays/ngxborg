# Contributing to ngxborg

Thanks for considering it. The short version:

```bash
sudo apt-get install -y libpam0g-dev   # authpam needs PAM headers to build
go build ./...
gofmt -l .                             # should print nothing
go vet ./...
go test -race -count=1 ./...
```

ngxborg only builds and runs on Linux — see the full guide for why and
for a from-source build walkthrough.

Open an issue before a large change so we can agree on the approach first;
small fixes and docs corrections can just be a pull request.

**The full guide** — code style, what a good PR looks like, what to
include in a bug report, and how to report a security issue privately —
lives at
**[docs/contributing.md](docs/contributing.md)** (also published at
[mevijays.github.io/ngxborg/contributing](https://mevijays.github.io/ngxborg/contributing/)).
