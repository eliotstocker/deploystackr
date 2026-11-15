package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractPRNumber(t *testing.T) {
	cases := []struct {
		msg  string
		want int
	}{
		{"Merge pull request #123 from user/branch", 123},
		{"Merge PR #456", 456},
		{"feat: support multiple clusters (#789)", 789},
		{"See merge request group/project!321", 321},
		{"regular commit message without PR", 0},
	}

	for _, c := range cases {
		got := ExtractPRNumber(c.msg)
		if got != c.want {
			t.Errorf("ExtractPRNumber(%q) = %d; want %d", c.msg, got, c.want)
		}
	}
}

func TestExtractPRsFromText(t *testing.T) {
	text := `
- Added feature (#101)
- Fixed bug in https://github.com/cert-manager/cert-manager/pull/202
- Merge PR #303
- Duplicate #101 should be deduplicated
`
	prs := ExtractPRsFromText(text)
	if len(prs) != 3 {
		t.Fatalf("expected 3 PRs, got %d: %v", len(prs), prs)
	}
	if prs[0] != 303 || prs[1] != 202 || prs[2] != 101 {
		t.Errorf("expected [303, 202, 101], got %v", prs)
	}
}

func TestSetupGitAuth_CustomSSHKey(t *testing.T) {
	tmpDir := t.TempDir()
	dummyKey := filepath.Join(tmpDir, "test_id_ed25519")
	if err := os.WriteFile(dummyKey, []byte("fake-key"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SSH_KEY_PATH", dummyKey)
	t.Setenv("GIT_SSH_COMMAND", "")

	SetupGitAuth(tmpDir)

	sshCmd := os.Getenv("GIT_SSH_COMMAND")
	expected := "ssh -i " + dummyKey + " -o StrictHostKeyChecking=accept-new"
	if sshCmd != expected {
		t.Errorf("expected GIT_SSH_COMMAND=%q, got %q", expected, sshCmd)
	}
}

func TestSetupGitAuth_HTTPSAskpass(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GIT_ASKPASS", "")
	t.Setenv("GITHUB_TOKEN", "ghp_testtoken123")

	SetupGitAuth(tmpDir)

	askpass := os.Getenv("GIT_ASKPASS")
	if askpass == "" {
		t.Error("expected GIT_ASKPASS to be set when GITHUB_TOKEN is present")
	}
	if _, err := os.Stat(askpass); err != nil {
		t.Errorf("expected askpass script at %s to exist: %v", askpass, err)
	}
}

func TestResolveGitRef(t *testing.T) {
	tmpDir := t.TempDir()
	// Init git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()

	// Create dummy commit
	testFile := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)
	exec.Command("git", "-C", tmpDir, "add", "file.txt").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run()

	// Tag with v1.2.3
	exec.Command("git", "-C", tmpDir, "tag", "v1.2.3").Run()

	// Exact tag match
	if got := resolveGitRef(context.Background(), tmpDir, "v1.2.3"); got != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %s", got)
	}

	// Ref without v resolves to v1.2.3
	if got := resolveGitRef(context.Background(), tmpDir, "1.2.3"); got != "v1.2.3" {
		t.Errorf("expected v1.2.3 when querying 1.2.3, got %s", got)
	}

	// Non-existent ref returns input
	if got := resolveGitRef(context.Background(), tmpDir, "9.9.9"); got != "9.9.9" {
		t.Errorf("expected 9.9.9, got %s", got)
	}
}

