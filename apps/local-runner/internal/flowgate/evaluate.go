package flowgate

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var taskIDRegex = regexp.MustCompile(`\bTask-\d+\b`)

func Evaluate(tr TurnResult, rules []Rule) []Violation {
	var violations []Violation
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		v := checkRule(rule, tr)
		if v != nil {
			violations = append(violations, *v)
		}
	}
	return violations
}

func checkRule(rule Rule, tr TurnResult) *Violation {
	switch rule.Trigger {
	case "code_changed":
		// Use WrittenPaths (files the AI tool-called) not GitDiff: the git working tree may
		// contain pre-existing dirty files or runner-internal state (e.g. test_baseline.json)
		// that are not changes the AI made and must not trigger a change-audit requirement.
		// run-23820: pendingGateCodePaths re-check may set WrittenPaths with an empty
		// GitDiff — still honor a CA path held from the prior coding turn.
		if HasCodeChangesInList(tr.WrittenPaths) &&
			!HasChangeAuditNote(tr.GitDiff) &&
			!HasChangeAuditNoteInPaths(tr.WrittenPaths) {
			return &Violation{Rule: rule, Detail: "code changed but no change-audit note found"}
		}

	case "commit_feature_key_missing":
		if len(tr.CommitSubjects) == 0 {
			return nil
		}
		if !HasCodeChanges(tr.GitDiff) && !HasCodeChangesInList(tr.WrittenPaths) {
			return nil
		}
		if missingFeatureKey(tr.CommitSubjects, tr.KnownFeatureKeys) {
			return &Violation{
				Rule:        rule,
				Detail:      "code-changing turn committed without a verified feature key",
				Options:     append([]string(nil), tr.SuggestedFeatureKeys...),
				SourceDocID: tr.SourceDocID,
			}
		}

	case "bug_fixed":
		isBugFix := tr.ChangeType == "bugfix"
		if !isBugFix {
			msgLower := strings.ToLower(tr.FinalMessage)
			isBugFix = strings.Contains(msgLower, "fixed bug") ||
				strings.Contains(msgLower, "bug fix")
		}
		if isBugFix && !HasBugFixDoc(tr.GitDiff) && !HasBugFixDocInPaths(tr.WrittenPaths) {
			if tr.ChangeType == "bugfix" {
				return &Violation{
					Rule:        rule,
					Detail:      "declared bug mode but no bugfix document found",
					SourceDocID: tr.SourceDocID,
					Declared:    true,
				}
			}
			return &Violation{Rule: rule, Detail: "bug fix detected but no bugfix doc found"}
		}

	case "task_referenced":
		// Explicitly declared Task mode wins first. If the run already carries
		// ChangeType == "task", we do not consult the final-message regex at all.
		// Otherwise we fall back to the v1 Task-ID heuristic. (SD-20 §2.7, Task-113)
		hasTaskRef := tr.ChangeType == "task"
		if !hasTaskRef {
			hasTaskRef = taskIDRegex.MatchString(tr.FinalMessage)
		}
		if hasTaskRef && !HasTaskDoc(tr.GitDiff) {
			if tr.ChangeType == "task" {
				return &Violation{
					Rule:        rule,
					Detail:      "declared task mode but no task document found",
					SourceDocID: tr.SourceDocID,
					Declared:    true,
				}
			}
			return &Violation{Rule: rule, Detail: "task reference detected but no task document found"}
		}

	case "tests_failed":
		// Ordinary suite failures only (V9-27) — not regression names.
		if tr.Tests.Ran && len(tr.Tests.Failed) > 0 {
			return &Violation{Rule: rule, Detail: "Tests failed: " + strings.Join(tr.Tests.Failed, ", ")}
		}

	case "regression_test_broke":
		// Prefer dedicated Regressed list; fall back to Failed for legacy callers
		// that only populate Failed with regression names.
		names := tr.Tests.Regressed
		if len(names) == 0 {
			names = tr.Tests.Failed
		}
		if tr.Tests.Ran && len(names) > 0 {
			return &Violation{
				Rule:           rule,
				Detail:         "Tests failed: " + strings.Join(names, ", "),
				Options:        []string{"keep-test-fix-code", "suggest-requirement-change", "custom"},
				RegressedTests: names,
			}
		}

	case "removed_referenced_code":
		for _, f := range tr.GitDiff {
			if f.Status == "D" && strings.HasSuffix(f.Path, ".go") {
				return &Violation{Rule: rule, Detail: "Removed: " + f.Path}
			}
		}

	case "required_artifact_output_missing":
		// Task-223: enforce file_artifact OUTPUT write contract via filesystem
		// existence (WrittenPaths alone is incomplete across providers).
		missing := MissingRequiredFileArtifactOutputs(tr.WorkspaceCwd, tr.RequiredFileArtifactOutputs)
		if len(missing) > 0 {
			return &Violation{
				Rule:   rule,
				Detail: "required file artifact output missing: " + strings.Join(missing, ", "),
			}
		}

	case "required_artifact_output_structure_missing":
		// Task-225: after existence, require declared structure.sections headings.
		// Paths still missing on disk are owned by r-artifact-output only.
		gaps := MissingRequiredFileArtifactStructures(tr.WorkspaceCwd, tr.RequiredStructuredFileArtifactOutputs)
		if len(gaps) > 0 {
			return &Violation{
				Rule:   rule,
				Detail: "required file artifact structure missing: " + strings.Join(gaps, "; "),
			}
		}

	case "required_telegram_send_missing":
		// Task-233: verify a real message_id, not just that a tool was called.
		missing := MissingTelegramSends(tr.FinalMessage, tr.RequiredTelegramSends)
		if len(missing) > 0 {
			return &Violation{
				Rule:   rule,
				Detail: "required Telegram notification not confirmed sent (no message_id evidence) for: " + strings.Join(missing, ", "),
			}
		}

	case "code_changed_no_contract":
		// Task-185 (CP-43 P-2): a code-changing turn used an inferred contract
		// rather than an AI-declared one. Same WrittenPaths reasoning as
		// "code_changed" above — a dirty working tree the AI didn't touch must
		// not itself trigger this.
		if HasCodeChangesInList(tr.WrittenPaths) && !tr.ContractDeclared {
			return &Violation{Rule: rule, Detail: "code changed without a declared Change Contract (used an inferred one)"}
		}

	case "edit_outside_declared_scope":
		// Task-185 (CP-43 P-2): actual_touched \ declared_scope is non-empty.
		// Never resolve to "block" on file-level truth alone (SD-21 D-5/Q-2) —
		// downgrade the configured action to "warn" unless the caller marked
		// this high-severity (structure available AND out-of-scope dependents).
		if len(tr.ScopeOutOfScopePaths) > 0 {
			effective := rule
			if !tr.ScopeHighSeverity && effective.Action == "block" {
				effective.Action = "warn"
			}
			return &Violation{Rule: effective, Detail: "edit outside declared scope: " + strings.Join(tr.ScopeOutOfScopePaths, ", ")}
		}

	case "governing_spec_changed":
		// Task-186 (CP-43 P-3): the feature's governing doc(s) changed since
		// the Canonical Head last recorded them.
		if tr.HeadSpecDrifted {
			return &Violation{Rule: rule, Detail: "governing spec changed since the Canonical Head was last computed — reconcile the Head"}
		}

	case "code_diverged_from_intent":
		// Task-186 (CP-43 P-3): out-of-contract code change, spec unchanged.
		if tr.HeadCodeDrifted {
			return &Violation{Rule: rule, Detail: "code changed out of contract with no corresponding spec/behavior update"}
		}

	case "governing_spec_added_to_spec_less":
		// Task-186 (CP-43 P-3): a spec_less feature just gained a governing
		// doc — human must confirm the new spec matches current behavior
		// before the Head is rebaselined (never automatic, BR-2).
		if tr.HeadAttachSpecPending {
			return &Violation{Rule: rule, Detail: "a governing spec was added to a previously spec-less feature — confirm it matches current behavior to rebaseline the Canonical Head"}
		}

	case "feature_rename_merge_or_deprecate":
		// Task-187 (CP-43 P-4): a retire intent was detected for the active
		// feature — human must confirm the target(s) before RetireHead runs
		// (never automatic, BR-2).
		if tr.HeadRetirePending {
			return &Violation{Rule: rule, Detail: "a rename/merge/deprecate was detected for this feature — confirm the target(s) before retiring its Canonical Head"}
		}

	case "production_change_no_new_test":
		// CP-53 P-5 / Task-277: production paths changed without a newly added test file.
		if HasCodeChanges(tr.GitDiff) && !HasNewTestFileAdded(tr.GitDiff) {
			return &Violation{Rule: rule, Detail: "production code changed without a newly added test file in this turn"}
		}

	case "pre_existing_test_edited":
		// Task-260: hard enforce additive-tests-only. TamperedTestPaths is
		// populated from oracle.Tampered (IsTestFile M/D/R/C already filtered
		// by test_overrides on filepath.Base). Pure A (new test file) never
		// appears there, so it correctly does not fire for additive work.
		// No GitDiff fallback — a GitDiff M/D/R/C that oracle filtered via
		// override must NOT re-fire here (Critical: overrides would be ignored).
		// Callers constructing TurnResult directly must populate TamperedTestPaths.
		if len(tr.TamperedTestPaths) > 0 {
			return &Violation{
				Rule:   rule,
				Detail: "pre-existing test file(s) edited: " + strings.Join(tr.TamperedTestPaths, ", "),
			}
		}
	}
	return nil
}

