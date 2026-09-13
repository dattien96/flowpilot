package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// ContextSourceConventions — CP-62 P-7 (Task-343, repo-as-config): project
// conventions load from static files instead of per-flow prompt forks.
// Precedence: user-level ~/.flowpilot/conventions.md, then workspace
// .flowpilot/conventions.md (falling back to workspace AGENTS.md). The body
// renders under a "## Context" heading so the Task-334 Budget Packer
// classifies it mandatory_doc (CP-23 R-1: retained whole, never pruned) —
// Tier-1 through the existing machinery, no packer change.
const ContextSourceConventions ContextSourceID = "conventions"

type conventionsSource struct{ priority int }

func (s *conventionsSource) ID() string          { return string(ContextSourceConventions) }
func (s *conventionsSource) Priority() int       { return s.priority }
func (s *conventionsSource) Deterministic() bool { return true }

// conventionsBodyFor reads the conventions layers for a workspace. Exported
// for tests: user-level file first, workspace conventions file second
// (workspace AGENTS.md is the fallback when the workspace conventions file
// is absent). Missing files are skipped silently — an all-missing setup is
// an empty section, never an error (flow continues unchanged).
func conventionsBodyFor(workspace string) (body string, sourceRefs []string) {
	var parts []string
	appendFile := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return
		}
		parts = append(parts, content)
		sourceRefs = append(sourceRefs, filepath.ToSlash(path))
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		appendFile(filepath.Join(home, ".flowpilot", "conventions.md"))
	}
	if strings.TrimSpace(workspace) != "" {
		wsConventions := filepath.Join(workspace, ".flowpilot", "conventions.md")
		if _, err := os.Stat(wsConventions); err == nil {
			appendFile(wsConventions)
		} else {
			// Workspace fallback: AGENTS.md (the repo-as-config standard).
			appendFile(filepath.Join(workspace, "AGENTS.md"))
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n---\n\n"), sourceRefs
}

func (s *conventionsSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if strings.TrimSpace(hints.Workspace) == "" {
		return section, nil
	}
	body, refs := conventionsBodyFor(hints.Workspace)
	if body == "" {
		return section, nil
	}
	section.SourceRef = strings.Join(refs, ", ")
	section.Body = "## Context — Project Conventions (repo-as-config, never pruned)\n\n" + body
	return section, nil
}
