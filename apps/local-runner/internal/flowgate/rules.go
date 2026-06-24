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
	Tests        TestOutcome   `json:"tests"`
	ChangeType   string        `json:"change_type,omitempty"`
	SourceDocID  string        `json:"source_doc_id,omitempty"`
}

type Violation struct {
	Rule        Rule   `json:"rule"`
	Detail      string `json:"detail"`
	SourceDocID string `json:"source_doc_id,omitempty"`
	Declared    bool   `json:"declared,omitempty"`
}

func DefaultRules() []Rule {
	return []Rule{
		{ID: "r-ca", Scope: "step", Trigger: "code_changed", RequiredOutput: "change_audit_note", Action: "reprompt", Enabled: true},
		{ID: "r-bug", Scope: "step", Trigger: "bug_fixed", RequiredOutput: "bugfix_doc", Action: "reprompt", Enabled: true},
		{ID: "r-task", Scope: "step", Trigger: "task_referenced", RequiredOutput: "task_doc", Action: "reprompt", Enabled: true},
		{ID: "r-tests", Scope: "step", Trigger: "tests_failed", RequiredOutput: "tests_green_or_explained", Action: "block", Enabled: true},
		{ID: "r-reg", Scope: "step", Trigger: "regression_test_broke", RequiredOutput: "restore_green_without_weakening", Action: "block", Enabled: true},
		{ID: "r-dep", Scope: "step", Trigger: "removed_referenced_code", RequiredOutput: "confirm_or_update_callers", Action: "block", Enabled: true},
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
	return rules, nil
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
