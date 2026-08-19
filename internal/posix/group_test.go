package posix

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadGroupFileParsesMembers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "group")
	content := "# a comment, and a blank line follow\n\n" +
		"root:x:0:\n" +
		"ngxborg:x:2001:alice,bob\n" +
		"ngxborg-admin:x:2002:alice\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	groups, err := readGroupFile(path)
	if err != nil {
		t.Fatal(err)
	}

	want := []groupEntry{
		{name: "root", gid: "0", members: nil},
		{name: "ngxborg", gid: "2001", members: []string{"alice", "bob"}},
		{name: "ngxborg-admin", gid: "2002", members: []string{"alice"}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("got %+v, want %+v", groups, want)
	}
}

func TestReadGroupFileMissingFile(t *testing.T) {
	if _, err := readGroupFile("/no/such/group/file"); err == nil {
		t.Error("expected an error for a missing file")
	}
}
