package sshaccess

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ngxborg/internal/posix"
)

// borgBinary is invoked with an absolute path in the forced command rather
// than bare "borg": authorized_keys' command= runs outside any login shell,
// so it inherits no PATH an operator's shell profile might set — an
// absolute path is the only way this is guaranteed to find the right
// binary regardless of how sparse that environment is.
const borgBinary = "/usr/bin/borg"

// keyLinePrefix marks every line this package wrote, so ListKeys/RemoveKey
// can tell an ngxborg-managed entry apart from any other key an operator's
// own tooling (or the operator by hand) might have placed in the same file
// — those are left alone entirely.
const keyLinePrefix = `command="` + borgBinary + ` serve --restrict-to-path `

// KeyEntry is one ngxborg-managed authorized_keys line, decoded back into
// its parts for display.
type KeyEntry struct {
	RepoPath    string
	AppendOnly  bool
	KeyType     string
	KeyMaterial string
	Comment     string
}

// sshPublicKeyTypes are the algorithm names a valid OpenSSH public key line
// starts with.
var sshPublicKeyTypes = map[string]bool{
	"ssh-rsa": true, "ssh-ed25519": true,
	"ecdsa-sha2-nistp256": true, "ecdsa-sha2-nistp384": true, "ecdsa-sha2-nistp521": true,
	"sk-ssh-ed25519@openssh.com": true, "sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// validatePublicKeyLine rejects anything that is not plausibly one public
// key on one line, the same shape of check ngxsetup applies to its own
// break-glass SSH key setting, for the same reason: this string is about to
// be written verbatim into a root-writable file that controls who can
// connect as this account, so it is worth being sure before it gets there.
func validatePublicKeyLine(line string) (keyType, keyMaterial, comment string, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "\n") {
		return "", "", "", fmt.Errorf("expected a single public key line")
	}
	if strings.Contains(line, "PRIVATE KEY") {
		return "", "", "", fmt.Errorf("that looks like a private key, not a public one")
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", "", fmt.Errorf("expected '<type> <base64-key> [comment]', e.g. 'ssh-ed25519 AAAA... you@host'")
	}
	if !sshPublicKeyTypes[fields[0]] {
		return "", "", "", fmt.Errorf("%q is not a recognised SSH public key type", fields[0])
	}
	if _, err := base64.StdEncoding.DecodeString(fields[1]); err != nil {
		return "", "", "", fmt.Errorf("key material after %q is not valid base64: %w", fields[0], err)
	}
	c := ""
	if len(fields) > 2 {
		c = strings.Join(fields[2:], " ")
	}
	return fields[0], fields[1], c, nil
}

// authorizedKeysPath resolves a tenant's authorized_keys file and ensures
// its containing directory exists with the permissions sshd insists on
// (world- or group-writable ~/.ssh is a hard refusal, not a warning).
func authorizedKeysPath(username string) (string, uint32, uint32, error) {
	home, err := posix.HomeDir(username)
	if err != nil {
		return "", 0, 0, err
	}
	uid, gid, err := posix.UIDGID(username)
	if err != nil {
		return "", 0, 0, err
	}
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", 0, 0, fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.Chown(dir, int(uid), int(gid)); err != nil {
		return "", 0, 0, fmt.Errorf("setting ownership of %s: %w", dir, err)
	}
	return filepath.Join(dir, "authorized_keys"), uid, gid, nil
}

// AddKey registers a public key restricted to one repository — the
// mechanism behind "give a tenant's borg client a way to connect": the key
// can run exactly one command, `borg serve` scoped to repoPath, nothing
// else. Idempotent by key material: re-adding the same key retargets its
// existing line to the new repoPath/appendOnly rather than creating a
// second, conflicting entry — a given key restricted to two different
// repositories at once is not a state this package will produce, since
// which restriction actually takes effect would depend on which line sshd
// happens to match first.
func AddKey(username, repoPath, pubKeyLine string, appendOnly bool) error {
	keyType, keyMaterial, comment, err := validatePublicKeyLine(pubKeyLine)
	if err != nil {
		return err
	}
	path, uid, gid, err := authorizedKeysPath(username)
	if err != nil {
		return err
	}

	existing, err := readLines(path)
	if err != nil {
		return err
	}
	newLine := buildLine(repoPath, appendOnly, keyType, keyMaterial, comment)

	var out []string
	replaced := false
	for _, line := range existing {
		if lineKeyMaterial(line) == keyMaterial {
			out = append(out, newLine) // retarget in place
			replaced = true
			continue
		}
		out = append(out, line)
	}
	if !replaced {
		out = append(out, newLine)
	}

	if err := writeLines(path, out); err != nil {
		return err
	}
	return os.Chown(path, int(uid), int(gid))
}

