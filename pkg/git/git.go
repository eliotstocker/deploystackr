package git

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"deploystackr/pkg/config"
)

// CommitInfo holds metadata for a Git commit.
type CommitInfo struct {
	SHA         string    `json:"sha"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Timestamp   time.Time `json:"timestamp"`
	Subject     string    `json:"subject"`
	PRNumber    int       `json:"pr_number,omitempty"`
}

// DiffEntry represents a commit entry in a version diff / changelog.
type DiffEntry struct {
	SHA         string    `json:"sha"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Timestamp   time.Time `json:"timestamp"`
	Subject     string    `json:"subject"`
	PRNumber    int       `json:"pr_number,omitempty"`
}

// ChangelogDiff represents the comparison between two releases/tags.
type ChangelogDiff struct {
	FromVersion string      `json:"from_version"`
	ToVersion   string      `json:"to_version"`
	Commits     []DiffEntry `json:"commits"`
	DiffStat    string      `json:"diff_stat,omitempty"`
	RawDiff     string      `json:"raw_diff,omitempty"`
	IsRollback  bool        `json:"is_rollback,omitempty"`
}

var (
	// Regex matching PR numbers from merge commit messages
	githubPRRegex  = regexp.MustCompile(`(?i)(?:Merge pull request #|Merge PR #|pull request #|\(#)(\d+)\)?`)
	gitlabMRRegex  = regexp.MustCompile(`(?i)(?:See merge request [^\s]+!|merge request !|!)(\d+)`)
	prURLRegex     = regexp.MustCompile(`(?i)(?:/pull/|/merge_requests/)(\d+)`)
	prGenericRegex = regexp.MustCompile(`(?i)(?:#|GH-)(\d+)`)
)

// ExtractPRNumber parses a PR/MR number from a commit subject or body.
func ExtractPRNumber(message string) int {
	if matches := githubPRRegex.FindStringSubmatch(message); len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	if matches := gitlabMRRegex.FindStringSubmatch(message); len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	return 0
}

// ExtractPRsFromText extracts all unique PR numbers from release notes or changelog bodies.
func ExtractPRsFromText(text string) []int {
	if text == "" {
		return nil
	}
	seen := make(map[int]bool)
	var prs []int

	for _, re := range []*regexp.Regexp{prURLRegex, githubPRRegex, gitlabMRRegex, prGenericRegex} {
		matches := re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 1000000 && !seen[n] {
					seen[n] = true
					prs = append(prs, n)
				}
			}
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(prs)))
	return prs
}

// SetupGitAuth initializes the environment for non-interactive Git operations via SSH and HTTPS.
func SetupGitAuth(scratchDir string) {
	// Always disable terminal interactive prompts
	_ = os.Setenv("GIT_TERMINAL_PROMPT", "0")

	// Avoid dubious ownership errors across volume mounts in containers
	_ = exec.Command("git", "config", "--global", "--add", "safe.directory", "*").Run()

	// SSH configuration
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		if keyPath := FindSSHKey(); keyPath != "" {
			secureKey := ensureSecureKeyPermissions(keyPath, scratchDir)
			_ = os.Setenv("GIT_SSH_COMMAND", fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new", secureKey))
			log.Printf("[git] SSH authentication configured using key: %s (StrictHostKeyChecking=accept-new)", secureKey)
		} else {
			_ = os.Setenv("GIT_SSH_COMMAND", "ssh -o StrictHostKeyChecking=accept-new")
			log.Printf("[git] SSH authentication configured with default: ssh -o StrictHostKeyChecking=accept-new")
		}
	}

	// Ensure ~/.ssh exists with strict permissions
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		_ = os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	}

	// HTTPS / PAT authentication
	if os.Getenv("GIT_ASKPASS") == "" {
		hasToken := config.GetGitHubToken() != "" || os.Getenv("GIT_TOKEN") != "" || os.Getenv("GIT_PASSWORD") != ""
		if hasToken {
			if _, err := os.Stat("/usr/local/bin/git-askpass.sh"); err == nil {
				_ = os.Setenv("GIT_ASKPASS", "/usr/local/bin/git-askpass.sh")
				log.Printf("[git] HTTPS authentication configured using /usr/local/bin/git-askpass.sh")
			} else {
				askpassDir := scratchDir
				if askpassDir == "" {
					askpassDir = os.TempDir()
				}
				_ = os.MkdirAll(askpassDir, 0755)
				askpassScript := filepath.Join(askpassDir, "git-askpass.sh")
				scriptContent := `#!/bin/sh
case "$1" in
    *Username*|*username*)
        echo "${GIT_USERNAME:-x-access-token}"
        ;;
    *)
        echo "${GITHUB_TOKEN:-${GIT_TOKEN:-$GIT_PASSWORD}}"
        ;;
esac
`
				if err := os.WriteFile(askpassScript, []byte(scriptContent), 0700); err == nil {
					_ = os.Setenv("GIT_ASKPASS", askpassScript)
					log.Printf("[git] HTTPS authentication configured using %s", askpassScript)
				}
			}

			if os.Getenv("GIT_USERNAME") == "" {
				_ = os.Setenv("GIT_USERNAME", "x-access-token")
			}
		}
	}

	// Configure GPG or SSH commit signing
	SetupGitSigning(scratchDir)
}

