package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirm asks a yes/no question on stderr (so it never pollutes stdout a
// script might be capturing) and reads the answer from stdin. Anything but
// an explicit "y"/"yes" is treated as no, the safe default for an
// irreversible operation.
func confirm(question string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
