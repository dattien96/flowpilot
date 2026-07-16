package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// errGitCommitGuardRequired is returned when ForceShellBridge is set but the
// pre-exec git guard could not be installed. Fail closed: do not run the turn.
var errGitCommitGuardRequired = fmt.Errorf("FlowPilot git commit guard required for ForceShellBridge but install failed")

// errGitMainRepoMutated is returned when the primary workspace refs or baseline
// file identity changed during a ForceShellBridge Gemini turn. Fail closed —
// never rewrite shared history or clobber concurrent dirty edits.
var errGitMainRepoMutated = fmt.Errorf("FlowPilot ForceShellBridge: primary workspace changed during turn; refusing unsafe apply")

// gitHeadCheckpoint captures primary-workspace identity for a ForceShellBridge
// turn. Agent work always runs in an isolated sandbox; main is only mutated by
// an all-or-nothing copy-back after conflict checks.
type gitHeadCheckpoint struct {
	SHA       string // HEAD when born; empty if unborn
	Unborn    bool
	MainCwd   string
	// Refs maps full ref name → tip SHA (all heads/tags at turn start).
	Refs map[string]string
	// Files maps repo-relative path → identity fingerprint (mode|type|hash/target).
	// Captures tracked + dirty worktree so concurrent main edits are detected.
	Files map[string]string
}