// FindSSHKey looks for mounted or standard SSH private keys.
func FindSSHKey() string {
	if custom := os.Getenv("SSH_KEY_PATH"); custom != "" {
		if fi, err := os.Stat(custom); err == nil && !fi.IsDir() {
			return custom
		}
	}

	candidates := []string{
		"/etc/deploystackr/ssh/id_rsa",
		"/etc/deploystackr/ssh/id_ed25519",
		"/etc/deploystackr/ssh/id_ecdsa",
		"/etc/deploystackr/ssh/ssh-privatekey",
		"/etc/deploystackr/secrets/id_rsa",
		"/etc/deploystackr/secrets/id_ed25519",
		"/etc/deploystackr/secrets/ssh-privatekey",
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".ssh", "id_rsa"),
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_ecdsa"),
		)
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// FindGPGKey looks for mounted GPG private key files.
func FindGPGKey() string {
	if custom := os.Getenv("GPG_KEY_FILE"); custom != "" {
		if fi, err := os.Stat(custom); err == nil && !fi.IsDir() {
			return custom
		}
	}
	if custom := os.Getenv("GPG_KEY_PATH"); custom != "" {
		if fi, err := os.Stat(custom); err == nil && !fi.IsDir() {
			return custom
		}
	}

	candidates := []string{
		"/etc/deploystackr/gpg/signing.key",
		"/etc/deploystackr/gpg/secring.gpg",
		"/etc/deploystackr/gpg/private.key",
		"/etc/deploystackr/secrets/gpg-key",
		"/etc/deploystackr/secrets/signing.key",
		"/etc/deploystackr/secrets/gpg-signing-key",
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// SetupGitSigning configures commit signing via mounted GPG or SSH keys.
func SetupGitSigning(scratchDir string) {
	// 1. Check GPG Key
	if gpgKey := FindGPGKey(); gpgKey != "" {
		cmd := exec.Command("gpg", "--batch", "--yes", "--import", gpgKey)
		if out, err := cmd.CombinedOutput(); err == nil {
			listCmd := exec.Command("gpg", "--list-secret-keys", "--with-colons")
			if listOut, err := listCmd.Output(); err == nil {
				keyID := extractGPGKeyID(string(listOut))
				if keyID != "" {
					_ = exec.Command("git", "config", "--global", "user.signingkey", keyID).Run()
					_ = exec.Command("git", "config", "--global", "commit.gpgsign", "true").Run()
					_ = exec.Command("git", "config", "--global", "gpg.program", "gpg").Run()
					log.Printf("[git] GPG commit signing configured using key ID: %s", keyID)
					return
				}
			}
		} else {
			log.Printf("[git] Notice: gpg import from %s: %v (%s)", gpgKey, err, string(out))
		}
	}

	// 2. Check SSH Commit Signing
	sshSigningKey := os.Getenv("SSH_SIGNING_KEY_PATH")
	if sshSigningKey == "" && strings.EqualFold(os.Getenv("SIGN_COMMITS_WITH_SSH"), "true") {
		sshSigningKey = FindSSHKey()
	}
	if sshSigningKey != "" {
		if fi, err := os.Stat(sshSigningKey); err == nil && !fi.IsDir() {
			secureKey := ensureSecureKeyPermissions(sshSigningKey, scratchDir)
			_ = exec.Command("git", "config", "--global", "gpg.format", "ssh").Run()
			_ = exec.Command("git", "config", "--global", "user.signingkey", secureKey).Run()
			_ = exec.Command("git", "config", "--global", "commit.gpgsign", "true").Run()
			log.Printf("[git] SSH commit signing configured using key: %s", secureKey)
		}
	}
}

func extractGPGKeyID(colonOutput string) string {
	lines := strings.Split(colonOutput, "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) > 4 && (parts[0] == "sec" || parts[0] == "fpr") {
			if len(parts[4]) >= 8 {
				return parts[4]
			}
		}
	}
	return ""
}

func ensureSecureKeyPermissions(keyPath, scratchDir string) string {
	fi, err := os.Stat(keyPath)
	if err != nil {
		return keyPath
	}
	if fi.Mode().Perm() <= 0600 {
		return keyPath
	}

	// Try chmod in place
	if err := os.Chmod(keyPath, 0600); err == nil {
		return keyPath
	}

	// If read-only mount, copy to scratchDir with 0600
	if scratchDir != "" {
		safeDir := filepath.Join(scratchDir, "secure_keys")
		_ = os.MkdirAll(safeDir, 0700)
		safePath := filepath.Join(safeDir, filepath.Base(keyPath))
		data, err := os.ReadFile(keyPath)
		if err == nil {
			if err := os.WriteFile(safePath, data, 0600); err == nil {
				return safePath
			}
		}
	}

	return keyPath
}

// isGitRepo checks if a directory is a valid git repo or bare clone.
func isGitRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	return false
}

