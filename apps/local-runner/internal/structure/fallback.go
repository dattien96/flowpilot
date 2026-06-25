package structure

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"os/exec"
)

type fallbackProvider struct {
	repoDir string
}

func (f *fallbackProvider) Available() bool {
	return false
}

// Dependents performs a file-level fallback when GitNexus is unavailable.
// For .go files or directories it combines import scanning with git co-change history.
// Always returns Complete=false because this is a structural approximation only.
func (f *fallbackProvider) Dependents(ctx context.Context, target string) (DependentsSummary, error) {
	nearest := f.gatherNearestFiles(ctx, target)
	if len(nearest) > 10 {
		nearest = nearest[:10]
	}
	return DependentsSummary{
		Count:    len(nearest),
		Nearest:  nearest,
		Complete: false,
	}, nil
}

// gatherNearestFiles merges import-based dependents with git co-change history.
func (f *fallbackProvider) gatherNearestFiles(ctx context.Context, target string) []string {
	seen := make(map[string]bool)
	var result []string

	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	for _, p := range f.importDependents(target) {
		add(p)
	}
	for _, p := range f.gitCoChanged(ctx, target) {
		add(p)
	}
	return result
}

// importDependents finds .go files that import the package path derived from target.
// target may be a .go file path or a directory.
func (f *fallbackProvider) importDependents(target string) []string {
	pkgPath := packagePathOf(target)
	if pkgPath == "" {
		return nil
	}

	var matches []string
	_ = filepath.WalkDir(f.repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if bytes.Contains(data, []byte(pkgPath)) {
			rel, relErr := filepath.Rel(f.repoDir, path)
			if relErr == nil {
				matches = append(matches, rel)
			}
		}
		return nil
	})
	return matches
}

// gitCoChanged uses "git log --name-only" to find files that were committed
// alongside target, acting as a rough structural-proximity signal.
func (f *fallbackProvider) gitCoChanged(ctx context.Context, target string) []string {
	cmd := exec.CommandContext(ctx,
		"git", "-C", f.repoDir,
		"log", "--all", "--name-only", "--format=", "--follow", "--", target,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &bytes.Buffer{}

	if err := cmd.Run(); err != nil {
		return nil
	}

	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == target || seen[line] {
			continue
		}
		seen[line] = true
		files = append(files, line)
	}
	return files
}

// packagePathOf converts a file or directory path to a Go import path fragment
// by stripping the ".go" extension and returning the directory portion.
func packagePathOf(target string) string {
	target = filepath.ToSlash(target)
	if strings.HasSuffix(target, ".go") {
		target = target[:len(target)-3]
		idx := strings.LastIndex(target, "/")
		if idx >= 0 {
			return target[:idx]
		}
		return target
	}
	return strings.TrimSuffix(target, "/")
}
