// Package build holds version metadata stamped in at release build time.
package build

// Version is set at release build time via
// -ldflags "-X ngxborg/internal/build.Version=vX.Y.Z". Left as "dev" for
// any build that did not set it, so a developer build never claims to be a
// tagged release it is not.
var Version = "dev"

// Maintainer and RepoURL are shown by `ngxborg version` and in the web UI's
// footer, matching ngxsetup's own precedent for the same reason: an
// operator looking at a running instance should be able to tell whose tool
// this is and where to file an issue without hunting through documentation.
var (
	Maintainer = "Vijay Vishwakarma"
	RepoURL    = "https://github.com/mevijays/ngxborg"
)