// CloneOrFetch ensures a repository is cloned and up-to-date in targetDir.
func CloneOrFetch(ctx context.Context, repoURL, branch, targetDir string, bare bool) error {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) || !isGitRepo(targetDir) {
		_ = os.RemoveAll(targetDir)
		args := []string{"clone"}
		if bare {
			args = append(args, "--bare")
		}
		if branch != "" {
			args = append(args, "--branch", branch)
		}
		args = append(args, repoURL, targetDir)

		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone failed (%s): %s: %w", errOut.String(), out.String(), err)
		}
		return nil
	}

	// Target exists, fetch latest updates
	var fetchArgs []string
	if bare {
		fetchArgs = []string{"fetch", "origin", "+refs/heads/*:refs/heads/*", "--tags"}
	} else {
		fetchArgs = []string{"fetch", "origin"}
	}
	fetchCmd := exec.CommandContext(ctx, "git", fetchArgs...)
	fetchCmd.Dir = targetDir
	fetchCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		fallbackCmd := exec.CommandContext(ctx, "git", "fetch", "--all")
		fallbackCmd.Dir = targetDir
		fallbackCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if _, fallbackErr := fallbackCmd.CombinedOutput(); fallbackErr != nil {
			return fmt.Errorf("git fetch failed (%s): %w", string(out), err)
		}
	}

	return nil
}

// LogFileCommits runs `git log --follow` on targetFile and returns commits modifying it in reverse chronological order.
func LogFileCommits(ctx context.Context, repoDir, branch, targetFile string) ([]CommitInfo, error) {
	// Format: %H|%an|%ae|%aI|%s
	format := "%H%x09%an%x09%ae%x09%aI%x09%s"
	args := []string{
		"log",
		"--follow",
		fmt.Sprintf("--format=%s", format),
	}
	if branch != "" {
		args = append(args, branch)
	}
	args = append(args, "--", targetFile)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	var commits []CommitInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		sha := parts[0]
		authorName := parts[1]
		authorEmail := parts[2]
		ts, _ := time.Parse(time.RFC3339, parts[3])
		subject := parts[4]

		pr := ExtractPRNumber(subject)

		commits = append(commits, CommitInfo{
			SHA:         sha,
			AuthorName:  authorName,
			AuthorEmail: authorEmail,
			Timestamp:   ts,
			Subject:     subject,
			PRNumber:    pr,
		})
	}

	return commits, nil
}

// ShowFileAtCommit retrieves the raw content of targetFile at a specific commit.
func ShowFileAtCommit(ctx context.Context, repoDir, commitSHA, targetFile string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "show", fmt.Sprintf("%s:%s", commitSHA, targetFile))
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s failed: %w", commitSHA, targetFile, err)
	}
	return out, nil
}

// CleanVersionRef strips image repository names and digest suffixes from a version reference.
func CleanVersionRef(v string) string {
	v = strings.TrimSpace(v)
	if idx := strings.Index(v, "@"); idx != -1 {
		v = v[:idx]
	}
	if idx := strings.LastIndex(v, ":"); idx != -1 {
		v = v[idx+1:]
	}
	return v
}

