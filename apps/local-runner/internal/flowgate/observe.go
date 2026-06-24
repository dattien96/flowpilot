package flowgate

import (
	"bufio"
	"os/exec"
	"strings"
)

// ObserveGitDiffSince returns changed files between baseSHA and HEAD (committed
// changes) combined with any uncommitted working-tree changes. This is the
// correct view to use after an AI turn: the AI may have committed files, making
// git status --porcelain return nothing even though real changes occurred.
// If baseSHA is empty the function falls back to ObserveGitDiff (uncommitted only).
func ObserveGitDiffSince(repoDir, baseSHA string) ([]ChangedFile, error) {
	uncommitted, _ := ObserveGitDiff(repoDir)

	if baseSHA == "" {
		return uncommitted, nil
	}

	// git diff <baseSHA>..HEAD --name-status lists committed changes since baseSHA.
	cmd := exec.Command("git", "-C", repoDir, "diff", baseSHA+"..HEAD", "--name-status")
	out, err := cmd.Output()
	if err != nil {
		return uncommitted, nil
	}

	byPath := make(map[string]ChangedFile)
	// Seed with uncommitted changes first.
	for _, f := range uncommitted {
		byPath[f.Path] = f
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 2 {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		statusChar := string(parts[0][0])
		path := parts[len(parts)-1]
		status := resolveStatus(statusChar, " ")
		if status == "" {
			continue
		}
		// Uncommitted version of the same file takes precedence.
		if _, alreadySeen := byPath[path]; !alreadySeen {
			byPath[path] = ChangedFile{Path: path, Status: status}
		}
	}

	files := make([]ChangedFile, 0, len(byPath))
	for _, f := range byPath {
		files = append(files, f)
	}
	return files, nil
}

func ObserveGitDiff(repoDir string) ([]ChangedFile, error) {
	cmd := exec.Command("git", "-C", repoDir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	var files []ChangedFile
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		indexStatus := string(line[0])
		workStatus := string(line[1])
		path := strings.TrimSpace(line[3:])

		status := resolveStatus(indexStatus, workStatus)
		if status == "" {
			continue
		}
		files = append(files, ChangedFile{Path: path, Status: status})
	}
	return files, nil
}

func resolveStatus(index, work string) string {
	combined := index + work
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

func HasBugFixDoc(diff []ChangedFile) bool {
	for _, f := range diff {
		if strings.Contains(f.Path, "requirements/09-BugFix") || strings.Contains(f.Path, "BUG-") {
			return true
		}
	}
	return false
}

func IsDocOrAuditFile(path string) bool {
	return strings.HasPrefix(path, "requirements/") ||
		strings.HasPrefix(path, "change-audit/") ||
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