var telegramMessageIDPattern = regexp.MustCompile(`(?i)message_id["':=\s]*\d+`)

// MissingTelegramSends returns the required chat ids for which no evidence
// of a successful Telegram send was found in finalMessage (Task-233,
// CP-05-05 Q-4 resolved: a real message_id in the tool-call response is the
// only acceptable evidence, not merely "the AI mentioned sending a
// message" — mirrors CP-05-03 §12.3's lesson that quoted guidance text must
// not be mistaken for actual tool use).
//
// v1 simplification: this scans FinalMessage's text for message_id
// occurrences and requires at least as many as there are required targets —
// it cannot reliably attribute a specific occurrence to a specific chat id
// from unstructured provider output. When that distinction matters (multiple
// simultaneous Telegram targets in one turn), a future iteration should
// parse structured tool-call/response logs instead of scanning FinalMessage.
func MissingTelegramSends(finalMessage string, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	matches := telegramMessageIDPattern.FindAllString(finalMessage, -1)
	if len(matches) >= len(required) {
		return nil
	}
	return append([]string(nil), required...)
}

// MissingRequiredFileArtifactOutputs returns required paths that do not exist
// as regular files under workspaceCwd. Empty workspace or empty required list
// yields nil (no violation). Paths that escape the workspace (including
// symlink escapes after EvalSymlinks) are treated as missing so they cannot
// silently satisfy the write contract.
func MissingRequiredFileArtifactOutputs(workspaceCwd string, required []string) []string {
	workspaceCwd = strings.TrimSpace(workspaceCwd)
	if workspaceCwd == "" || len(required) == 0 {
		return nil
	}
	root, err := filepath.Abs(workspaceCwd)
	if err != nil {
		return append([]string(nil), required...)
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	var missing []string
	for _, rel := range required {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			missing = append(missing, rel)
			continue
		}
		abs := filepath.Join(root, clean)
		// Resolve symlinks; if the target is outside root, treat as missing.
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		} else if !errors.Is(err, os.ErrNotExist) && !os.IsNotExist(err) {
			// Broken path / not found handled below via Stat.
			_ = err
		}
		if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
			missing = append(missing, rel)
			continue
		}
		st, err := os.Stat(abs)
		if err != nil || st.IsDir() {
			missing = append(missing, rel)
		}
	}
	return missing
}

