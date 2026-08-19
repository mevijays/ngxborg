package posix

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type groupEntry struct {
	name    string
	gid     string
	members []string
}

// readGroupFile parses /etc/group's colon-separated
// "name:passwd:gid:member1,member2,..." format.
func readGroupFile(path string) ([]groupEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	var groups []groupEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		var members []string
		if fields[3] != "" {
			members = strings.Split(fields[3], ",")
		}
		groups = append(groups, groupEntry{name: fields[0], gid: fields[2], members: members})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}
