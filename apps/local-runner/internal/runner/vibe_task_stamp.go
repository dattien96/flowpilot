package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BUG-367: vibe Task/CP docs must not be marked `status: done`. The operator
// tracks completion via DoD / Acceptance Check checkboxes and the TUI
// task x/y chip. `done` is reserved for a later human/ledger move into
// requirements/**/done/.

var (
	vibeDocFrontmatterStatus = regexp.MustCompile(`(?m)^status:[ \t]*"?(draft|done|in_progress|approved)"?[ \t]*$`)
	vibeDocMetadataStatus    = regexp.MustCompile("(?m)^- Status:[ \t]*`?(draft|done|in_progress|approved)`?[ \t]*$")
)

func rewriteVibeDocStatus(src, want string) string {
	want = strings.TrimSpace(want)
	if want == "" || want == "done" {
		want = "in_progress"
	}
	out := vibeDocFrontmatterStatus.ReplaceAllString(src, "status: "+want)
	out = vibeDocMetadataStatus.ReplaceAllString(out, "- Status: `"+want+"`")
	return out
}

func writeIfChanged(abs, prev, next string) {
	if next == prev || strings.TrimSpace(abs) == "" {
		return
	}
	_ = os.WriteFile(abs, []byte(next), 0o644)
}

func resolveVibeWorkspacePath(cwd, rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ""
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return rel
	}
	return filepath.Join(cwd, filepath.FromSlash(rel))
}

