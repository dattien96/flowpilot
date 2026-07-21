package flowgate

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// gitEnv forces English git messages (BUG-288 R13-11) so isNotAGitRepoErr and
// similar classifiers do not break when the user's LANG is non-English.
func gitEnv() []string {
	return append(os.Environ(), "LC_ALL=C", "LANG=C")
}

// ObserveGitDiffSince returns changed files between baseSHA and HEAD (committed
// changes) combined with any uncommitted working-tree changes. This is the
// correct view to use after an AI turn: the AI may have committed files, making
// git status --porcelain return nothing even though real changes occurred.
// If baseSHA is empty the function falls back to ObserveGitDiff (uncommitted only).
//
// V9-15: uses git -z / -z --name-status so paths with spaces/tabs/newlines/quotes
// are not split by Fields.
func ObserveGitDiffSince(repoDir, baseSHA string) ([]ChangedFile, error) {
	// V10R4 P1: never swallow observation errors as empty/clean diffs — audit
	// and tier-1 gates must escalate when Git cannot verify work.
	uncommitted, err := ObserveGitDiff(repoDir)
	if err != nil {
		return nil, err
	}

	if baseSHA == "" {
		return uncommitted, nil
	}

	// git diff -z --name-status <baseSHA>..HEAD
	cmd := exec.Command("git", "-C", repoDir, "diff", "-z", "--name-status", baseSHA+"..HEAD")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		// Distinguish empty range (ok) from real failure. Exit 0/1 with empty
		// output can be clean; non-nil err with no usable output is failure.
		// git diff returns exit 0 always for valid range; exit 128 for bad SHA/repo.
		return nil, err
	}

	byPath := make(map[string]ChangedFile)
	for _, f := range uncommitted {
		byPath[f.Path] = f
	}
	for _, f := range parseNameStatusZ(out) {
		if _, alreadySeen := byPath[f.Path]; !alreadySeen {
			byPath[f.Path] = f
		}
	}

	files := make([]ChangedFile, 0, len(byPath))
	for _, f := range byPath {
		files = append(files, f)
	}
	return files, nil
}

func ObserveGitDiff(repoDir string) ([]ChangedFile, error) {
	// -z + -uall: NUL-terminated records; expand untracked dirs to files (V9-15).
	cmd := exec.Command("git", "-C", repoDir, "status", "--porcelain", "-z", "-uall")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parsePorcelainZ(out), nil
}

// parsePorcelainZ parses `git status --porcelain -z` output.
// Format: XY <path>\0  or, for rename/copy, XY <path>\0<orig_path>\0.
// BUG-288 P2-05 (Vòng 12): git-status(1) documents that for -z rename/copy the
// first NUL-delimited path field is the CURRENT/destination path and the second
// is the ORIGINAL/source path (NOT the same order as the human-readable
// "ORIG_PATH -> PATH" form). A previous parser assumed the non-z order and
// overwrote destination with source. V10: do NOT TrimSpace — leading/trailing
// whitespace in filenames is legal.
func parsePorcelainZ(out []byte) []ChangedFile {
	var files []ChangedFile
	parts := bytes.Split(out, []byte{0})
	for i := 0; i < len(parts); i++ {
		rec := parts[i]
		if len(rec) < 3 {
			continue
		}
		indexStatus := string(rec[0])
		workStatus := string(rec[1])
		// After "XY " (3 bytes) comes the destination/current path.
		path := string(rec[3:])
		if path == "" {
			continue
		}
		// Rename/copy: consume the second NUL field (the original/source
		// path) so it is not mistaken for the next record's status line —
		// but the destination path above is already the correct "the path"
		// for policy purposes; do not overwrite it.
		if (indexStatus == "R" || indexStatus == "C" || workStatus == "R" || workStatus == "C") && i+1 < len(parts) {
			i++
		}
		if path == "" {
			continue
		}
		if indexStatus == "?" && workStatus == "?" {
			files = append(files, ChangedFile{Path: path, Status: "A"})
			continue
		}
		status := resolveStatus(indexStatus, workStatus)
		if status == "" {
			continue
		}
		files = append(files, ChangedFile{Path: path, Status: status})
	}
	return files
}

