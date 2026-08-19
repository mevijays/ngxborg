// Package borgrepo manages the on-disk lifecycle of a tenant's backup
// repositories.
//
// The one thing this package deliberately never does is run `borg init`,
// `borg create`, or anything else that needs a repository's encryption
// passphrase. That is not an oversight — it is Borg's own architecture: a
// repository's passphrase belongs to whoever is backing data up, not to the
// server storing it, and in the standard "repokey" encryption mode the
// passphrase never needs to leave the client at all. ngxborg's job on the
// server side is narrower and simpler: reserve an empty, correctly-owned
// directory and restrict one SSH key to it (see internal/sshaccess); the
// tenant's own borg client then runs `borg init` against that directory the
// first time it connects, choosing its own encryption and passphrase. What
// this package inspects afterwards (size, whether init has happened yet) is
// everything that can be read off the filesystem without ever needing that
// passphrase.
package borgrepo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ngxborg/internal/posix"
)

// Base is where every tenant's repositories live, one subdirectory per
// tenant username, one subdirectory per repo within that. A var, not a
// const, so unit tests can point it at a temp directory instead of the
// real root-only system path.
var Base = "/var/lib/ngxborg/repos"

// TrashBase holds soft-deleted repositories pending a Purge — the same
// "delete is recoverable, purge is not" split ngxsetup itself uses for
// site removal, for the same reason: "wrong repo name" should be a mistake
// recoverable in one more command, not a permanent one.
var TrashBase = "/var/lib/ngxborg/.trash"

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// ValidateName rejects anything that is not a short, plain identifier — no
// "/", no "..", no leading dot, no whitespace. This is interpolated
// straight into filesystem paths, so the only safe policy is refusing
// everything that is not obviously safe rather than trying to escape
// everything that might not be.
func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%q is not a valid name — letters, digits, '-' and '_' only, starting with a letter or digit", name)
	}
	return nil
}

// Repo describes one repository as seen from the server side — everything
// knowable without its passphrase.
type Repo struct {
	Tenant      string
	Name        string
	Path        string
	Initialized bool
	Disabled    bool
	SizeMB      int64
	CreatedAt   time.Time
}

// disabledMode is what Disable sets a repository directory's permissions
// to: nothing, for anyone, owner included. A tenant's own borg serve
// process (running as that tenant, not root) then fails to even os.stat()
// the directory — the exact PermissionError CreateRepo's own history
// already proved is fatal to every borg operation, deliberately reused
// here as the mechanism rather than inventing a second one. Root-run
// operations (the CLI, the web UI) are unaffected, since root ignores
// standard Unix permission checks — which is correct: disabling a
// repository is about closing the tenant's own access to it, not the
// operator's.
const disabledMode = 0o000

// enabledMode is what CreateRepo already used and what Enable restores.
const enabledMode = 0o700

func repoDir(tenant, name string) string { return filepath.Join(Base, tenant, name) }

// Path returns where a repository would live on disk, whether or not it
// currently exists — used to build the forced SSH command a key gets
// restricted to (internal/sshaccess.AddKey) before or after the repo
// itself is created; a key can legitimately be registered first if an
// operator wants the exact path known ahead of time.
func Path(tenant, name string) string { return repoDir(tenant, name) }

// Exists reports whether a repository directory is currently present
// (live, not merely once-existed-then-trashed).
func Exists(tenant, name string) bool {
	_, err := os.Stat(repoDir(tenant, name))
	return err == nil
}

// CreateRepo reserves an empty, tenant-owned directory for a new repository.
// It does not run `borg init` — see the package doc comment — so the
// returned Repo has Initialized=false until the tenant's own borg client
// connects and runs init against the path this returns.
func CreateRepo(tenant, name string) (Repo, error) {
	if err := ValidateName(tenant); err != nil {
		return Repo{}, err
	}
	if err := ValidateName(name); err != nil {
		return Repo{}, err
	}
	if !posix.IsTenant(tenant) {
		return Repo{}, fmt.Errorf("%q is not a registered ngxborg tenant", tenant)
	}
	dir := repoDir(tenant, name)
	if _, err := os.Stat(dir); err == nil {
		return Repo{}, fmt.Errorf("tenant %s already has a repository named %q", tenant, name)
	}
	uid, gid, err := posix.UIDGID(tenant)
	if err != nil {
		return Repo{}, err
	}
	// The tenant-level directory (Base/tenant/) needs the same ownership
	// treatment as the repo directory itself, not just the leaf: MkdirAll
	// creates every missing intermediate directory as root:root 0700 (this
	// process's own identity, running as root), and `borg serve` running
	// as the tenant cannot traverse into a parent directory it has no
	// permission to even stat — confirmed live: `borg init` failed with a
	// bare PermissionError until this was fixed, `mybackup/` itself was
	// correctly tenant-owned but its own parent, created as a side effect
	// of MkdirAll a moment earlier, was not.
	tenantDir := filepath.Join(Base, tenant)
	if err := ensureOwnedDir(tenantDir, uid, gid, 0o750); err != nil {
		return Repo{}, err
	}
	if err := ensureOwnedDir(dir, uid, gid, 0o700); err != nil {
		return Repo{}, err
	}
	return statRepo(tenant, name, dir)
}

