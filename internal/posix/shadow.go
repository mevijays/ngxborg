package posix

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// shadowPath is a var, not a const, so tests could point it at a fixture —
// none do yet (IsDisabled needs a real registered account to mean
// anything, the same constraint the rest of this package's tests document
// for CreateRepo-adjacent behaviour), but there is no reason to close that
// door.
var shadowPath = "/etc/shadow"

// readShadowField returns one colon-separated field from a username's
// /etc/shadow line. Reading this file requires root — it is mode 0640,
// owned by root:shadow on every distribution this tool targets — which
// every ngxborg command already requires for unrelated reasons.
func readShadowField(username string, field int) (string, error) {
	f, err := os.Open(shadowPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", shadowPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) <= field || fields[0] != username {
			continue
		}
		return fields[field], nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no shadow entry for %q", username)
}
