package changeledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotGitRepo is returned by InstallPostCommitHook when repoDir is not a git
// work tree — no .git directory and no worktree pointer file. Callers must
// treat it as a truthful "skipped" outcome; the function never fabricates a
// .git/ skeleton.
var ErrNotGitRepo = errors.New("changeledger: not a git work tree")

const (
	ledgerSentinelFile = "ledger-needs-update"
	hookMarker         = "flowpilot:ledger-hook"
)

// postCommitHook is the full standalone hook written when .git/hooks/post-commit
// does not yet exist.
const postCommitHook = "#!/bin/sh\n" +
	"# " + hookMarker + "\n" +
	"ROOT=\"$(git rev-parse --show-toplevel)\"\n" +
	"mkdir -p \"$ROOT/.flowpilot\"\n" +
	"touch \"$ROOT/.flowpilot/" + ledgerSentinelFile + "\" 2>/dev/null || true\n"

// postCommitHookAppend is added at the end of an existing hook that lacks the marker.
const postCommitHookAppend = "\n# " + hookMarker + "\n" +
	"ROOT=\"$(git rev-parse --show-toplevel)\"\n" +
	"mkdir -p \"$ROOT/.flowpilot\"\n" +
	"touch \"$ROOT/.flowpilot/" + ledgerSentinelFile + "\" 2>/dev/null || true\n"

// SentinelPath returns the path of the dirty-marker written by the post-commit hook.
func SentinelPath(dotFlowpilotDir string) string {
	return filepath.Join(dotFlowpilotDir, ledgerSentinelFile)
}

// InstallPostCommitHook writes (or augments) .git/hooks/post-commit so that each
// commit touches .flowpilot/ledger-needs-update. Safe rules:
//   - If the marker is already present: no-op.
//   - If the file exists but lacks the marker: append our block.
//   - If the file does not exist: write the standalone hook.
func InstallPostCommitHook(repoDir string) error {
	hookDir, err := resolveHooksDir(repoDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	hookPath := filepath.Join(hookDir, "post-commit")

	existing, err := os.ReadFile(hookPath)
	if err == nil {
		if strings.Contains(string(existing), hookMarker) {
			return nil // already installed
		}
		f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, writeErr := f.WriteString(postCommitHookAppend)
		f.Close()
		return writeErr
	}
	return os.WriteFile(hookPath, []byte(postCommitHook), 0o755)
}

// resolveHooksDir returns the real hooks directory for repoDir without
// creating anything: <repo>/.git/hooks for a normal repo, or the gitdir
// target's hooks/ for a linked worktree (where .git is a pointer file).
// Returns ErrNotGitRepo when repoDir is not a git work tree at all.
func resolveHooksDir(repoDir string) (string, error) {
	dotGit := filepath.Join(repoDir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotGitRepo
		}
		return "", err
	}
	if info.IsDir() {
		return filepath.Join(dotGit, "hooks"), nil
	}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:"); ok {
			gitdir := strings.TrimSpace(rest)
			if gitdir == "" {
				break
			}
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(repoDir, gitdir)
			}
			return filepath.Join(gitdir, "hooks"), nil
		}
	}
	return "", ErrNotGitRepo
}

// CheckAndRebuild checks for the dirty sentinel. When present, it runs an
// incremental ledger build (parse new commits → enrich → upsert) and removes the
// sentinel. Returns true when a rebuild was performed.
func CheckAndRebuild(repoDir, dotFlowpilotDir string) (bool, error) {
	sentinel := SentinelPath(dotFlowpilotDir)
	if _, err := os.Stat(sentinel); err != nil {
		return false, nil // sentinel absent — nothing to do
	}
	if err := Build(repoDir, dotFlowpilotDir); err != nil {
		return false, err
	}
	_ = os.Remove(sentinel)
	return true, nil
}
