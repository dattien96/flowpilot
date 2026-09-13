// Package skilllearn implements CP-23 Phase 3 (Task-336): Mistake-to-Skill
// Promotion and Skillpack Sync ("Thăng cấp bài học thành Skill và Đồng bộ
// Skillpack").
//
// Promotion Ladder (CP-23 D-4):
//
//	single incident    → DriftEvent persisted by Task-335 under
//	                     <workspace>/.flowpilot/workflow_drift_events.json
//	>= 2 same pattern  → LessonCandidate (AggregateDriftEvents), persisted as
//	                     JSON under
//	                     <workspace>/.flowpilot/workflow_lesson_candidates.json
//	human approval     → CompactRuleCard (for Task-334's BudgetPacker) plus a
//	                     standard SKILL.md exported either into the target
//	                     project's .agents/.claude/.grok skills dirs or into
//	                     the platform core flow-pack tree
//	                     (apps/local-runner/internal/skillpack/flow-pack).
//
// T-1 HARD RULE: no code path in this package ever creates a permanent skill
// file without explicit human approval. Candidates stay in StatusCandidate
// until the user calls PromoterService.ApproveCandidateAndExport or
// RejectCandidate; UpsertCandidates (the only store ingestion path) forces
// StatusCandidate and there is no background exporter.
//
// Provider-agnostic parity: every function is deterministic pure Go — 0 LLM,
// 0 tokens — so Claude / Codex / Grok / any provider produce byte-identical
// candidates, rule cards and SKILL.md content for identical inputs.
package skilllearn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/driftdetect"
)

// CandidateStatus is the lifecycle state of a LessonCandidate
// (Task-336 §11 Code Guide).
type CandidateStatus string

const (
	// StatusCandidate: proposed lesson waiting for human review (T-1).
	StatusCandidate CandidateStatus = "candidate"
	// StatusApproved: the human approved; export pending/attempted.
	StatusApproved CandidateStatus = "approved"
	// StatusRejected: the human rejected the lesson.
	StatusRejected CandidateStatus = "rejected"
	// StatusPromoted: the lesson was exported as a SKILL.md file.
	StatusPromoted CandidateStatus = "promoted"
)

// LessonCandidate is one proposed lesson distilled from repeated drift events
// (Task-336 §11 Code Guide).
type LessonCandidate struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	Title             string          `json:"title"`
	Group             string          `json:"group"` // "common", "android", "golang",...
	TriggerPattern    string          `json:"trigger_pattern"`
	AntiPattern       string          `json:"anti_pattern"`
	PreferredBehavior string          `json:"preferred_behavior"`
	RepeatCount       int             `json:"repeat_count"`
	SupportingRunIDs  []string        `json:"supporting_run_ids"`
	Status            CandidateStatus `json:"status"`
}

// SkillExportOptions controls where PromoteCandidateToSkill writes the
// exported SKILL.md (Task-336 §11 Code Guide shape plus the additive options
// this task needs for provider dirs, testable core roots and conflict
// acknowledgment).
type SkillExportOptions struct {
	// WorkspaceRoot is the root directory of the target project
	// (ExportToCore=false) or of the FlowPilot repo (ExportToCore=true). It
	// must already exist; the exporter never creates it.
	WorkspaceRoot string `json:"workspace_root"`
	// ExportToCore selects the platform-core destination
	// (<root>/internal/skillpack/flow-pack/<group>/<slug>/SKILL.md) instead of
	// the target-project skills dirs.
	ExportToCore bool `json:"export_to_core"`
	// SkillDirs lists the provider skill roots relative to WorkspaceRoot for
	// target-project exports (DOD item 5: .agents/.claude/.grok). Empty
	// defaults to [".agents/skills"]. See ProjectSkillDirsAll.
	SkillDirs []string `json:"skill_dirs,omitempty"`
	// FlowPackRoot overrides the core flow-pack root; empty defaults to
	// <WorkspaceRoot>/internal/skillpack/flow-pack. The override keeps core
	// exports testable in temporary directories (additive-tests-only: tests
	// must never write into the real flow-pack tree).
	FlowPackRoot string `json:"flow_pack_root,omitempty"`
	// Force acknowledges a core-skill conflict warning (DOD item 10). When a
	// conflict is detected and Force is false the export fails with a
	// *ConflictError; with Force=true the export proceeds.
	Force bool `json:"force,omitempty"`
}

