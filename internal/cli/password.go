package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
)

// readPassword gets a password from whichever source fits the caller: piped
// stdin (scripting — `echo "$PW" | ngxborg user passwd alice`) reads it
// unmasked, since anything already piping a secret through a shell has
// already accepted that exposure; an interactive terminal instead prompts
// twice with echo turned off via `stty`, matching how ssh-keygen/passwd
// themselves ask. Shelling out to stty rather than pulling in a terminal
// library keeps this project dependency-free, consistent with everywhere
// else it shells out to a real tool instead of reimplementing one.
func readPassword(prompt string) (string, error) {
	if !isTerminal(os.Stdin) {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("reading password from stdin: %w", err)
		}
		return trimNewline(line), nil
	}

	fmt.Fprint(os.Stderr, prompt)
	pw1, err := readMasked()
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirm: ")
	pw2, err := readMasked()
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if pw1 != pw2 {
		return "", fmt.Errorf("passwords did not match")
	}
	if pw1 == "" {
		return "", fmt.Errorf("password must not be empty")
	}
	return pw1, nil
}

func readMasked() (string, error) {
	if err := exec.Command("stty", "-F", "/dev/tty", "-echo").Run(); err != nil {
		// Not a real terminal after all, or stty unavailable — fall back to
		// a plain (visible) read rather than failing outright.
		reader := bufio.NewReader(os.Stdin)
		line, rerr := reader.ReadString('\n')
		return trimNewline(line), rerr
	}
	defer exec.Command("stty", "-F", "/dev/tty", "echo").Run()

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return trimNewline(line), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