// parseNameStatusZ parses `git diff -z --name-status` output.
// Format: STATUS\0PATH\0 or for renames Rxxx\0OLD\0NEW\0 (Path = NEW).
// V10: preserve path bytes; no TrimSpace.
func parseNameStatusZ(out []byte) []ChangedFile {
	var files []ChangedFile
	parts := bytes.Split(out, []byte{0})
	for i := 0; i < len(parts); {
		st := string(parts[i])
		if st == "" {
			i++
			continue
		}
		// Status token may be "R100" etc.; first rune is the class.
		statusChar := string(st[0])
		i++
		if i >= len(parts) {
			break
		}
		path := string(parts[i])
		i++
		// Rename/copy: next field is destination; use that as Path.
		if (statusChar == "R" || statusChar == "C") && i < len(parts) {
			dest := string(parts[i])
			i++
			if dest != "" {
				path = dest
			}
		}
		if path == "" {
			continue
		}
		status := resolveStatus(statusChar, " ")
		if status == "" {
			continue
		}
		files = append(files, ChangedFile{Path: path, Status: status})
	}
	return files
}

func resolveStatus(index, work string) string {
	combined := index + work
	// BUG-288 #15: staged/committed renames (R) and copies (C) are code moves —
	// treat as Modified so Tier-1/audit do not see an empty diff.
	if strings.ContainsAny(combined, "RC") {
		return "M"
	}
	if strings.ContainsAny(combined, "AD") {
		if index == "A" || work == "A" {
			return "A"
		}
		return "D"
	}
	if strings.ContainsAny(combined, "M") {
		return "M"
	}
	return ""
}

func HasChangeAuditNote(diff []ChangedFile) bool {
	for _, f := range diff {
		if strings.Contains(f.Path, "change-audit/CA-") && (f.Status == "A" || f.Status == "M") {
			return true
		}
	}
	return false
}

// HasChangeAuditNoteInPaths reports a change-audit CA note among tool-written
// or held path strings (no git status). Used when a gate re-check carries
// pendingGateCodePaths as WrittenPaths with an empty GitDiff (BUG-288 #8 /
// run-23820 remediation turns): the CA from the prior coding turn must still
// satisfy r-ca.
func HasChangeAuditNoteInPaths(paths []string) bool {
	for _, p := range paths {
		if strings.Contains(p, "change-audit/CA-") {
			return true
		}
	}
	return false
}

func HasTaskDoc(diff []ChangedFile) bool {
	for _, f := range diff {
		if strings.Contains(f.Path, "FORMAT-REFERENCE-") {
			continue
		}
		if strings.Contains(f.Path, "requirements/08-Task") || strings.Contains(f.Path, "Task-") {
			return true
		}
	}
	return false
}

func HasBugFixDoc(diff []ChangedFile) bool {
	for _, f := range diff {
		// Exclude FORMAT-REFERENCE-*.md files — they are scaffold templates, not real
		// BugFix documents. The reqscaffold creates these in the target project on bind;
		// without this guard, the untracked template causes HasBugFixDoc to return true,
		// silently suppressing the r-bug violation. (BUG-141)
		if strings.Contains(f.Path, "FORMAT-REFERENCE-") {
			continue
		}
		if strings.Contains(f.Path, "requirements/09-BugFix") || strings.Contains(f.Path, "BUG-") {
			return true
		}
	}
	return false
}

// HasBugFixDocInPaths mirrors HasBugFixDoc for path lists (pending re-check /
// WrittenPaths) without a git status. FORMAT-REFERENCE templates are excluded.
func HasBugFixDocInPaths(paths []string) bool {
	for _, p := range paths {
		if strings.Contains(p, "FORMAT-REFERENCE-") {
			continue
		}
		if strings.Contains(p, "requirements/09-BugFix") || strings.Contains(p, "BUG-") {
			return true
		}
	}
	return false
}

func IsDocOrAuditFile(path string) bool {
	// BUG-288 #16 / F-25: .flowpilot/** is runtime metadata, not product code.
	return strings.HasPrefix(path, "requirements/") ||
		strings.HasPrefix(path, "change-audit/") ||
		strings.HasPrefix(path, ".flowpilot/") ||
		strings.HasSuffix(path, ".md")
}

func HasCodeChanges(diff []ChangedFile) bool {
	for _, f := range diff {
		if !IsDocOrAuditFile(f.Path) {
			return true
		}
	}
	return false
}

// HasCodeChangesInList checks if any path in the list is a source code file
// (not a doc/audit/requirements file). Takes a plain []string instead of []ChangedFile
// so it can be called with TurnResult.WrittenPaths (files the AI tool-called directly),
// which is the correct signal for whether the AI changed code in this turn.
func HasCodeChangesInList(paths []string) bool {
	for _, p := range paths {
		if !IsDocOrAuditFile(p) {
			return true
		}
	}
	return false
}