// ProjectSkillDirsAll lists the per-provider skill roots of Task-336 DOD item
// 5, relative to the target project root. Callers pass this (or a subset) via
// SkillExportOptions.SkillDirs to mirror what skillpack.Install populates.
var ProjectSkillDirsAll = []string{
	filepath.Join(".agents", "skills"),
	filepath.Join(".claude", "skills"),
	filepath.Join(".grok", "skills"),
}

// defaultProjectSkillDir is the Code Guide §11 step 2 destination
// (.agents/skills) used when SkillDirs is empty.
var defaultProjectSkillDir = filepath.Join(".agents", "skills")

// minPromotionThreshold is the CP-23 D-4 invariant: a lesson candidate is
// never distilled from a single incident — the same drift pattern must repeat
// at least twice.
const minPromotionThreshold = 2

// ConflictError reports that a candidate's content collides with the
// safe-fix-contract core skills (Task-336 Constraint + DOD item 10). The
// export is refused until the caller explicitly acknowledges the conflict by
// setting SkillExportOptions.Force=true.
type ConflictError struct {
	CandidateID string
	Reason      string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("skilllearn: candidate %q conflicts with the safe-fix-contract core skills: %s — re-run with SkillExportOptions.Force=true to acknowledge",
		e.CandidateID, e.Reason)
}

// PromoteCandidateToSkill converts a candidate into a standard SKILL.md file
// on disk (Task-336 §11 Code Guide). Destination per opts:
//
//   - target project: <WorkspaceRoot>/<skill-dir>/<group>/<slug>/SKILL.md
//     (skill-dir defaults to .agents/skills; see SkillDirs);
//   - core flow-pack: <WorkspaceRoot>/internal/skillpack/flow-pack/<group>/<slug>/SKILL.md
//     (overridable via FlowPackRoot).
//
// Duplicate slugs get a new version directory (<slug>-v2, -v3, ...) — an
// existing file is never overwritten (Task-336 Open Question). A core-skill
// conflict (DOD item 10) fails the export with *ConflictError unless
// opts.Force is set. Status transitions are owned by PromoterService: this
// free function receives the candidate by value and cannot persist state, so
// it only validates that the status is exportable (candidate/approved — a
// rejected lesson is never exportable) and performs the export.
func PromoteCandidateToSkill(candidate LessonCandidate, opts SkillExportOptions) error {
	_, err := SkillExporter{}.Export(candidate, opts)
	return err
}

// signalLessonText is one row of the deterministic signal→lesson mapping
// (see signalLessons).
type signalLessonText struct {
	Trigger           string
	AntiPattern       string
	PreferredBehavior string
}

// signalLessons is the deterministic mapping from a Task-335 drift signal name
// to the lesson text (Task-336: "derive TriggerPattern/AntiPattern/
// PreferredBehavior deterministically from the signal names + note text").
// The wording mirrors driftdetect.signalAdvisories (Task-335) so a promoted
// lesson and the live drift correction notes never disagree.
var signalLessons = map[string]signalLessonText{
	driftdetect.SignalOutOfScopeEdit: {
		Trigger:           "turns that edit files outside the declared scope",
		AntiPattern:       "Editing files outside the declared scope (r-scope ScopeOutOfScopePaths violation).",
		PreferredBehavior: "Touch only files inside the declared scope; propose an explicit scope change first for anything else.",
	},
	driftdetect.SignalRepeatedTestFailure: {
		Trigger:           "turns that re-run the same failing test without changing strategy",
		AntiPattern:       "Re-running the same failing test >= 2 consecutive turns without changing the fix strategy.",
		PreferredBehavior: "Read the failure carefully and change strategy after the first repeated failure (different fix, different test, or ask the user).",
	},
	driftdetect.SignalApologyLoop: {
		Trigger:           "turns stuck in an apology or filler loop across >= 2 consecutive turns",
		AntiPattern:       "Repeatedly apologizing or emitting filler phrases without changing the approach.",
		PreferredBehavior: "Skip apologies; state the root cause and the new plan, then execute it.",
	},
	driftdetect.SignalZeroDeltaProgress: {
		Trigger:           "turns that consume more than 2000 tokens with zero file delta",
		AntiPattern:       "Burning tokens with no file or artifact delta (CP-23 R-2 guard).",
		PreferredBehavior: "Produce a concrete file delta or present a clear plan before heavy exploration.",
	},
}

