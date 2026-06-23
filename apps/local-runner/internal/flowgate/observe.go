package flowgate

import (
	"bufio"
	"os/exec"
	"strings"
)

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