// RemoveKey deletes an ngxborg-managed key by its base64 key material (the
// same value ListKeys reports as KeyMaterial). Non-ngxborg lines in the same
// file are never touched.
func RemoveKey(username, keyMaterial string) error {
	path, uid, gid, err := authorizedKeysPath(username)
	if err != nil {
		return err
	}
	existing, err := readLines(path)
	if err != nil {
		return err
	}
	var out []string
	found := false
	for _, line := range existing {
		if lineKeyMaterial(line) == keyMaterial {
			found = true
			continue
		}
		out = append(out, line)
	}
	if !found {
		return fmt.Errorf("no ngxborg-managed key with that material found for %s", username)
	}
	if err := writeLines(path, out); err != nil {
		return err
	}
	return os.Chown(path, int(uid), int(gid))
}

// ListKeys returns every ngxborg-managed key registered for a tenant.
// Lines this package did not write (an operator's own unrestricted key,
// say) are silently skipped — they are none of ngxborg's business to
// report on or manage.
func ListKeys(username string) ([]KeyEntry, error) {
	path, _, _, err := authorizedKeysPath(username)
	if err != nil {
		return nil, err
	}
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var entries []KeyEntry
	for _, line := range lines {
		if e, ok := parseLine(line); ok {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func buildLine(repoPath string, appendOnly bool, keyType, keyMaterial, comment string) string {
	cmd := borgBinary + " serve --restrict-to-path " + repoPath
	if appendOnly {
		cmd += " --append-only"
	}
	line := fmt.Sprintf(`command="%s",restrict %s %s`, cmd, keyType, keyMaterial)
	if comment != "" {
		line += " " + comment
	}
	return line
}

// parseLine decodes a line this package wrote. It deliberately only
// recognises its own exact format (keyLinePrefix's fixed structure) rather
// than trying to generally parse arbitrary command= restrictions, so a line
// an operator hand-edited to something unusual is reported as "not
// ngxborg's" rather than misparsed.
func parseLine(line string) (KeyEntry, bool) {
	if !strings.HasPrefix(line, keyLinePrefix) {
		return KeyEntry{}, false
	}
	rest := strings.TrimPrefix(line, keyLinePrefix)
	pathEnd := strings.IndexByte(rest, '"')
	if pathEnd < 0 {
		return KeyEntry{}, false
	}
	pathAndFlags := rest[:pathEnd]
	appendOnly := false
	repoPath := pathAndFlags
	if idx := strings.Index(pathAndFlags, " --append-only"); idx >= 0 {
		appendOnly = true
		repoPath = pathAndFlags[:idx]
	}

	after := rest[pathEnd+1:]
	const marker = `,restrict `
	mi := strings.Index(after, marker)
	if mi < 0 {
		return KeyEntry{}, false
	}
	keyPart := strings.TrimSpace(after[mi+len(marker):])
	fields := strings.Fields(keyPart)
	if len(fields) < 2 {
		return KeyEntry{}, false
	}
	comment := ""
	if len(fields) > 2 {
		comment = strings.Join(fields[2:], " ")
	}
	return KeyEntry{
		RepoPath:    repoPath,
		AppendOnly:  appendOnly,
		KeyType:     fields[0],
		KeyMaterial: fields[1],
		Comment:     comment,
	}, true
}

// lineKeyMaterial extracts just the base64 key material from any
// authorized_keys line this package wrote, for matching against a
// candidate key regardless of what repo/flags it currently carries.
func lineKeyMaterial(line string) string {
	e, ok := parseLine(line)
	if !ok {
		return ""
	}
	return e.KeyMaterial
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	// authorized_keys lines can legitimately be long (an ecdsa/rsa key plus
	// a verbose command= restriction easily exceeds bufio's 64KB default).
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func writeLines(path string, lines []string) error {
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	if err := atomicWrite(path, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// FormatCount is a small display helper for the CLI/web UI ("3 keys").
func FormatCount(n int) string {
	if n == 1 {
		return "1 key"
	}
	return strconv.Itoa(n) + " keys"
}
