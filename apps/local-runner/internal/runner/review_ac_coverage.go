package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-344 (CP-62 P-2 follow-up): per-AC verdict coverage enforcement on the
// live submit path. Task-338 shipped the pure checks (ExtractACIDs,
// ValidateReviewOutcomeVerdicts); this file resolves the reviewer's governing
// task artifact deterministically (0 LLM tokens) and enforces coverage at the
// one point every provider path funnels through — turnBridge.SubmitFlowControl.
//
// Governing-artifact resolution (in order, first readable file wins):
//  1. rs.vibeTaskName — the runtime value is shortTaskName (file name only),
//     so a bare name is located under <workspace>/requirements (newest match).
//  2. The run's own vibe plan entry (root/inline-hub submitters: the vibe
//     synthesis hub reviews inside the root run, where vibeTaskName is empty
//     but vibeTaskPlan+vibeSprintIndex name the current task doc).
//  3. The submitting node's INPUT file_artifact pathTemplate binding
//     (read_only faces, e.g. plan_reviewer/plan_md) — glob the newest match.
//  4. Sibling OUTPUT pathTemplate bindings (read_only reviewer without its
//     own binding — the harness writer resolves the task doc name at
//     authoring time, so the runner can only match the pattern).
//
// Enforcement matrix:
//   - verdict_only (owner debate): never AC-covered — the owner answers the
//     debate question, not the AC checklist (Q-3 / Task-340).
//   - read_only (reviewer faces): empty verdicts already count as missing.
//   - hub/other (posture ""): passthrough unless verdict rows were submitted
//     — partial rows are rejected, an empty submit keeps legacy behavior.
// An unresolvable artifact yields no expected set: freeform/legacy flows are
// byte-identical (Task-338 fault-tolerant fallback).

func (s *InteractiveService) validateReviewACCoverage(rs *interactiveRun, in FlowControlInput) error {
	if s == nil || rs == nil || !in.viaReviewOutcome {
		return nil
	}
	posture := s.flowNodePostureFor(rs)
	if posture == PostureVerdictOnly {
		return nil
	}
	expected := s.expectedReviewACs(rs, posture)
	if len(expected) == 0 {
		return nil
	}
	rows, _ := in.Payload["verdicts"].([]VerdictRow)
	if posture == "" && len(rows) == 0 {
		return nil
	}
	return ValidateReviewOutcomeVerdicts(ReviewOutcomeInput{Verdicts: rows}, expected)
}

// expectedReviewACs resolves (and caches on the run) the expected AC set for
// the submitter. The state snapshot happens under s.mu; all file I/O happens
// outside it.
func (s *InteractiveService) expectedReviewACs(rs *interactiveRun, posture string) []string {
	if s == nil || rs == nil {
		return nil
	}
	s.mu.Lock()
	if rs.expectedACsResolved {
		cached := rs.expectedACsCache
		s.mu.Unlock()
		return cached
	}
	ws := strings.TrimSpace(rs.workspaceCwd)
	parentID := strings.TrimSpace(rs.parentRunID)
	taskName := strings.TrimSpace(rs.vibeTaskName)
	plan := append([]string(nil), rs.vibeTaskPlan...)
	sprintIndex := rs.vibeSprintIndex
	var node agentpack.FlowNode
	var siblings []agentpack.FlowNode
	if parent := s.runs[parentID]; parent != nil {
		if ws == "" {
			ws = strings.TrimSpace(parent.workspaceCwd)
		}
		siblings = append([]agentpack.FlowNode(nil), parent.activeFlowNodes...)
		stepID, label := strings.TrimSpace(rs.stepID), strings.TrimSpace(rs.label)
		for _, n := range parent.activeFlowNodes {
			if n.ID == stepID || (label != "" && n.ID == label) {
				node = n
				break
			}
		}
	}
	s.mu.Unlock()

	acs := resolveExpectedACs(ws, taskName, plan, sprintIndex, node, siblings, posture)

	s.mu.Lock()
	rs.expectedACsCache, rs.expectedACsResolved = acs, true
	s.mu.Unlock()
	return acs
}