// installGitCommitGuard creates a temp dir with a hardened `git` shim that
// blocks commit-creating invocations before they execute.
func installGitCommitGuard(forceShellBridge bool) (guardDir string, cleanup func(), err error) {
	cleanup = func() {}
	if !forceShellBridge {
		return "", cleanup, nil
	}
	realGit, lookErr := exec.LookPath("git")
	if lookErr != nil || strings.TrimSpace(realGit) == "" {
		realGit = "/usr/bin/git"
	}
	if abs, absErr := filepath.Abs(realGit); absErr == nil {
		realGit = abs
	}
	dir, mkErr := os.MkdirTemp("", "fp-git-guard-*")
	if mkErr != nil {
		return "", cleanup, mkErr
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	shimPath := filepath.Join(dir, "git")
	// Block commit + -C / --git-dir pointing outside the agent sandbox when
	// FLOWPILOT_GIT_SANDBOX is set (isolates absolute-path git to main).
	script := fmt.Sprintf(`#!/bin/sh
REAL_GIT=%q
SANDBOX="${FLOWPILOT_GIT_SANDBOX:-}"
deny() {
  echo "FlowPilot gate: $1" >&2
  exit 1
}
# Block commits.
alias_commit_names=""
i=1
while [ "$i" -le "$#" ]; do
  eval "a=\${$i}"
  case "$a" in
    -C|--git-dir|--work-tree|--namespace|--config-env)
      i=$((i+1))
      eval "val=\${$i}"
      if [ -n "$SANDBOX" ]; then
        case "$val" in
          "$SANDBOX"|"$SANDBOX"/*) ;;
          *) deny "git -C/--git-dir outside sandbox is blocked (ForceShellBridge)" ;;
        esac
      fi
      i=$((i+1)); continue ;;
    -c)
      i=$((i+1))
      eval "cfg=\${$i}"
      case "$cfg" in
        alias.*=*)
          name=$(printf '%%s' "$cfg" | sed -n 's/^alias\.\([^=]*\)=.*/\1/p')
          val=$(printf '%%s' "$cfg" | sed -n 's/^alias\.[^=]*=//p')
          case " $val " in
            *" commit "*|commit|commit\ *|*\ commit|*"commit-tree"*|!git\ commit*|!git\ commit-tree*)
              alias_commit_names="$alias_commit_names $name "
              ;;
          esac
          ;;
      esac
      i=$((i+1)); continue ;;
    --git-dir=*|--work-tree=*)
      if [ -n "$SANDBOX" ]; then
        val=$(printf '%%s' "$a" | sed 's/^[^=]*=//')
        case "$val" in
          "$SANDBOX"|"$SANDBOX"/*) ;;
          *) deny "git --git-dir/--work-tree outside sandbox is blocked (ForceShellBridge)" ;;
        esac
      fi
      i=$((i+1)); continue ;;
    --namespace=*|--config-env=*|-c*)
      i=$((i+1)); continue ;;
    --)
      i=$((i+1)); break ;;
    -*)
      i=$((i+1)); continue ;;
    *)
      break ;;
  esac
done
if [ "$i" -le "$#" ]; then
  eval "sub=\${$i}"
  case "$sub" in
    commit|commit-tree) deny "git commit is blocked for this agent (ForceShellBridge)" ;;
  esac
  case "$alias_commit_names" in
    *" $sub "*) deny "git commit alias is blocked for this agent (ForceShellBridge)" ;;
  esac
fi
j=1
while [ "$j" -le "$#" ]; do
  eval "t=\${$j}"
  if [ "$t" = "commit" ] || [ "$t" = "commit-tree" ]; then
    deny "git commit is blocked for this agent (ForceShellBridge)"
  fi
  j=$((j+1))
done
exec "$REAL_GIT" "$@"
`, realGit)
	if werr := os.WriteFile(shimPath, []byte(script), 0o755); werr != nil {
		cleanup()
		return "", func() {}, werr
	}
	if st, stErr := os.Stat(shimPath); stErr != nil || st.Mode()&0o111 == 0 {
		cleanup()
		return "", func() {}, fmt.Errorf("git guard shim not executable: %v", stErr)
	}
	return dir, cleanup, nil
}

func envWithGitCommitGuard(overrides map[string]string, guardDir, sandbox string) map[string]string {
	out := make(map[string]string, len(overrides)+2)
	for k, v := range overrides {
		out[k] = v
	}
	if strings.TrimSpace(guardDir) != "" {
		base := out["PATH"]
		if base == "" {
			base = os.Getenv("PATH")
		}
		out["PATH"] = guardDir + string(os.PathListSeparator) + base
	}
	if strings.TrimSpace(sandbox) != "" {
		out["FLOWPILOT_GIT_SANDBOX"] = sandbox
	}
	return out
}

// captureGitHeadCheckpoint records HEAD, all refs, and a worktree file baseline.
func captureGitHeadCheckpoint(cwd string) (gitHeadCheckpoint, error) {
	cwd = strings.TrimSpace(cwd)
	cp := gitHeadCheckpoint{MainCwd: cwd, Refs: map[string]string{}, Files: map[string]string{}}
	if cwd == "" {
		return cp, fmt.Errorf("empty cwd")
	}
	head, err := captureGitHead(cwd)
	if err == nil && strings.TrimSpace(head) != "" {
		cp.SHA = head
	} else if isUnbornGitRepo(cwd) {
		cp.Unborn = true
	} else if err != nil {
		return cp, err
	} else {
		return cp, fmt.Errorf("empty HEAD")
	}
	refs, rerr := listGitRefs(cwd)
	if rerr != nil {
		// Non-fatal for non-git dirs; otherwise fail closed.
		if !cp.Unborn && cp.SHA == "" {
			return cp, rerr
		}
	} else {
		cp.Refs = refs
	}
	files, ferr := snapshotWorkspaceFiles(cwd)
	if ferr != nil {
		return cp, ferr
	}
	cp.Files = files
	return cp, nil
}

func isUnbornGitRepo(cwd string) bool {
	cmd := exec.Command("git", "-C", cwd, "rev-list", "-n", "1", "--all")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) == ""
}

func listGitRefs(cwd string) (map[string]string, error) {
	out, err := exec.Command("git", "-C", cwd, "for-each-ref", "--format=%(refname) %(objectname)").Output()
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[0]] = parts[1]
	}
	return m, nil
}

