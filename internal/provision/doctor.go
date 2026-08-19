package provision

import (
	"os"
	"os/user"
	"strconv"

	"ngxborg/internal/authpam"
	"ngxborg/internal/borgrepo"
	"ngxborg/internal/facts"
	"ngxborg/internal/posix"
	"ngxborg/internal/sshaccess"
	"ngxborg/internal/system"
)

// Status is one diagnostic result, the same shape ngxsetup's own doctor
// command uses: a finding without a suggested fix has done half a job.
type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Check struct {
	Name   string
	Status Status
	Detail string
	Fix    string
}

// Diagnose runs every health check ngxborg has and returns the findings.
func (c *Ctx) Diagnose() []Check {
	var out []Check
	add := func(ck Check) { out = append(out, ck) }

	if _, err := user.LookupGroup(posix.TenantGroup); err != nil {
		add(Check{"tenant group", StatusFail, posix.TenantGroup + " does not exist", "ngxborg setup"})
	} else {
		add(Check{"tenant group", StatusOK, posix.TenantGroup + " exists", ""})
	}
	if _, err := user.LookupGroup(posix.AdminGroup); err != nil {
		add(Check{"admin group", StatusFail, posix.AdminGroup + " does not exist", "ngxborg setup"})
	} else {
		add(Check{"admin group", StatusOK, posix.AdminGroup + " exists", ""})
	}

	pamPath := "/etc/pam.d/" + authpam.ServiceName
	if _, err := os.Stat(pamPath); err != nil {
		add(Check{"PAM stack", StatusFail, pamPath + " is missing", "ngxborg setup"})
	} else {
		add(Check{"PAM stack", StatusOK, pamPath + " present", ""})
	}

	for _, pkg := range []string{"borgbackup", "openssh-server"} {
		if system.PackageInstalled(c.Context, c.Runner, pkg) {
			add(Check{"package: " + pkg, StatusOK, "installed", ""})
		} else {
			add(Check{"package: " + pkg, StatusFail, "not installed", "ngxborg setup"})
		}
	}

	if port, err := sshaccess.BorgPort(); err != nil {
		add(Check{"ssh dual-port", StatusFail, err.Error(), "ngxborg setup"})
	} else {
		out = append(out, sshdCheck(c, port))
	}

	for _, dir := range []string{borgrepo.Base, borgrepo.TrashBase} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			add(Check{"storage: " + dir, StatusFail, "missing", "ngxborg setup"})
		} else {
			add(Check{"storage: " + dir, StatusOK, "present", ""})
		}
	}

	if system.IsActive(c.Context, c.Runner, "ngxborg-web.service") {
		add(Check{"web UI", StatusOK, "running", ""})
	} else if system.UnitExists(c.Context, c.Runner, "ngxborg-web.service") {
		add(Check{"web UI", StatusWarn, "installed but not running", "ngxborg install service"})
	} else {
		add(Check{"web UI", StatusWarn, "not installed", "ngxborg install service (optional)"})
	}

	if c.Facts.DiskFreeMB > 0 && c.Facts.DiskFreeMB < 1024 {
		add(Check{"disk space", StatusWarn, facts.FormatMB(c.Facts.DiskFreeMB) + " free", "repositories will fail to accept new archives soon"})
	} else if c.Facts.DiskFreeMB > 0 {
		add(Check{"disk space", StatusOK, facts.FormatMB(c.Facts.DiskFreeMB) + " free", ""})
	}

	return out
}

func sshdCheck(c *Ctx, borgPort int) Check {
	// sshd -t refuses to run at all without /run/sshd existing (see
	// sshaccess.EnsurePrivilegeSeparationDir's doc comment) — a doctor run
	// against a host where setup hasn't (re-)created it since the last
	// boot would otherwise misreport a perfectly valid config as broken.
	if err := sshaccess.EnsurePrivilegeSeparationDir(); err != nil {
		return Check{"sshd config", StatusFail, "could not create /run/sshd", err.Error()}
	}
	if err := c.Runner.Run(c.Context, "sshd", "-t"); err != nil {
		return Check{"sshd config", StatusFail, "sshd -t rejects the current configuration", "check " + sshaccess.DropInPath}
	}
	return Check{"sshd config", StatusOK, "valid; borg port " + strconv.Itoa(borgPort), ""}
}
