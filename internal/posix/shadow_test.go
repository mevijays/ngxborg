package posix

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadShadowField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shadow")
	content := "root:!:19700:0:99999:7::\n" +
		"alice:$6$hash:19700:0:99999:7::1\n" +
		"bob:$6$hash:19700:0:99999:7::\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := shadowPath
	defer func() { shadowPath = orig }()
	shadowPath = path

	got, err := readShadowField("alice", 7)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1" {
		t.Errorf("readShadowField(alice, 7) = %q, want %q", got, "1")
	}

	got, err = readShadowField("bob", 7)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("readShadowField(bob, 7) = %q, want empty (no expiry)", got)
	}

	if _, err := readShadowField("nonexistent", 7); err == nil {
		t.Error("expected an error for an account with no shadow entry")
	}
}

func TestIsDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shadow")
	// Field index 7 (the 8th field, expire_date): "1" is epoch day 1
	// (1970-01-02, long past); "99999" is far in the future; empty means
	// never expires.
	content := "expired:x:19700:0:99999:7::1\n" +
		"future:x:19700:0:99999:7::99999\n" +
		"never:x:19700:0:99999:7::\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := shadowPath
	defer func() { shadowPath = orig }()
	shadowPath = path

	cases := map[string]bool{"expired": true, "future": false, "never": false}
	for username, want := range cases {
		got, err := IsDisabled(username)
		if err != nil {
			t.Fatalf("IsDisabled(%q): %v", username, err)
		}
		if got != want {
			t.Errorf("IsDisabled(%q) = %v, want %v", username, got, want)
		}
	}
}