func TestGetVersionDiffForwardAndReverse(t *testing.T) {
	tmpDir := t.TempDir()
	exec.Command("git", "-C", tmpDir, "init").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()

	f := filepath.Join(tmpDir, "app.txt")
	os.WriteFile(f, []byte("v1"), 0644)
	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "chore: initial v1.0.0").Run()
	exec.Command("git", "-C", tmpDir, "tag", "v1.0.0").Run()

	os.WriteFile(f, []byte("v2"), 0644)
	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "feat: add feature X (#12)").Run()
	exec.Command("git", "-C", tmpDir, "tag", "v2.0.0").Run()

	// 1. Forward diff: from v1.0.0 to v2.0.0
	fwd, err := GetVersionDiff(context.Background(), tmpDir, "v1.0.0", "v2.0.0")
	if err != nil {
		t.Fatalf("unexpected forward diff error: %v", err)
	}
	if fwd.IsRollback {
		t.Errorf("expected forward diff IsRollback to be false")
	}
	if len(fwd.Commits) != 1 {
		t.Fatalf("expected 1 commit in forward diff, got %d", len(fwd.Commits))
	}
	if fwd.Commits[0].Subject != "feat: add feature X (#12)" {
		t.Errorf("unexpected subject: %s", fwd.Commits[0].Subject)
	}
	if fwd.Commits[0].PRNumber != 12 {
		t.Errorf("expected PRNumber 12, got %d", fwd.Commits[0].PRNumber)
	}

	// 2. Reverse diff (Rollback): from active v2.0.0 to candidate v1.0.0
	rev, err := GetVersionDiff(context.Background(), tmpDir, "v2.0.0", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected reverse diff error: %v", err)
	}
	if !rev.IsRollback {
		t.Errorf("expected reverse diff IsRollback to be true")
	}
	if len(rev.Commits) != 1 {
		t.Fatalf("expected 1 commit in reverse diff, got %d", len(rev.Commits))
	}
	if rev.Commits[0].Subject != "feat: add feature X (#12)" {
		t.Errorf("unexpected subject in rollback: %s", rev.Commits[0].Subject)
	}
}

func TestFindGPGAndSSHKeys(t *testing.T) {
	tmpDir := t.TempDir()
	gpgKey := filepath.Join(tmpDir, "test.gpg")
	_ = os.WriteFile(gpgKey, []byte("fake-gpg"), 0600)

	t.Setenv("GPG_KEY_FILE", gpgKey)
	foundGPG := FindGPGKey()
	if foundGPG != gpgKey {
		t.Errorf("expected %s, got %s", gpgKey, foundGPG)
	}

	sshKey := filepath.Join(tmpDir, "test.ssh")
	_ = os.WriteFile(sshKey, []byte("fake-ssh"), 0600)

	t.Setenv("SSH_KEY_PATH", sshKey)
	foundSSH := FindSSHKey()
	if foundSSH != sshKey {
		t.Errorf("expected %s, got %s", sshKey, foundSSH)
	}
}

func TestEnsureSecureKeyPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "open_key")
	_ = os.WriteFile(keyPath, []byte("private-key-material"), 0644)

	scratch := filepath.Join(tmpDir, "scratch")
	secured := ensureSecureKeyPermissions(keyPath, scratch)

	fi, err := os.Stat(secured)
	if err != nil {
		t.Fatalf("stat secured key failed: %v", err)
	}
	if fi.Mode().Perm() > 0600 {
		t.Errorf("expected permissions <= 0600, got %o", fi.Mode().Perm())
	}
}

func TestCleanVersionRef(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"1.0.0", "1.0.0"},
		{"v1.0.0", "v1.0.0"},
		{"ghcr.io/org/app:1.0.0", "1.0.0"},
		{"octoeverywhere/octoeverywhere:5.4.1", "5.4.1"},
		{"app:v2.0.0@sha256:abcdef", "v2.0.0"},
	}
	for _, c := range cases {
		if got := CleanVersionRef(c.input); got != c.expected {
			t.Errorf("CleanVersionRef(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestGetVersionDiff_WithTagPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	exec.Command("git", "-C", tmpDir, "init").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()

	f := filepath.Join(tmpDir, "app.txt")
	os.WriteFile(f, []byte("v1"), 0644)
	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "chore: v1.0.0").Run()
	exec.Command("git", "-C", tmpDir, "tag", "release-1.0.0").Run()

	os.WriteFile(f, []byte("v2"), 0644)
	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "feat: new feature").Run()
	exec.Command("git", "-C", tmpDir, "tag", "release-2.0.0").Run()

	// Diff using OCI-style bare versions "1.0.0" and "2.0.0" with tagPrefix "release-"
	diff, err := GetVersionDiff(context.Background(), tmpDir, "1.0.0", "2.0.0", "release-")
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if len(diff.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(diff.Commits))
	}

	// Diff with full image refs and tagPrefix "release-"
	diffImage, err := GetVersionDiff(context.Background(), tmpDir, "docker.io/repo/app:1.0.0", "docker.io/repo/app:2.0.0", "release-")
	if err != nil {
		t.Fatalf("unexpected diff error with image refs: %v", err)
	}
	if len(diffImage.Commits) != 1 {
		t.Fatalf("expected 1 commit with image refs, got %d", len(diffImage.Commits))
	}
}