// canonicalSignalOrder is the deterministic citation order for multi-signal
// lessons: Task-335 weight order (heaviest first), then unknown signals in
// first-seen order. Same ordering rule as driftdetect.GenerateSystemNote.
var canonicalSignalOrder = []string{
	driftdetect.SignalOutOfScopeEdit,
	driftdetect.SignalRepeatedTestFailure,
	driftdetect.SignalApologyLoop,
	driftdetect.SignalZeroDeltaProgress,
}

// maxNoteRunes caps the drift SystemNotePrompt appended to PreferredBehavior
// so generated skills stay compact (token hygiene, CP-23 Phase 1 spirit).
const maxNoteRunes = 400

// lessonForSignal returns the lesson text for a signal name. Unknown signals
// (future Task-335 heuristics) get a generic deterministic fallback so the
// aggregator never drops them.
func lessonForSignal(signal string) signalLessonText {
	if lesson, ok := signalLessons[signal]; ok {
		return lesson
	}
	return signalLessonText{
		Trigger:           fmt.Sprintf("turns triggering the drift signal %q", signal),
		AntiPattern:       fmt.Sprintf("Repeated drift signal %s (unrecognized heuristic).", signal),
		PreferredBehavior: "Re-evaluate the current approach whenever this drift signal fires.",
	}
}

// normalizeSignals trims, drops empties and de-duplicates a signal list,
// ordering canonical signals first (weight order) then unknown signals in
// received order — deterministic regardless of input order.
func normalizeSignals(signals []string) []string {
	seen := make(map[string]bool, len(signals))
	unknown := make([]string, 0, len(signals))
	for _, raw := range signals {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if isCanonicalSignal(s) {
			continue // appended below in canonical order
		}
		unknown = append(unknown, s)
	}
	out := make([]string, 0, len(seen))
	for _, canonical := range canonicalSignalOrder {
		if seen[canonical] {
			out = append(out, canonical)
		}
	}
	return append(out, unknown...)
}

func isCanonicalSignal(signal string) bool {
	for _, canonical := range canonicalSignalOrder {
		if signal == canonical {
			return true
		}
	}
	return false
}

// mergeSignals unions b into a keeping normalizeSignals ordering.
func mergeSignals(a, b []string) []string {
	return normalizeSignals(append(append([]string(nil), a...), b...))
}

// patternKey joins the normalized signal names with "+" — the grouping key of
// AggregateDriftEvents. Identical signal sets collapse into one pattern.
func patternKey(signals []string) string {
	return strings.Join(signals, "+")
}