// resolveVibeTaskDocPath resolves a plan task path to its on-disk location:
// the todo/ path itself when present, else the same file under done/ — a
// sprint-settled doc (BUG-566). "" when neither exists.
func resolveVibeTaskDocPath(cwd, rel string) string {
	if abs := resolveVibeWorkspacePath(cwd, rel); abs != "" {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	if !strings.Contains(rel, "/todo/") {
		return ""
	}
	abs := resolveVibeWorkspacePath(cwd, strings.Replace(rel, "/todo/", "/done/", 1))
	if abs == "" {
		return ""
	}
	if _, err := os.Stat(abs); err == nil {
		return abs
	}
	return ""
}

// settleVibeTaskDocDone moves the current sprint's Task doc from
// requirements/08-Task/todo/ to requirements/08-Task/done/ so the flow — not
// the operator — owns the todo→done lifecycle transition (BUG-566; live
// run-69320 parked the audit gate on task_referenced until a human git mv'd
// the doc). Pure rename: contents preserved byte-for-byte, status untouched
// (BUG-367 keeps `done` off doc status; the directory carries that signal).
// Returns the settled workspace-relative done/ path — also when the doc was
// already settled — or "" when nothing applies (non-todo path, missing
// source, I/O failure). An existing done/ file is never overwritten.
func settleVibeTaskDocDone(cwd, rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || !strings.Contains(rel, "/todo/") {
		return ""
	}
	doneRel := strings.Replace(rel, "/todo/", "/done/", 1)
	dst := resolveVibeWorkspacePath(cwd, doneRel)
	if dst == "" {
		return ""
	}
	if _, err := os.Stat(dst); err == nil {
		return filepath.ToSlash(doneRel)
	}
	src := resolveVibeWorkspacePath(cwd, rel)
	if src == "" {
		return ""
	}
	if _, err := os.Stat(src); err != nil {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return ""
	}
	if err := os.Rename(src, dst); err != nil {
		return ""
	}
	return filepath.ToSlash(doneRel)
}

// readVibeDocStatus returns draft|in_progress|approved|done from a Task/CP
// frontmatter or Metadata Status line (empty if unread/unknown). Task docs
// resolve through todo/ then done/ so a sprint-settled doc still reports
// its real status (BUG-566).
func readVibeDocStatus(cwd, rel string) string {
	abs := resolveVibeWorkspacePath(cwd, rel)
	if abs == "" {
		return ""
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		abs = resolveVibeTaskDocPath(cwd, rel)
	}
	if abs == "" {
		return ""
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	src := string(b)
	if m := vibeDocFrontmatterStatus.FindStringSubmatch(src); len(m) == 2 {
		return strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := vibeDocMetadataStatus.FindStringSubmatch(src); len(m) == 2 {
		return strings.ToLower(strings.TrimSpace(m[1]))
	}
	return ""
}

// stampVibeTaskInProgress sets a Task file to `in_progress` (never `done`).
func stampVibeTaskInProgress(cwd, taskPath string) {
	abs := resolveVibeWorkspacePath(cwd, taskPath)
	if abs == "" {
		return
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	src := string(b)
	writeIfChanged(abs, src, rewriteVibeDocStatus(src, "in_progress"))
}

// stampVibeTaskDoDChecked ticks DoD / Acceptance Check boxes on the completed
// Task and forces status off `done` onto `in_progress`. The doc is resolved
// through todo/ then done/ — a sprint already settled by the audit (BUG-566)
// still gets its boxes ticked on the moved file.
func stampVibeTaskDoDChecked(cwd, taskPath string) {
	abs := resolveVibeTaskDocPath(cwd, taskPath)
	if abs == "" {
		return
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	src := string(b)
	next := rewriteVibeDocStatus(src, "in_progress")
	next = tickVibeDoDCheckboxes(next)
	writeIfChanged(abs, src, next)
}

// stampVibeCPApproved mirrors stampVibeSSApproved for the locked CP: draft/done
// become `approved`, never `done`.
func stampVibeCPApproved(cwd string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return
	}
	var matches []string
	for _, pat := range []string{
		filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo", "CP-*.md"),
		filepath.Join(cwd, "requirements", "07-Coding-Plan", "inprogress", "CP-*.md"),
	} {
		got, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		matches = append(matches, got...)
	}
	for _, abs := range matches {
		if strings.HasPrefix(strings.ToUpper(filepath.Base(abs)), "FORMAT-") {
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		src := string(b)
		writeIfChanged(abs, src, rewriteVibeDocStatus(src, "approved"))
	}
}

// stampVibeCPDoDChecked ticks CP Definition of Done boxes after the last sprint
// without flipping status to done (stays approved).
func stampVibeCPDoDChecked(cwd string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return
	}
	var matches []string
	for _, pat := range []string{
		filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo", "CP-*.md"),
		filepath.Join(cwd, "requirements", "07-Coding-Plan", "inprogress", "CP-*.md"),
	} {
		got, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		matches = append(matches, got...)
	}
	for _, abs := range matches {
		if strings.HasPrefix(strings.ToUpper(filepath.Base(abs)), "FORMAT-") {
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		src := string(b)
		next := rewriteVibeDocStatus(src, "approved")
		next = tickVibeDoDCheckboxes(next)
		writeIfChanged(abs, src, next)
	}
}

func stampCompletedVibeTask(cwd string, plan []string, startedIndex int) {
	if startedIndex <= 0 || startedIndex-1 >= len(plan) {
		return
	}
	stampVibeTaskDoDChecked(cwd, plan[startedIndex-1])
	if startedIndex >= len(plan) {
		stampVibeCPDoDChecked(cwd)
	}
}

func tickVibeDoDCheckboxes(src string) string {
	lines := strings.Split(src, "\n")
	in := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			h := strings.ToLower(trim)
			in = strings.Contains(h, "definition of done") ||
				strings.Contains(h, "acceptance check") ||
				strings.Contains(h, "dod")
			continue
		}
		if !in {
			continue
		}
		body := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(body, "- [ ]") {
			lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
		}
	}
	return strings.Join(lines, "\n")
}

// vibeTaskProgress reports 1-based current / total for a started sprint.
// startedIndex is vibeSprintIndex (count of takeNext starts). Zero current
// means no sprint has started yet.
func vibeTaskProgress(plan []string, startedIndex int) (current, total int, name string) {
	total = len(plan)
	if total == 0 {
		return 0, 0, ""
	}
	if startedIndex <= 0 {
		return 0, total, ""
	}
	current = startedIndex
	if current > total {
		current = total
	}
	name = shortTaskName(plan[current-1])
	return current, total, name
}

func attachVibeTaskProgressLocked(rs *interactiveRun, st AgentLoopState) AgentLoopState {
	if rs == nil {
		return st
	}
	cur, total, name := vibeTaskProgress(rs.vibeTaskPlan, rs.vibeSprintIndex)
	st.VibeTaskIndex = cur
	st.VibeTaskTotal = total
	st.VibeTaskName = name
	return st
}
