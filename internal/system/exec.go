// Package system wraps the commands and services this tool has to drive.
//
// Two rules shape the design. Errors carry the command's actual output,
// because "exit status 1" from apt or useradd is worthless to whoever is
// reading the terminal at 2am. And nothing sensitive — a password, a
// passphrase — is ever passed as a command-line argument, because /proc
// makes every process's argv readable by every user on the machine; secrets
// go in over stdin instead.
package system

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"ngxborg/internal/logx"
)

// Runner executes external commands.
type Runner struct {
	// DryRun prints commands instead of running them. Read-only commands still
	// execute, since the tool has to be able to inspect the system in order to
	// report what it would do.
	DryRun bool
	// Timeout bounds a single command.
	Timeout time.Duration
	// ExtraEnv is appended to the child's environment.
	ExtraEnv []string
	// Dir sets the child's working directory. Empty means inherit this
	// process's own.
	Dir string
}

const defaultTimeout = 10 * time.Minute

func (r Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return defaultTimeout
}

// Look reports whether a command exists on PATH.
func (r Runner) Look(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Run executes a command that changes system state. It is suppressed by DryRun.
func (r Runner) Run(ctx context.Context, name string, args ...string) error {
	if r.DryRun {
		logx.Change("[dry-run] would run: %s %s", name, strings.Join(args, " "))
		return nil
	}
	logx.Debug("$ %s %s", name, strings.Join(args, " "))
	_, err := r.capture(ctx, nil, name, args...)
	return err
}

// Output runs a read-only command and returns its stdout. It executes even
// under DryRun: the tool needs to inspect the machine to decide what it would
// change.
func (r Runner) Output(ctx context.Context, name string, args ...string) (string, error) {
	logx.Debug("$ %s %s", name, strings.Join(args, " "))
	out, err := r.capture(ctx, nil, name, args...)
	return strings.TrimSpace(out), err
}

// RunStdin executes a command with input piped to stdin — how a password
// reaches chpasswd, how a repository passphrase reaches borg, and generally
// how anything sensitive reaches a child process without ever appearing in
// its argv.
func (r Runner) RunStdin(ctx context.Context, stdin, name string, args ...string) (string, error) {
	if r.DryRun {
		logx.Change("[dry-run] would run: %s %s (with piped input)", name, strings.Join(args, " "))
		return "", nil
	}
	logx.Debug("$ %s %s (stdin: %d bytes)", name, strings.Join(args, " "), len(stdin))
	return r.capture(ctx, strings.NewReader(stdin), name, args...)
}

// TryRun runs a command whose failure is acceptable, logging it as a warning.
func (r Runner) TryRun(ctx context.Context, name string, args ...string) {
	if err := r.Run(ctx, name, args...); err != nil {
		logx.Debug("optional command failed: %s %s: %v", name, strings.Join(args, " "), err)
	}
}

func (r Runner) capture(ctx context.Context, stdin io.Reader, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	cmd.Dir = r.Dir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	cmd.Env = append(append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"LC_ALL=C",
	), r.ExtraEnv...)

	err := cmd.Run()
	if err == nil {
		return out.String(), nil
	}
	if cctx.Err() == context.DeadlineExceeded {
		return out.String(), fmt.Errorf("%s timed out after %s", name, r.timeout())
	}

	detail := strings.TrimSpace(errBuf.String())
	if detail == "" {
		detail = strings.TrimSpace(out.String())
	}
	if detail == "" {
		return out.String(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, indent(detail))
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 20 {
		lines = append([]string{"    ..."}, lines[len(lines)-20:]...)
	}
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}
