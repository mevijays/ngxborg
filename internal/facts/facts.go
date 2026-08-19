// Package facts detects just enough about the host to provision it safely:
// which distribution it is (apt vs. something else) and how much room is on
// disk for repositories. ngxborg does not need the sizing/tuning depth
// ngxsetup's own facts package has — a backup server's job is to hold data
// reliably, not to squeeze the last request/second out of nginx — so this
// stays deliberately small.
package facts

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// OS identifies the distribution.
type OS struct {
	ID         string // "ubuntu", "debian"
	IDLike     string // "debian"
	VersionID  string // "24.04"
	PrettyName string
}

// DebianFamily reports whether apt-based provisioning applies. ngxborg only
// supports Debian-family hosts today, the same boundary ngxsetup draws, for
// the same reason: package names, sshd_config.d support, and systemd unit
// conventions all assume it.
func (o OS) DebianFamily() bool {
	return o.ID == "debian" || o.ID == "ubuntu" ||
		strings.Contains(o.IDLike, "debian") || strings.Contains(o.IDLike, "ubuntu")
}

// Facts is what setup and doctor need to know about the machine.
type Facts struct {
	OS OS
	// DiskFreeMB is free space on the filesystem holding the repository
	// root, used only to warn an operator creating a repo on a nearly full
	// disk — never to size anything, unlike ngxsetup's tuning engine.
	DiskFreeMB int64
}

// Detect reads the machine's real OS release info and disk usage under root.
func Detect(root string) Facts {
	return Facts{
		OS:         detectOS(),
		DiskFreeMB: diskFreeMB(root),
	}
}

func detectOS() OS {
	var o OS
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return o
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			o.ID = v
		case "ID_LIKE":
			o.IDLike = v
		case "VERSION_ID":
			o.VersionID = v
		case "PRETTY_NAME":
			o.PrettyName = v
		}
	}
	return o
}

// diskFreeMB shells out to `df` rather than calling statfs(2) directly: the
// syscall's result struct has different field layouts per OS (this one field
// is the whole reason ngxborg's own tests can run on a Mac dev machine at
// all, since the rest of the codebase needs cgo and only builds on Linux —
// see internal/authpam), and df's "-Pm" output (POSIX format, megabyte
// blocks) is stable across every platform this could plausibly run on.
func diskFreeMB(path string) int64 {
	out, err := exec.Command("df", "-Pm", path).Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0
	}
	free, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return 0
	}
	return free
}

// FormatMB renders a megabyte count the way an operator reads it — GB above
// 1024, MB below — matching ngxsetup's own KV-table conventions.
func FormatMB(mb int64) string {
	if mb >= 1024 {
		return strconv.FormatFloat(float64(mb)/1024, 'f', 1, 64) + " GB"
	}
	return strconv.FormatInt(mb, 10) + " MB"
}