// ensureOwnedDir creates a directory (if absent) and makes sure it is owned
// by uid:gid — including when it already existed, so a stale root-owned
// directory from before this fix (or from any other path that might
// create one) self-heals the next time a repo is created under it, rather
// than requiring a manual chown.
func ensureOwnedDir(dir string, uid, gid uint32, mode os.FileMode) error {
	if err := os.MkdirAll(dir, mode); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.Chown(dir, int(uid), int(gid)); err != nil {
		return fmt.Errorf("setting ownership of %s: %w", dir, err)
	}
	return nil
}

// ListRepos returns one tenant's own repositories.
func ListRepos(tenant string) ([]Repo, error) {
	if err := ValidateName(tenant); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(Base, tenant))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listing repositories for %s: %w", tenant, err)
	}
	var repos []Repo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r, err := statRepo(tenant, e.Name(), repoDir(tenant, e.Name()))
		if err != nil {
			continue // skip anything that vanished between ReadDir and Stat
		}
		repos = append(repos, r)
	}
	return repos, nil
}

// ListAllRepos returns every tenant's repositories — the admin view.
func ListAllRepos() ([]Repo, error) {
	tenants, err := os.ReadDir(Base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", Base, err)
	}
	var all []Repo
	for _, t := range tenants {
		if !t.IsDir() {
			continue
		}
		repos, err := ListRepos(t.Name())
		if err != nil {
			continue
		}
		all = append(all, repos...)
	}
	return all, nil
}

// Delete soft-deletes a repository by moving it into TrashBase, timestamped
// so a name can be reused (or the same repo restored) without collision.
// The tenant's SSH key restriction still points at the old path afterward
// and must be removed or repointed separately — Delete only moves data,
// deliberately not touching internal/sshaccess, so a delete never silently
// changes what a key is allowed to do.
func Delete(tenant, name string) error {
	dir := repoDir(tenant, name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no repository %q for tenant %s: %w", name, tenant, err)
	}
	if err := os.MkdirAll(TrashBase, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(TrashBase, fmt.Sprintf("%s__%s__%s", tenant, name, time.Now().UTC().Format("20060102-150405")))
	if err := os.Rename(dir, dest); err != nil {
		return fmt.Errorf("moving %s to trash: %w", dir, err)
	}
	return nil
}

// Purge permanently and irreversibly erases a repository — either a live
// one (skipping the trash step entirely, for an operator who wants it gone
// immediately) or, more commonly, one already sitting in the trash from an
// earlier Delete.
func Purge(tenant, name string) error {
	live := repoDir(tenant, name)
	if _, err := os.Stat(live); err == nil {
		return os.RemoveAll(live)
	}
	trashed, err := findInTrash(tenant, name)
	if err != nil {
		return err
	}
	if trashed == "" {
		return fmt.Errorf("no repository %q for tenant %s, live or in trash", name, tenant)
	}
	return os.RemoveAll(trashed)
}

func findInTrash(tenant, name string) (string, error) {
	entries, err := os.ReadDir(TrashBase)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	prefix := tenant + "__" + name + "__"
	var latest string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", nil
	}
	return filepath.Join(TrashBase, latest), nil
}

func statRepo(tenant, name, dir string) (Repo, error) {
	// os.Stat succeeds regardless of dir's own permission bits — stat(2)
	// never checks a target's own mode, only that every parent directory
	// on the way to it is traversable, which root (the only thing that
	// ever calls this) always is. That is what makes reading Mode() back
	// out a reliable way to detect Disable's effect rather than something
	// this call would itself be blocked by.
	info, err := os.Stat(dir)
	if err != nil {
		return Repo{}, err
	}
	return Repo{
		Tenant:      tenant,
		Name:        name,
		Path:        dir,
		Initialized: isInitialized(dir),
		Disabled:    info.Mode().Perm() == disabledMode,
		SizeMB:      dirSizeMB(dir),
		CreatedAt:   info.ModTime(),
	}, nil
}

// Disable blocks a tenant's own access to a repository — every borg
// operation their SSH key could otherwise run against it starts failing
// immediately — without touching the repository's data, its registered
// keys, or its metadata. See disabledMode's own doc comment for the
// mechanism and why it only affects the tenant, never the operator.
func Disable(tenant, name string) error {
	dir := repoDir(tenant, name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no repository %q for tenant %s: %w", name, tenant, err)
	}
	if err := os.Chmod(dir, disabledMode); err != nil {
		return fmt.Errorf("disabling %s: %w", dir, err)
	}
	return nil
}

// Enable reverses Disable, restoring the same tenant-owned, tenant-only
// permissions CreateRepo originally set.
func Enable(tenant, name string) error {
	dir := repoDir(tenant, name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no repository %q for tenant %s: %w", name, tenant, err)
	}
	if err := os.Chmod(dir, enabledMode); err != nil {
		return fmt.Errorf("enabling %s: %w", dir, err)
	}
	return nil
}

// isInitialized checks for the "config" file `borg init` writes at a
// repository's root — recognisable by its [repository] section header,
// present in plaintext regardless of encryption mode, so this needs no
// passphrase.
func isInitialized(dir string) bool {
	body, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), "[repository]")
}

// dirSizeMB shells out to `du` rather than walking the tree in Go: a large
// repository can hold hundreds of thousands of chunk files, and du is both
// faster and already handles hardlinks/sparse files the way an operator
// expects "disk usage" to be reported.
func dirSizeMB(dir string) int64 {
	out, err := exec.Command("du", "-sm", dir).Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return n
}
