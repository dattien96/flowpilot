package flowgate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Rule struct {
	ID             string `json:"id"`
	Scope          string `json:"scope"`
	Trigger        string `json:"trigger"`
	RequiredOutput string `json:"required_output"`
	Action         string `json:"action"`
	Enabled        bool   `json:"enabled"`
}

type ChangedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type TestOutcome struct {
	Ran    bool     `json:"ran"`
	Passed []string `json:"passed,omitempty"`
	Failed []string `json:"failed,omitempty"`
}

type TurnResult struct {
	RunID        string        `json:"run_id"`
	StepID       string        `json:"step_id"`
	FinalMessage string        `json:"final_message"`
	ToolCalls    []string      `json:"tool_calls,omitempty"`
	GitDiff      []ChangedFile `json:"git_diff,omitempty"`
	// WrittenPaths lists files actually written by AI tool calls in this turn.
	// Use this (not GitDiff) to decide whether the AI changed source code — GitDiff
	// includes pre-existing dirty files and runner-internal state (e.g. test_baseline.json)
	// that are invisible to the user and must not trigger change-audit requirements.
	WrittenPaths         []string    `json:"written_paths,omitempty"`
	Tests                TestOutcome `json:"tests"`
	ChangeType           string      `json:"change_type,omitempty"`
	SourceDocID          string      `json:"source_doc_id,omitempty"`
	CommitSubjects       []string    `json:"commit_subjects,omitempty"`
	ChangedPaths         []string    `json:"changed_paths,omitempty"`
	KnownFeatureKeys     []string    `json:"known_feature_keys,omitempty"`
	SuggestedFeatureKeys []string    `json:"suggested_feature_keys,omitempty"`
	// WorkspaceCwd is the project root used to check required file-artifact
	// output existence on disk (Task-223). Empty skips the filesystem check.
	WorkspaceCwd string `json:"workspace_cwd,omitempty"`
	// RequiredFileArtifactOutputs lists workspace-relative paths that must
	// exist after this turn when the active flow node has required
	// file_artifact.v1 OUTPUT bindings (Task-223 write contract).
	RequiredFileArtifactOutputs []string `json:"required_file_artifact_outputs,omitempty"`
	// RequiredStructuredFileArtifactOutputs lists required OUTPUT paths that
	// also declare instance structure.sections (Task-225). Evaluated only
	// after existence passes for each path.
	RequiredStructuredFileArtifactOutputs []StructuredFileArtifactOutput `json:"required_structured_file_artifact_outputs,omitempty"`
	// RequiredTelegramSends lists chat ids that must show evidence of a
	// successful Telegram send (a real message_id, not just a tool call —
	// CP-05-05 Q-4) after this turn, when the active flow node has required
	// telegram.v1 OUTPUT bindings (Task-233 write contract). There is no
	// filesystem artifact to check — evidence comes from FinalMessage.
	RequiredTelegramSends []string `json:"required_telegram_sends,omitempty"`
	// Task-185 (CP-43 P-2): scope-drift signals. Computed by the caller from
	// changecontract.Store + changecontract.ScopeDiff/HighSeverity before
	// Evaluate runs — flowgate cannot import changecontract (infer.go already
	// imports flowgate for ChangedFile, so the reverse import would cycle).
	//
	// ContractDeclared is true only when the AI's own [Change Contract] block
	// was found this turn (confidence=declared); false for an inferred or
	// altogether-missing contract.
	ContractDeclared bool `json:"contract_declared,omitempty"`
	// ScopeOutOfScopePaths are the diff paths outside the turn's declared
	// scope (empty for an inferred contract, which trivially matches its own
	// diff — see changecontract.ScopeDiff).
	ScopeOutOfScopePaths []string `json:"scope_out_of_scope_paths,omitempty"`
	// ScopeHighSeverity is true only when structure.Available() AND at least
	// one out-of-scope path has dependents (changecontract.HighSeverity) —
	// the sole condition under which r-scope may resolve to "block" rather
	// than "warn" (SD-21 D-5/Q-2: file-level truth alone never blocks).
	ScopeHighSeverity bool `json:"scope_high_severity,omitempty"`
	// Task-186 (CP-43 P-3): Canonical Head drift signals, computed by the
	// caller from changecontract.SpecDrifted/CodeDrifted before Evaluate runs
	// (same import-cycle reason as the Task-185 fields above).
	HeadSpecDrifted bool `json:"head_spec_drifted,omitempty"`
	HeadCodeDrifted bool `json:"head_code_drifted,omitempty"`
	// HeadAttachSpecPending is true when a spec_less feature's Head gained a
	// governing doc this turn — a re-baseline candidate (r-attach-spec), not
	// a drift (SD-21 §5: "spec_less -> current" via attach-spec is distinct
	// from "current -> spec_drifted").
	HeadAttachSpecPending bool `json:"head_attach_spec_pending,omitempty"`
	// Task-187 (CP-43 P-4): true when the caller has detected a retire intent
	// (rename/merge/deprecate) for the active feature awaiting human
	// confirmation of targets before RetireHead runs (BR-2, never automatic).
	HeadRetirePending bool `json:"head_retire_pending,omitempty"`
}

// StructuredFileArtifactOutput is a required file_artifact OUTPUT path with
// optional markdown section titles from instance config_json.structure.
type StructuredFileArtifactOutput struct {
	Path     string   `json:"path"`
	Sections []string `json:"sections,omitempty"`
}

type Violation struct {
	Rule        Rule   `json:"rule"`
	Detail      string `json:"detail"`
	SourceDocID string `json:"source_doc_id,omitempty"`
	Declared    bool   `json:"declared,omitempty"`
	// Options lists the decision choices for r-reg blocks (Task-155 decision card).
	// Populated only for regression_test_broke violations.
	Options []string `json:"options,omitempty"`
	// RegressedTests is the list of specifically-identified regressed test names.
	// Populated only for regression_test_broke violations.
	RegressedTests []string `json:"regressed_tests,omitempty"`
}