// AggregateDriftEvents consumes the persisted Task-335 drift events and
// distills LessonCandidates from repeated wrong-way patterns (Task-336 §11
// Code Guide; CP-23 Phase 3 Promotion Ladder).
//
// Deterministic mapping rules (documented contract):
//
//  1. Dedupe: events are deduped by (RunID, TurnID) keeping the first
//     occurrence — CA-838 reviewer note: gate-resume replays append duplicate
//     JSONL lines for the same turn. Events with an empty TurnID are kept as
//     -is (not dedupeable). Events without any TriggeredSignals are skipped:
//     no signal, no pattern, nothing to learn.
//  2. Pattern key: the event's TriggeredSignals are normalized (trimmed,
//     lower-cased, de-duplicated, canonical weight order) and joined with
//     "+" (e.g. "out_of_scope_edit+repeated_test_failure"). Events group by
//     that key.
//  3. Threshold: a group becomes a candidate only when len(group) >=
//     threshold; threshold < 2 is clamped to 2 (CP-23 D-4 — never from a
//     single incident).
//  4. Candidate fields:
//     - RepeatCount = len(group);
//     - SupportingRunIDs = distinct non-empty RunIDs in first-appearance
//     order;
//     - TriggerPattern = per-signal trigger phrases joined with "; "
//     (signalLessons mapping, canonical order);
//     - AntiPattern = per-signal anti-pattern sentences joined with "; ";
//     - PreferredBehavior = per-signal preferred-behavior sentences joined
//     with "; ", plus the first non-empty SystemNotePrompt of the group
//     (input order, capped at 400 runes) appended as "Drift correction
//     note:" — the note-text contribution required by Task-336;
//     - Title = "Repeated drift: <humanized signals>" (underscores → spaces,
//     joined with " + ");
//     - ID = "lc-" + first 12 hex chars of sha256(patternKey) — stable per
//     pattern so re-aggregation upserts instead of duplicating;
//     - Group = "common" (always installed for every platform; the user can
//     retarget via PromoterService.UpdateCandidate before approval);
//     - ProjectID = "" — DriftEvent carries no project identity (Task-335
//     shape), so the caller that maps RunID→project fills it (via
//     UpdateCandidate or before UpsertCandidates);
//     - Status = StatusCandidate (T-1: aggregation never pre-approves).
//
// Output order: groups sorted lexicographically by pattern key. Pure
// function: identical input → identical output (provider-agnostic parity).
func AggregateDriftEvents(events []driftdetect.DriftEvent, threshold int) []LessonCandidate {
	effective := threshold
	if effective < minPromotionThreshold {
		effective = minPromotionThreshold
	}

	grouped := make(map[string][]driftdetect.DriftEvent)
	seenTurns := make(map[string]bool)
	for _, ev := range events {
		signals := normalizeSignals(ev.TriggeredSignals)
		if len(signals) == 0 {
			continue
		}
		if strings.TrimSpace(ev.TurnID) != "" {
			key := strings.TrimSpace(ev.RunID) + "\x1f" + strings.TrimSpace(ev.TurnID)
			if seenTurns[key] {
				continue
			}
			seenTurns[key] = true
		}
		key := patternKey(signals)
		grouped[key] = append(grouped[key], ev)
	}

	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]LessonCandidate, 0, len(keys))
	for _, key := range keys {
		group := grouped[key]
		if len(group) < effective {
			continue
		}
		out = append(out, candidateFromGroup(key, group))
	}
	return out
}

// candidateFromGroup builds the LessonCandidate for one pattern group
// (mapping rules documented on AggregateDriftEvents).
func candidateFromGroup(patternKey string, events []driftdetect.DriftEvent) LessonCandidate {
	var union []string
	for _, ev := range events {
		union = mergeSignals(union, ev.TriggeredSignals)
	}

	triggers := make([]string, 0, len(union))
	antis := make([]string, 0, len(union))
	preferred := make([]string, 0, len(union))
	humanized := make([]string, 0, len(union))
	for _, signal := range union {
		lesson := lessonForSignal(signal)
		triggers = append(triggers, lesson.Trigger)
		antis = append(antis, lesson.AntiPattern)
		preferred = append(preferred, lesson.PreferredBehavior)
		humanized = append(humanized, strings.ReplaceAll(signal, "_", " "))
	}

	behavior := strings.Join(preferred, "; ")
	if note := firstNonEmptyNote(events); note != "" {
		behavior += "\n\nDrift correction note: " + truncateRunes(note, maxNoteRunes)
	}

	runIDs := make([]string, 0, len(events))
	seenRuns := make(map[string]bool, len(events))
	for _, ev := range events {
		run := strings.TrimSpace(ev.RunID)
		if run == "" || seenRuns[run] {
			continue
		}
		seenRuns[run] = true
		runIDs = append(runIDs, run)
	}

	sum := sha256.Sum256([]byte(patternKey))
	return LessonCandidate{
		ID:                "lc-" + hex.EncodeToString(sum[:])[:12],
		ProjectID:         "",
		Title:             "Repeated drift: " + strings.Join(humanized, " + "),
		Group:             "common",
		TriggerPattern:    strings.Join(triggers, "; "),
		AntiPattern:       strings.Join(antis, "; "),
		PreferredBehavior: behavior,
		RepeatCount:       len(events),
		SupportingRunIDs:  runIDs,
		Status:            StatusCandidate,
	}
}

// firstNonEmptyNote returns the first non-empty SystemNotePrompt of the group
// in input order (deterministic) with surrounding whitespace trimmed.
func firstNonEmptyNote(events []driftdetect.DriftEvent) string {
	for _, ev := range events {
		if note := strings.TrimSpace(ev.SystemNotePrompt); note != "" {
			return note
		}
	}
	return ""
}

// truncateRunes caps s at max runes, appending "..." when truncated.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
