package changeledger

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// changedPathsLogFormat puts the record separator *before* the hash so that
// splitting the output on 0x1e yields exactly one chunk per commit: the hash on
// the first line, then that commit's files one per line (git prints the pretty
// header, a blank line, then the --name-only list).
const changedPathsLogFormat = "--pretty=format:\x1e%H"

// changedPathsByCommit runs ONE `git log --name-only` over the same commit range
// ParseRepo uses and maps commit hash -> repo-relative, forward-slash, sorted
// paths (CP-54 P-1 / Task-261 T-1).
//
// Deliberately a separate log pass rather than a hook into enrich.go: that file's
// pathFeatureKey runs `git show` per commit and only for the Priority-3 entries
// that reach it at all, so filling ChangedPaths there would cost one subprocess
// per commit. This costs one subprocess for the whole range.
//
// Non-fatal (Task-261 T-5): a missing git, an empty repo, or a cursor ahead of
// HEAD yields an empty map, never an error.
func changedPathsByCommit(repoDir, cursor string) map[string][]string {
	args := []string{"-C", repoDir, "log", "--no-merges", "--name-only", changedPathsLogFormat}
	if cursor != "" {
		args = append(args, cursor+"..HEAD")
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil || len(out) == 0 {
		return map[string][]string{}
	}

	byCommit := make(map[string][]string)
	for _, chunk := range strings.Split(string(out), "\x1e") {
		if hash, paths := parseChangedPathsChunk(chunk); hash != "" {
			byCommit[hash] = paths
		}
	}
	return byCommit
}

// parseChangedPathsChunk reads one 0x1e-delimited chunk: the commit hash on the
// first non-empty line, then the files it touched. Paths are sorted so the
// result never depends on the order git happened to emit them (BUG-266's
// determinism rule).
func parseChangedPathsChunk(chunk string) (string, []string) {
	var hash string
	var paths []string

	for _, line := range strings.Split(chunk, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if hash == "" {
			hash = line
			continue
		}
		paths = append(paths, filepath.ToSlash(line))
	}

	sort.Strings(paths)
	return hash, paths
}

// attachChangedPaths fills ChangedPaths on entries whose commit appears in
// byCommit, leaving the rest untouched (nil = "no locus signal", readers fall
// back to recency).
func attachChangedPaths(entries []Entry, byCommit map[string][]string) {
	if len(byCommit) == 0 {
		return
	}
	for i := range entries {
		if paths, ok := byCommit[entries[i].CommitHash]; ok && len(paths) > 0 {
			entries[i].ChangedPaths = paths
		}
	}
}

// backfillChangedPaths fills ChangedPaths on entries written before CP-54 P-1
// (Task-261 T-D). Without it the field would only ever cover commits made after
// this landed, leaving the existing history — the whole point of the retrieval
// layer — unscoreable.
//
// Cheap when there is nothing to do: it inspects the in-memory entries first and
// only shells out when at least one is missing the field. Non-fatal — a failure
// leaves entries as they are.
//
// Residual case: an entry whose commit is no longer reachable from HEAD (rebased
// away) or that touched no files can never be filled, so the git pass repeats on
// each Build. That is one bounded `git log` per engine init, not per commit.
func (l *Ledger) backfillChangedPaths(repoDir string) {
	if !l.hasEntryMissingChangedPaths() {
		return
	}

	byCommit := changedPathsByCommit(repoDir, "")
	if len(byCommit) == 0 {
		return
	}

	l.mu.Lock()
	filled := false
	for hash, e := range l.entries {
		if len(e.ChangedPaths) > 0 {
			continue
		}
		if paths, ok := byCommit[hash]; ok && len(paths) > 0 {
			e.ChangedPaths = paths
			l.entries[hash] = e
			filled = true
		}
	}
	l.mu.Unlock()

	if filled {
		// Compact takes the mutex itself, so it must be called unlocked.
		_ = l.Compact()
	}
}

// hasEntryMissingChangedPaths reports whether any in-memory entry still lacks
// the field, so the caller can skip the git pass entirely.
func (l *Ledger) hasEntryMissingChangedPaths() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if len(e.ChangedPaths) == 0 {
			return true
		}
	}
	return false
}
