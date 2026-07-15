package runner

import (
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"flowpilot-runner/internal/flowgate"
)

// extractPromptSourcePaths pulls path-like tokens from a user/plan prompt
// (Task-246). Pure string parse — no filesystem I/O.
func extractPromptSourcePaths(prompt string) []string {
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, tok := range strings.Fields(prompt) {
		tok = strings.TrimFunc(tok, func(r rune) bool {
			return r == '`' || r == '\'' || r == '"' || r == '(' || r == ')' || r == ',' ||
				r == '[' || r == ']' || r == '{' || r == '}' || unicode.IsSpace(r)
		})
		if tok == "" {
			continue
		}
		low := strings.ToLower(tok)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			continue
		}
		if !strings.ContainsAny(tok, `/\`) {
			continue
		}
		if filepath.Ext(tok) == "" {
			continue
		}
		norm := filepath.ToSlash(tok)
		if flowgate.IsDocOrAuditFile(norm) {
			continue
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, norm)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// uncommittedChangedPaths lists staged+unstaged+untracked paths vs HEAD
// (Task-246). Non-git workspace or any error → nil (soft degrade).
func uncommittedChangedPaths(workspace string) []string {
	if strings.TrimSpace(workspace) == "" {
		return nil
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	diffOut, err1 := exec.CommandContext(ctx, "git", "-C", workspace, "diff", "--name-only", "HEAD").Output()
	untrackedOut, err2 := exec.CommandContext(ctx, "git", "-C", workspace, "ls-files", "--others", "--exclude-standard").Output()
	if err1 != nil && err2 != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	add := func(raw []byte) {
		for _, line := range strings.Split(string(raw), "\n") {
			p := filepath.ToSlash(strings.TrimSpace(line))
			if p == "" || seen[p] || flowgate.IsDocOrAuditFile(p) {
				continue
			}
			seen[p] = true
			paths = append(paths, p)
			if len(paths) >= 20 {
				return
			}
		}
	}
	if err1 == nil {
		add(diffOut)
	}
	if err2 == nil && len(paths) < 20 {
		add(untrackedOut)
	}
	if time.Since(start) > time.Second {
		log.Printf("[source.excerpt] uncommittedChangedPaths workspace=%q took %s paths=%d", workspace, time.Since(start), len(paths))
	}
	return paths
}