// MissingRequiredFileArtifactStructures returns human-readable gap strings for
// required structured outputs that exist but lack one or more section headings.
// Paths that fail existence/safety are skipped (existence gate owns them).
// Format: "path (SectionA, SectionB)".
func MissingRequiredFileArtifactStructures(workspaceCwd string, required []StructuredFileArtifactOutput) []string {
	workspaceCwd = strings.TrimSpace(workspaceCwd)
	if workspaceCwd == "" || len(required) == 0 {
		return nil
	}
	// Existence-missing set so we do not double-fire structure on absent files.
	var paths []string
	for _, item := range required {
		p := strings.TrimSpace(item.Path)
		if p == "" || len(item.Sections) == 0 {
			continue
		}
		paths = append(paths, p)
	}
	missingExist := map[string]bool{}
	for _, p := range MissingRequiredFileArtifactOutputs(workspaceCwd, paths) {
		missingExist[p] = true
	}

	root, err := filepath.Abs(workspaceCwd)
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	var gaps []string
	for _, item := range required {
		rel := strings.TrimSpace(item.Path)
		if rel == "" || len(item.Sections) == 0 || missingExist[rel] {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			continue
		}
		abs := filepath.Join(root, clean)
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		missingSecs := MissingMarkdownSections(string(data), item.Sections)
		if len(missingSecs) == 0 {
			continue
		}
		gaps = append(gaps, rel+" ("+strings.Join(missingSecs, ", ")+")")
	}
	return gaps
}

