package sshaccess

import "testing"

const testEd25519 = "ZWQyNTUxOS1mYWtlLWtleS1tYXRlcmlhbC1mb3ItdW5pdC10ZXN0cw=="

func TestBuildAndParseLineRoundTrip(t *testing.T) {
	line := buildLine("/var/lib/ngxborg/repos/alice/mybackup", false, "ssh-ed25519", testEd25519, "alice@laptop")
	entry, ok := parseLine(line)
	if !ok {
		t.Fatalf("parseLine did not recognise its own output: %q", line)
	}
	if entry.RepoPath != "/var/lib/ngxborg/repos/alice/mybackup" {
		t.Errorf("RepoPath = %q", entry.RepoPath)
	}
	if entry.AppendOnly {
		t.Error("AppendOnly should be false")
	}
	if entry.KeyType != "ssh-ed25519" || entry.KeyMaterial != testEd25519 {
		t.Errorf("key type/material = %q/%q", entry.KeyType, entry.KeyMaterial)
	}
	if entry.Comment != "alice@laptop" {
		t.Errorf("Comment = %q", entry.Comment)
	}
}

func TestBuildAndParseLineAppendOnly(t *testing.T) {
	line := buildLine("/var/lib/ngxborg/repos/bob/nightly", true, "ssh-ed25519", testEd25519, "")
	entry, ok := parseLine(line)
	if !ok {
		t.Fatalf("parseLine did not recognise its own output: %q", line)
	}
	if !entry.AppendOnly {
		t.Error("AppendOnly should be true")
	}
	if entry.RepoPath != "/var/lib/ngxborg/repos/bob/nightly" {
		t.Errorf("RepoPath = %q, should not include the --append-only flag", entry.RepoPath)
	}
	if entry.Comment != "" {
		t.Errorf("Comment = %q, want empty", entry.Comment)
	}
}

func TestParseLineRejectsForeignLines(t *testing.T) {
	foreign := []string{
		"ssh-ed25519 " + testEd25519 + " operator@laptop",
		`command="/bin/rsync --server",no-pty ssh-rsa AAAAB3NzaC1yc2E...`,
		"",
		"# a comment",
	}
	for _, line := range foreign {
		if _, ok := parseLine(line); ok {
			t.Errorf("parseLine incorrectly claimed a foreign line: %q", line)
		}
	}
}

func TestLineKeyMaterialMatchesAcrossRetargeting(t *testing.T) {
	a := buildLine("/var/lib/ngxborg/repos/alice/one", false, "ssh-ed25519", testEd25519, "old")
	b := buildLine("/var/lib/ngxborg/repos/alice/two", true, "ssh-ed25519", testEd25519, "new")
	if lineKeyMaterial(a) != lineKeyMaterial(b) {
		t.Error("the same key material retargeted to a different repo should still match for retargeting/removal")
	}
}

func TestValidatePublicKeyLine(t *testing.T) {
	good := "ssh-ed25519 " + testEd25519 + " alice@laptop"
	typ, material, comment, err := validatePublicKeyLine(good)
	if err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if typ != "ssh-ed25519" || material != testEd25519 || comment != "alice@laptop" {
		t.Errorf("got %q/%q/%q", typ, material, comment)
	}

	bad := []string{
		"",
		"not-a-key-type " + testEd25519,
		"ssh-ed25519",
		"ssh-ed25519 not-valid-base64!!!",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----",
	}
	for _, line := range bad {
		if _, _, _, err := validatePublicKeyLine(line); err == nil {
			t.Errorf("validatePublicKeyLine(%q) should have failed", line)
		}
	}
}

func TestFormatCount(t *testing.T) {
	cases := map[int]string{0: "0 keys", 1: "1 key", 2: "2 keys"}
	for n, want := range cases {
		if got := FormatCount(n); got != want {
			t.Errorf("FormatCount(%d) = %q, want %q", n, got, want)
		}
	}
}