// resolveGitRef checks if ref exists directly, or falls back to resolving with prefix or with/without 'v' prefix.
func resolveGitRef(ctx context.Context, repoDir, ref string, tagPrefix ...string) string {
	ref = CleanVersionRef(ref)
	if ref == "" {
		return ""
	}

	pfx := ""
	if len(tagPrefix) > 0 && tagPrefix[0] != "" {
		pfx = tagPrefix[0]
	}

	//check with explicit tag prefix first if configured
	if pfx != "" {
		if !strings.HasPrefix(ref, pfx) {
			pfxRef := pfx + ref
			cmdP := exec.CommandContext(ctx, "git", "rev-parse", "--verify", pfxRef+"^{commit}")
			cmdP.Dir = repoDir
			if err := cmdP.Run(); err == nil {
				return pfxRef
			}
		} else {
			trimmedRef := strings.TrimPrefix(ref, pfx)
			cmdT := exec.CommandContext(ctx, "git", "rev-parse", "--verify", trimmedRef+"^{commit}")
			cmdT.Dir = repoDir
			if err := cmdT.Run(); err == nil {
				return trimmedRef
			}
		}
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", ref+"^{commit}")
	cmd.Dir = repoDir
	if err := cmd.Run(); err == nil {
		return ref
	}
	if !strings.HasPrefix(ref, "v") {
		vRef := "v" + ref
		cmdV := exec.CommandContext(ctx, "git", "rev-parse", "--verify", vRef+"^{commit}")
		cmdV.Dir = repoDir
		if err := cmdV.Run(); err == nil {
			return vRef
		}
	} else {
		cleanRef := strings.TrimPrefix(ref, "v")
		cmdC := exec.CommandContext(ctx, "git", "rev-parse", "--verify", cleanRef+"^{commit}")
		cmdC.Dir = repoDir
		if err := cmdC.Run(); err == nil {
			return cleanRef
		}
	}
	return ref
}

// GetVersionDiff compares two git refs/tags and produces commit list and diff.
func GetVersionDiff(ctx context.Context, repoDir, fromRef, toRef string, tagPrefix ...string) (*ChangelogDiff, error) {
	pfx := ""
	if len(tagPrefix) > 0 {
		pfx = tagPrefix[0]
	}
	resolvedFrom := resolveGitRef(ctx, repoDir, fromRef, pfx)
	resolvedTo := resolveGitRef(ctx, repoDir, toRef, pfx)

	format := "%H%x09%an%x09%ae%x09%aI%x09%s"

	// Check if this is a rollback / reverse diff (target toRef is older than fromRef).
	isRollback := false
	var logArgs []string
	var diffArgs []string

	if resolvedFrom != "" && resolvedTo != "" {
		// Check whether fromRef is an ancestor of toRef (forward diff)
		checkAncestor := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", resolvedFrom, resolvedTo)
		checkAncestor.Dir = repoDir
		if checkAncestor.Run() == nil {
			// Normal forward upgrade: fromRef..toRef
			logArgs = []string{"log", fmt.Sprintf("--format=%s", format), fmt.Sprintf("%s..%s", resolvedFrom, resolvedTo)}
			diffArgs = []string{"diff", "--shortstat", resolvedFrom, resolvedTo}
		} else {
			// Reverse/rollback diff: toRef is older or behind fromRef.
			// Show what changed between toRef and fromRef (commits present in fromRef that will be removed / superseded).
			isRollback = true
			logArgs = []string{"log", fmt.Sprintf("--format=%s", format), "--left-only", "--cherry-pick", fmt.Sprintf("%s...%s", resolvedFrom, resolvedTo)}
			diffArgs = []string{"diff", "--shortstat", resolvedFrom, resolvedTo}
		}
	} else if resolvedFrom == "" {
		logArgs = []string{"log", fmt.Sprintf("--format=%s", format), resolvedTo}
		diffArgs = []string{"diff", "--shortstat", resolvedTo}
	} else {
		logArgs = []string{"log", fmt.Sprintf("--format=%s", format), resolvedFrom}
		diffArgs = []string{"diff", "--shortstat", resolvedFrom}
	}

	cmd := exec.CommandContext(ctx, "git", logArgs...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	var commits []DiffEntry
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, parts[3])
		if IsReleaseCommit(parts[4]) {
			continue
		}
		commits = append(commits, DiffEntry{
			SHA:         parts[0],
			AuthorName:  parts[1],
			AuthorEmail: parts[2],
			Timestamp:   ts,
			Subject:     parts[4],
			PRNumber:    ExtractPRNumber(parts[4]),
		})
	}

	// Fetch diffstat (shortstat summary line)
	statCmd := exec.CommandContext(ctx, "git", diffArgs...)
	statCmd.Dir = repoDir
	statOut, _ := statCmd.Output()
	diffStat := strings.TrimSpace(string(statOut))
	if diffStat == "" {
		diffStat = "0 files changed"
	} else if idx := strings.LastIndex(diffStat, "\n"); idx != -1 {
		diffStat = strings.TrimSpace(diffStat[idx+1:])
	}

	return &ChangelogDiff{
		FromVersion: fromRef,
		ToVersion:   toRef,
		Commits:     commits,
		DiffStat:    diffStat,
		IsRollback:  isRollback,
	}, nil
}

// IsReleaseCommit detects automated or tag bump commits to skip in changelog views.
func IsReleaseCommit(subject string) bool {
	s := strings.ToLower(strings.TrimSpace(subject))
	return strings.HasPrefix(s, "chore(release") ||
		strings.HasPrefix(s, "release:") ||
		strings.HasPrefix(s, "release(") ||
		strings.HasPrefix(s, "build(release") ||
		strings.HasPrefix(s, "chore: release") ||
		strings.HasPrefix(s, "bump version") ||
		strings.HasPrefix(s, "version bump") ||
		strings.HasPrefix(s, "release v") ||
		strings.HasPrefix(s, "bump to v")
}