// snapshotWorkspaceFiles fingerprints tracked + untracked paths under cwd
// (relative to repo root). Symlinks use "link|target"; regular files use
// "file|mode|sha256"; missing paths are omitted.
func snapshotWorkspaceFiles(cwd string) (map[string]string, error) {
	out := map[string]string{}
	// Tracked files (incl. from index).
	tracked, _ := exec.Command("git", "-C", cwd, "ls-files", "-z").Output()
	// Untracked (dirty worktree).
	untracked, _ := exec.Command("git", "-C", cwd, "ls-files", "--others", "--exclude-standard", "-z").Output()
	// Also include dirty deletions as empty markers via status porcelain.
	paths := append(splitGitNUL(tracked), splitGitNUL(untracked)...)
	// When not a git repo or empty, walk top-level files (unborn / plain dir).
	if len(paths) == 0 {
		_ = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == ".flowpilot" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, rerr := filepath.Rel(cwd, path)
			if rerr != nil {
				return nil
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		fp, err := fileIdentityFingerprint(cwd, p)
		if err != nil {
			// Deleted tracked file: record as deleted sentinel so concurrent
			// recreation is still conflict-detected.
			if os.IsNotExist(err) {
				out[p] = "missing"
				continue
			}
			return nil, err
		}
		out[p] = fp
	}
	return out, nil
}

func fileIdentityFingerprint(cwd, rel string) (string, error) {
	full := filepath.Join(cwd, filepath.FromSlash(rel))
	st, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	mode := st.Mode()
	if mode&os.ModeSymlink != 0 {
		target, lerr := os.Readlink(full)
		if lerr != nil {
			return "", lerr
		}
		return "link|" + target, nil
	}
	if !mode.IsRegular() {
		return fmt.Sprintf("other|%o", mode.Perm()), nil
	}
	f, oerr := os.Open(full)
	if oerr != nil {
		return "", oerr
	}
	defer f.Close()
	h := sha256.New()
	if _, cerr := io.Copy(h, f); cerr != nil {
		return "", cerr
	}
	return fmt.Sprintf("file|%o|%s", mode.Perm(), hex.EncodeToString(h.Sum(nil))), nil
}

// prepareForceShellBridgeWorktree creates a disposable isolated sandbox for
// the Gemini turn (born: detached worktree; unborn: private copy with no main
// .git access). Agent never runs with mainCwd as Dir when isolation succeeds.
func prepareForceShellBridgeWorktree(mainCwd string, base gitHeadCheckpoint) (worktreePath string, cleanup func(), err error) {
	cleanup = func() {}
	mainCwd = strings.TrimSpace(mainCwd)
	if mainCwd == "" {
		return "", cleanup, fmt.Errorf("empty main cwd")
	}
	parent := filepath.Join(mainCwd, ".flowpilot", "gemini-wt")
	if mkErr := os.MkdirAll(parent, 0o755); mkErr != nil {
		return "", cleanup, mkErr
	}
	dir, mkErr := os.MkdirTemp(parent, "turn-*")
	if mkErr != nil {
		return "", cleanup, mkErr
	}
	_ = os.RemoveAll(dir)

	if base.Unborn || strings.TrimSpace(base.SHA) == "" {
		// Unborn / no HEAD: full file copy into private dir (not a git worktree
		// of main). Agent cannot reach main .git via relative paths.
		if cpErr := copyDirPreserve(mainCwd, dir, map[string]bool{".git": true, ".flowpilot": true}); cpErr != nil {
			_ = os.RemoveAll(dir)
			return "", cleanup, fmt.Errorf("isolate unborn workspace: %w", cpErr)
		}
		// Private empty git so tools that need a repo still work locally.
		_ = exec.Command("git", "-C", dir, "init").Run()
		cleanup = func() { _ = os.RemoveAll(dir) }
		return dir, cleanup, nil
	}

	add := exec.Command("git", "-C", mainCwd, "worktree", "add", "--detach", dir, base.SHA)
	if out, addErr := add.CombinedOutput(); addErr != nil {
		_ = os.RemoveAll(dir)
		return "", cleanup, fmt.Errorf("git worktree add: %w (%s)", addErr, strings.TrimSpace(string(out)))
	}
	// Overlay main's dirty (uncommitted) files so the agent sees the same tree.
	if overlayErr := overlayDirtyFromMain(mainCwd, dir); overlayErr != nil {
		_ = exec.Command("git", "-C", mainCwd, "worktree", "remove", "--force", dir).Run()
		_ = os.RemoveAll(dir)
		return "", cleanup, overlayErr
	}
	cleanup = func() {
		_ = exec.Command("git", "-C", mainCwd, "worktree", "remove", "--force", dir).Run()
		_ = os.RemoveAll(dir)
		_ = exec.Command("git", "-C", mainCwd, "worktree", "prune").Run()
	}
	return dir, cleanup, nil
}

func overlayDirtyFromMain(mainCwd, worktreePath string) error {
	// Uncommitted tracked changes + untracked (exclude .flowpilot).
	out, _ := exec.Command("git", "-C", mainCwd, "diff", "--name-only", "-z", "HEAD").Output()
	ut, _ := exec.Command("git", "-C", mainCwd, "ls-files", "--others", "--exclude-standard", "-z").Output()
	paths := append(splitGitNUL(out), splitGitNUL(ut)...)
	for _, p := range paths {
		if p == "" || strings.HasPrefix(p, ".flowpilot/") {
			continue
		}
		src := filepath.Join(mainCwd, filepath.FromSlash(p))
		dst := filepath.Join(worktreePath, filepath.FromSlash(p))
		if _, err := os.Lstat(src); err != nil {
			continue
		}
		if err := copyPathPreserve(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// finalizeForceShellBridgeWorktree verifies main refs + file baseline, then
// applies sandbox changes atomically (all-or-nothing staging).
func finalizeForceShellBridgeWorktree(mainCwd, worktreePath string, base gitHeadCheckpoint) error {
	mainCwd = strings.TrimSpace(mainCwd)
	if mainCwd == "" {
		return fmt.Errorf("%w: empty main cwd", errGitMainRepoMutated)
	}

	// 1) Refs + HEAD integrity (protect non-current branches too).
	if err := verifyMainRefsUnchanged(mainCwd, base); err != nil {
		return err
	}
	// 2) File baseline: concurrent dirty edits on target paths → conflict.
	if err := verifyMainFilesUnchanged(mainCwd, base); err != nil {
		return err
	}
	// 3) Build apply plan from sandbox and apply atomically.
	if worktreePath != "" && worktreePath != mainCwd {
		if err := applySandboxChangesAtomic(mainCwd, worktreePath, base.SHA); err != nil {
			return err
		}
	}
	// 4) Re-check refs after apply (no git ops should have mutated main).
	if err := verifyMainRefsUnchanged(mainCwd, base); err != nil {
		return err
	}
	return nil
}

func verifyMainRefsUnchanged(mainCwd string, base gitHeadCheckpoint) error {
	if base.Unborn {
		if head, err := captureGitHead(mainCwd); err == nil && strings.TrimSpace(head) != "" {
			return fmt.Errorf("%w: unborn repo gained HEAD %s", errGitMainRepoMutated, head)
		}
	} else if base.SHA != "" {
		head, err := captureGitHead(mainCwd)
		if err != nil {
			return fmt.Errorf("%w: cannot read main HEAD: %v", errGitMainRepoMutated, err)
		}
		if strings.TrimSpace(head) != base.SHA {
			return fmt.Errorf("%w: main HEAD moved %s → %s", errGitMainRepoMutated, base.SHA, head)
		}
	}
	now, err := listGitRefs(mainCwd)
	if err != nil {
		// Unborn / no refs is fine when baseline also had none.
		if len(base.Refs) == 0 {
			return nil
		}
		return fmt.Errorf("%w: list refs: %v", errGitMainRepoMutated, err)
	}
	if len(base.Refs) == 0 && len(now) == 0 {
		return nil
	}
	// Any ref added, removed, or tip-changed is a conflict.
	if len(now) != len(base.Refs) {
		return fmt.Errorf("%w: ref set changed (was %d now %d)", errGitMainRepoMutated, len(base.Refs), len(now))
	}
	for ref, sha := range base.Refs {
		if now[ref] != sha {
			return fmt.Errorf("%w: ref %s moved", errGitMainRepoMutated, ref)
		}
	}
	for ref := range now {
		if _, ok := base.Refs[ref]; !ok {
			return fmt.Errorf("%w: new ref %s", errGitMainRepoMutated, ref)
		}
	}
	return nil
}

func verifyMainFilesUnchanged(mainCwd string, base gitHeadCheckpoint) error {
	now, err := snapshotWorkspaceFiles(mainCwd)
	if err != nil {
		return fmt.Errorf("%w: resnapshot main: %v", errGitMainRepoMutated, err)
	}
	// Paths that existed at baseline must match fingerprint (detect concurrent dirty).
	for p, fp := range base.Files {
		cur, ok := now[p]
		if !ok {
			// File deleted on main during turn — conflict if we might write it.
			return fmt.Errorf("%w: main path %q changed (deleted)", errGitMainRepoMutated, p)
		}
		if cur != fp {
			return fmt.Errorf("%w: main path %q changed during turn", errGitMainRepoMutated, p)
		}
	}
	return nil
}

type applyEntry struct {
	Rel    string
	Delete bool
	// For non-delete: staged absolute path with content ready to rename into place.
	Staged string
	// Mode for regular files; for symlinks Mode has ModeSymlink and Target is set.
	Mode   os.FileMode
	Target string // symlink target
	IsLink bool
}

type rollbackItem struct {
	Rel     string
	Had     bool
	IsLink  bool
	Target  string
	Mode    os.FileMode
	Backup  string
	WasGone bool
}

func applySandboxChangesAtomic(mainCwd, sandbox, baseSHA string) error {
	// Collect changed paths in sandbox vs base.
	var paths []string
	if strings.TrimSpace(baseSHA) != "" {
		out, _ := exec.Command("git", "-C", sandbox, "diff", "--name-only", "-z", baseSHA).Output()
		paths = append(paths, splitGitNUL(out)...)
	} else {
		// Unborn private repo: walk sandbox for all files.
		_ = filepath.Walk(sandbox, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				if filepath.Base(path) == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, rerr := filepath.Rel(sandbox, path)
			if rerr != nil {
				return nil
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
	}
	ut, _ := exec.Command("git", "-C", sandbox, "ls-files", "--others", "--exclude-standard", "-z").Output()
	paths = append(paths, splitGitNUL(ut)...)

	// Deleted tracked paths in sandbox.
	if strings.TrimSpace(baseSHA) != "" {
		delOut, _ := exec.Command("git", "-C", sandbox, "diff", "--name-only", "--diff-filter=D", "-z", baseSHA).Output()
		paths = append(paths, splitGitNUL(delOut)...)
	}

	seen := map[string]struct{}{}
	var plan []applyEntry
	stageRoot, err := os.MkdirTemp(filepath.Join(mainCwd, ".flowpilot"), "gemini-apply-*")
	if err != nil {
		// Fallback: stage under temp
		stageRoot, err = os.MkdirTemp("", "gemini-apply-*")
		if err != nil {
			return err
		}
	}
	defer os.RemoveAll(stageRoot)

	for _, p := range paths {
		if p == "" || strings.HasPrefix(p, ".git/") || strings.HasPrefix(p, ".flowpilot/") {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		src := filepath.Join(sandbox, filepath.FromSlash(p))
		st, lerr := os.Lstat(src)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				plan = append(plan, applyEntry{Rel: p, Delete: true})
				continue
			}
			return lerr
		}
		entry := applyEntry{Rel: p, Mode: st.Mode()}
		staged := filepath.Join(stageRoot, filepath.FromSlash(p))
		if st.Mode()&os.ModeSymlink != 0 {
			target, rerr := os.Readlink(src)
			if rerr != nil {
				return rerr
			}
			entry.IsLink = true
			entry.Target = target
			if mkErr := os.MkdirAll(filepath.Dir(staged), 0o755); mkErr != nil {
				return mkErr
			}
			if lerr := os.Symlink(target, staged); lerr != nil {
				return lerr
			}
			entry.Staged = staged
		} else if st.Mode().IsRegular() {
			if mkErr := os.MkdirAll(filepath.Dir(staged), 0o755); mkErr != nil {
				return mkErr
			}
			if cerr := copyFileMode(src, staged, st.Mode()); cerr != nil {
				return cerr
			}
			entry.Staged = staged
		} else {
			// Skip special files.
			continue
		}
		plan = append(plan, entry)
	}

	// Snapshot current destination content for rollback before any main mutation.
	var rollback []rollbackItem
	rbDir, _ := os.MkdirTemp(stageRoot, "rb-*")
	for _, e := range plan {
		dst := filepath.Join(mainCwd, filepath.FromSlash(e.Rel))
		item := rollbackItem{Rel: e.Rel}
		st, err := os.Lstat(dst)
		if err != nil {
			item.WasGone = true
			rollback = append(rollback, item)
			continue
		}
		item.Had = true
		item.Mode = st.Mode()
		bak := filepath.Join(rbDir, filepath.FromSlash(e.Rel))
		_ = os.MkdirAll(filepath.Dir(bak), 0o755)
		if st.Mode()&os.ModeSymlink != 0 {
			t, _ := os.Readlink(dst)
			item.IsLink = true
			item.Target = t
			_ = os.Symlink(t, bak)
		} else if st.Mode().IsRegular() {
			_ = copyFileMode(dst, bak, st.Mode())
		}
		item.Backup = bak
		rollback = append(rollback, item)
	}

	applied := 0
	for _, e := range plan {
		dst := filepath.Join(mainCwd, filepath.FromSlash(e.Rel))
		if e.Delete {
			if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
				_ = rollbackApply(mainCwd, rollback[:applied+1])
				return fmt.Errorf("delete %s: %w", e.Rel, err)
			}
			applied++
			continue
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			_ = rollbackApply(mainCwd, rollback[:applied])
			return mkErr
		}
		// Replace destination: remove first so symlink→file works.
		_ = os.Remove(dst)
		if e.IsLink {
			if err := os.Symlink(e.Target, dst); err != nil {
				_ = rollbackApply(mainCwd, rollback[:applied])
				return fmt.Errorf("symlink %s: %w", e.Rel, err)
			}
		} else {
			// Rename staged file into place (same FS as stageRoot under .flowpilot).
			if err := os.Rename(e.Staged, dst); err != nil {
				// Cross-device fallback.
				if cerr := copyFileMode(e.Staged, dst, e.Mode); cerr != nil {
					_ = rollbackApply(mainCwd, rollback[:applied])
					return fmt.Errorf("write %s: %w", e.Rel, cerr)
				}
			} else {
				_ = os.Chmod(dst, e.Mode.Perm())
			}
		}
		applied++
	}
	return nil
}

func rollbackApply(mainCwd string, items []rollbackItem) error {
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		dst := filepath.Join(mainCwd, filepath.FromSlash(it.Rel))
		_ = os.Remove(dst)
		if it.WasGone || !it.Had {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if it.IsLink {
			_ = os.Symlink(it.Target, dst)
		} else if it.Backup != "" {
			_ = copyFileMode(it.Backup, dst, it.Mode)
		}
	}
	return nil
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
		return mkErr
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode.Perm())
}

func copyPathPreserve(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		target, lerr := os.Readlink(src)
		if lerr != nil {
			return lerr
		}
		_ = os.Remove(dst)
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return mkErr
		}
		return os.Symlink(target, dst)
	}
	if st.Mode().IsRegular() {
		return copyFileMode(src, dst, st.Mode())
	}
	return nil
}

func copyDirPreserve(src, dst string, skip map[string]bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		base := filepath.Base(path)
		if skip[base] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyPathPreserve(path, target)
	})
}

func splitGitNUL(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := strings.Split(string(b), "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// undoForceShellBridgeCommits retained for older tests — verifies main integrity only.
func undoForceShellBridgeCommits(cwd string, base gitHeadCheckpoint) error {
	return finalizeForceShellBridgeWorktree(cwd, "", base)
}
