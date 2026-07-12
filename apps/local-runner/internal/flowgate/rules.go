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