// MissingMarkdownSections returns required section titles not found as ATX
// headings in content. Matching is flexible: heading levels 1–6, case-
// insensitive titles, trim whitespace, light trailing punctuation (Task-225).
func MissingMarkdownSections(content string, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	present := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		title, ok := parseATXHeadingTitle(line)
		if !ok {
			continue
		}
		present[normalizeSectionTitle(title)] = true
	}
	var missing []string
	for _, sec := range required {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}
		if !present[normalizeSectionTitle(sec)] {
			missing = append(missing, sec)
		}
	}
	return missing
}

// parseATXHeadingTitle extracts the title from a CommonMark-ish ATX heading line.
// Accepts 1–6 leading # characters; requires at least one non-# character after.
func parseATXHeadingTitle(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '#' {
		return "", false
	}
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(line) {
		return "", false
	}
	// Prefer space after hashes (CommonMark) but allow tight forms for flexibility.
	title := strings.TrimSpace(line[i:])
	if title == "" {
		return "", false
	}
	// Closed ATX: strip trailing # run.
	title = strings.TrimRight(title, "#")
	title = strings.TrimSpace(title)
	if title == "" {
		return "", false
	}
	return title, true
}

func normalizeSectionTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "#")
	s = strings.TrimSpace(s)
	for {
		trimmed := strings.TrimRight(s, ":.")
		if trimmed == s {
			break
		}
		s = strings.TrimSpace(trimmed)
	}
	return strings.ToLower(s)
}

func missingFeatureKey(commitSubjects []string, knownKeys []string) bool {
	if len(commitSubjects) == 0 {
		return true
	}
	for _, subject := range commitSubjects {
		key, ok := declaredFeatureKey(subject)
		if !ok || !isKnownFeatureKey(key, knownKeys) {
			return true
		}
	}
	return false
}

func declaredFeatureKey(subject string) (string, bool) {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "[") {
		return "", false
	}
	endType := strings.Index(subject, "]")
	if endType < 0 {
		return "", false
	}
	rest := strings.TrimSpace(subject[endType+1:])
	if !strings.HasPrefix(rest, "[") {
		return "", false
	}
	endFeature := strings.Index(rest, "]")
	if endFeature < 0 {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(rest[1:endFeature]))
	key = strings.ReplaceAll(key, " ", "-")
	key = strings.ReplaceAll(key, "_", "-")
	for strings.Contains(key, "--") {
		key = strings.ReplaceAll(key, "--", "-")
	}
	return strings.Trim(key, "-"), strings.Trim(key, "-") != ""
}

func isKnownFeatureKey(key string, knownKeys []string) bool {
	for _, known := range knownKeys {
		if known == key {
			return true
		}
	}
	return false
}