func DefaultRules() []Rule {
	return []Rule{
		{ID: "r-ca", Scope: "step", Trigger: "code_changed", RequiredOutput: "change_audit_note", Action: "reprompt", Enabled: true},
		{ID: "r-fk", Scope: "step", Trigger: "commit_feature_key_missing", RequiredOutput: "verified_feature_key", Action: "reprompt", Enabled: true},
		{ID: "r-bug", Scope: "step", Trigger: "bug_fixed", RequiredOutput: "bugfix_doc", Action: "reprompt", Enabled: true},
		{ID: "r-task", Scope: "step", Trigger: "task_referenced", RequiredOutput: "task_doc", Action: "reprompt", Enabled: true},
		{ID: "r-tests", Scope: "step", Trigger: "tests_failed", RequiredOutput: "tests_green_or_explained", Action: "block", Enabled: true},
		{ID: "r-reg", Scope: "step", Trigger: "regression_test_broke", RequiredOutput: "restore_green_without_weakening", Action: "block", Enabled: true},
		{ID: "r-dep", Scope: "step", Trigger: "removed_referenced_code", RequiredOutput: "confirm_or_update_callers", Action: "block", Enabled: true},
		// Task-223: required file_artifact.v1 OUTPUT paths must exist after the turn.
		{ID: "r-artifact-output", Scope: "step", Trigger: "required_artifact_output_missing", RequiredOutput: "file_artifact_paths", Action: "reprompt", Enabled: true},
		// Task-225: required structured file_artifact OUTPUT paths must contain
		// the declared section headings after the file exists.
		{ID: "r-artifact-output-structure", Scope: "step", Trigger: "required_artifact_output_structure_missing", RequiredOutput: "file_artifact_structure", Action: "reprompt", Enabled: true},
		// Task-233 (CP-05-05 P-4): required telegram.v1 OUTPUT bindings must
		// show real message_id evidence of a successful send after the turn —
		// not just that a tool was called (CP-05-03 §12.3's quote-guidance
		// false-positive lesson).
		{ID: "r-artifact-telegram-sent", Scope: "step", Trigger: "required_telegram_send_missing", RequiredOutput: "telegram_message_id", Action: "reprompt", Enabled: true},
		// Task-185 (CP-43 P-2): the AI did not declare a Change Contract before
		// editing (an inferred contract was used instead) — nag once, never block.
		{ID: "r-contract", Scope: "step", Trigger: "code_changed_no_contract", RequiredOutput: "declared_change_contract", Action: "reprompt", Enabled: true},
		// Task-185 (CP-43 P-2): an edit landed outside the turn's declared scope.
		// Warn by default; checkRule downgrades any configured "block" back to
		// "warn" unless TurnResult.ScopeHighSeverity is true (SD-21 D-5/Q-2).
		{ID: "r-scope", Scope: "step", Trigger: "edit_outside_declared_scope", RequiredOutput: "confirm_or_revert_out_of_scope", Action: "warn", Enabled: true},
		// Task-186 (CP-43 P-3): a feature's governing spec changed since its
		// Canonical Head last recorded it. Surfaced for human reconciliation,
		// never auto-rewritten (BR-2).
		{ID: "r-spec-drift", Scope: "step", Trigger: "governing_spec_changed", RequiredOutput: "reconcile_canonical_head", Action: "warn", Enabled: true},
		// Task-186 (CP-43 P-3): out-of-contract code change with no
		// corresponding spec/behavior update to explain it.
		{ID: "r-code-drift", Scope: "step", Trigger: "code_diverged_from_intent", RequiredOutput: "reconcile_or_revert", Action: "warn", Enabled: true},
		// Task-186 (CP-43 P-3): a spec_less feature just gained its first
		// governing doc — a re-baseline candidate, requires human confirmation
		// before the Head is rebaselined (not auto-applied).
		{ID: "r-attach-spec", Scope: "step", Trigger: "governing_spec_added_to_spec_less", RequiredOutput: "confirm_spec_matches_behavior_then_rebaseline", Action: "approve", Enabled: true},
		// Task-187 (CP-43 P-4): a feature is being renamed/merged/deprecated.
		// Requires human confirmation of the target(s) before RetireHead runs —
		// never automatic (BR-2).
		{ID: "r-retire", Scope: "step", Trigger: "feature_rename_merge_or_deprecate", RequiredOutput: "confirm_targets_then_retire", Action: "approve", Enabled: true},
	}
}

func LoadRules(settingsDir string) ([]Rule, error) {
	path := filepath.Join(settingsDir, "flow-rules.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultRules(), nil
		}
		return nil, err
	}
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	// Task-223: old flow-rules.json files predate r-artifact-output; merge any
	// default rules missing by ID so new enforcement is not silently disabled.
	return MergeDefaultRules(rules), nil
}

// MergeDefaultRules returns loaded rules with any DefaultRules entries whose
// ID is absent appended (preserving loaded order and settings first).
func MergeDefaultRules(loaded []Rule) []Rule {
	if len(loaded) == 0 {
		return DefaultRules()
	}
	seen := make(map[string]bool, len(loaded))
	for _, r := range loaded {
		if r.ID != "" {
			seen[r.ID] = true
		}
	}
	out := append([]Rule(nil), loaded...)
	for _, d := range DefaultRules() {
		if !seen[d.ID] {
			out = append(out, d)
		}
	}
	return out
}

func SaveRules(settingsDir string, rules []Rule) error {
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(settingsDir, "flow-rules.json"), data, 0o644)
}