// resolveExpectedACs walks the source list; the first readable artifact with
// at least one AC-N token wins (an artifact without ACs falls through so a
// wrong-path hit cannot silently zero the coverage check).
func resolveExpectedACs(ws, taskName string, plan []string, sprintIndex int, node agentpack.FlowNode, siblings []agentpack.FlowNode, posture string) []string {
	if strings.TrimSpace(ws) == "" {
		return nil
	}
	if taskName != "" {
		if p := locateTaskDoc(ws, taskName); p != "" {
			if acs := readACs(p); len(acs) > 0 {
				return acs
			}
		}
	}
	if sprintIndex >= 1 && sprintIndex <= len(plan) {
		if p := joinWorkspacePath(ws, plan[sprintIndex-1]); p != "" {
			if acs := readACs(p); len(acs) > 0 {
				return acs
			}
		}
	}
	if posture == PostureReadOnly {
		for _, tmpl := range nodeInputPathTemplates(node) {
			if p := newestTemplateMatch(ws, tmpl); p != "" {
				if acs := readACs(p); len(acs) > 0 {
					return acs
				}
			}
		}
		for _, sib := range siblings {
			for _, tmpl := range nodeOutputPathTemplates(sib) {
				if p := newestTemplateMatch(ws, tmpl); p != "" {
					if acs := readACs(p); len(acs) > 0 {
						return acs
					}
				}
			}
		}
	}
	return nil
}

// locateTaskDoc resolves a task doc for the vibe runtime short name: a name
// with a separator is workspace-relative; a bare file name is located under
// <workspace>/requirements (newest mtime wins — the same instruction the
// runner gives agents for templated inputs).
func locateTaskDoc(ws, name string) string {
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator) {
		return joinWorkspacePath(ws, name)
	}
	root := filepath.Join(ws, "requirements")
	if _, err := os.Stat(root); err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // best-effort: unreadable entries never fail the run
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != name {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = path, info.ModTime()
		}
		return nil
	})
	return best
}

// newestTemplateMatch globs the newest file whose base name matches the
// CP-58 Task-307 pathTemplate (`{{idx}}` → digits, `{{slug}}` → kebab slug).
func newestTemplateMatch(ws, tmpl string) string {
	re, err := templateMatchRegex(tmpl)
	if err != nil {
		return ""
	}
	absDir := filepath.Join(ws, filepath.Dir(filepath.FromSlash(tmpl)))
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !re.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = filepath.Join(absDir, e.Name()), info.ModTime()
		}
	}
	return best
}

func templateMatchRegex(tmpl string) (*regexp.Regexp, error) {
	// The pattern matches the BASE file name (ReadDir entries), not the full
	// template path — the directory part is consumed by absDir above.
	pattern := regexp.QuoteMeta(filepath.Base(filepath.FromSlash(tmpl)))
	pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("{{idx}}"), `\d+`)
	pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("{{slug}}"), `[a-z0-9-]+`)
	return regexp.Compile("^" + pattern + "$")
}

func nodeInputPathTemplates(node agentpack.FlowNode) []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range node.ArtifactBindings {
		if b.Direction != "input" || b.ArtifactTypeID != ArtifactTypeFile {
			continue
		}
		raw, _ := b.ConfigJSON["pathTemplate"].(string)
		tmpl := strings.TrimSpace(raw)
		if tmpl == "" || seen[tmpl] {
			continue
		}
		seen[tmpl] = true
		out = append(out, tmpl)
	}
	return out
}

// nodeOutputPathTemplates collects required templated OUTPUT bindings —
// mirrors templatedFileArtifactOutputsForNode's gating (concrete-paths
// bindings stay on the exact-path contract and are not globbed).
func nodeOutputPathTemplates(node agentpack.FlowNode) []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range node.ArtifactBindings {
		if b.Direction != "output" || !b.Required || b.ArtifactTypeID != ArtifactTypeFile {
			continue
		}
		raw, _ := b.ConfigJSON["pathTemplate"].(string)
		tmpl := strings.TrimSpace(raw)
		if tmpl == "" || seen[tmpl] {
			continue
		}
		seen[tmpl] = true
		out = append(out, tmpl)
	}
	return out
}

// joinWorkspacePath returns the path only when it exists as a file, so a
// stale plan entry falls through to the next source.
func joinWorkspacePath(ws, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	full := filepath.Join(ws, filepath.FromSlash(p))
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		return ""
	}
	return full
}

func readACs(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return ExtractACIDs(string(data))
}
