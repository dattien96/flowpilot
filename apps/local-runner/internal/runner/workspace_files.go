package runner

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const workspaceFileListLimit = 40

func listWorkspaceFilePaths(cwd, query string) []string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", cwd, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, raw := range bytes.Split(out, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		paths = append(paths, filepath.ToSlash(string(raw)))
	}
	return filterWorkspaceFilePaths(paths, query, workspaceFileListLimit)
}

func filterWorkspaceFilePaths(paths []string, query string, limit int) []string {
	if limit <= 0 {
		limit = workspaceFileListLimit
	}
	q := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(query, `\`, "/")))
	out := make([]string, 0, limit)
	for _, p := range paths {
		norm := strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
		if q != "" && !strings.Contains(norm, q) {
			continue
		}
		out = append(out, p)
		if len(out) >= limit {
			break
		}
	}
	return out
}
