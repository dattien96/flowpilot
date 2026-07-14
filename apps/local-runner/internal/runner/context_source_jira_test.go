package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeJiraIssueAdapter struct {
	content string
	err     error
}

func (f *fakeJiraIssueAdapter) Fetch(ctx context.Context, issueKey string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.content, nil
}

type fakeJiraSprintAdapter struct {
	content string
	err     error
}

func (f *fakeJiraSprintAdapter) Fetch(ctx context.Context, sprintRef string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.content, nil
}

func TestJiraIssueSourceProducesBoundedSectionWithSourceRef(t *testing.T) {
	src := &jiraIssueSource{priority: 7, adapter: &fakeJiraIssueAdapter{content: "issue body"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{JiraIssueRef: "SCRUM-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "issue body" {
		t.Errorf("Body = %q, want %q", section.Body, "issue body")
	}
	if section.SourceRef != "jira:SCRUM-1" {
		t.Errorf("SourceRef = %q, want %q", section.SourceRef, "jira:SCRUM-1")
	}
	if section.SourceType != "jira.issue" {
		t.Errorf("SourceType = %q, want jira.issue", section.SourceType)
	}
}

func TestJiraIssueSourceNoRefIsEmptyNotError(t *testing.T) {
	src := &jiraIssueSource{priority: 7, adapter: &fakeJiraIssueAdapter{content: "should not be used"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "" || section.SourceRef != "" {
		t.Errorf("expected empty section for unconfigured issue ref, got %#v", section)
	}
}

func TestJiraIssueSourceDegradesOnAdapterError(t *testing.T) {
	src := &jiraIssueSource{priority: 7, adapter: &fakeJiraIssueAdapter{err: errors.New("jira unreachable")}}
	r := NewContextSourceRegistry()
	if err := r.Register(src); err != nil {
		t.Fatalf("register: %v", err)
	}
	sections, warnings := r.Collect(context.Background(), []string{"jira.issue"}, FlowContextHints{JiraIssueRef: "SCRUM-1"})
	if len(sections) != 0 {
		t.Errorf("expected no sections, got %#v", sections)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "jira unreachable") {
		t.Errorf("expected warning mentioning the adapter error, got %v", warnings)
	}
}

func TestJiraSprintSourceProducesBoundedSectionWithSourceRef(t *testing.T) {
	src := &jiraSprintSource{priority: 8, adapter: &fakeJiraSprintAdapter{content: "sprint body"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{JiraSprintRef: "active"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "sprint body" {
		t.Errorf("Body = %q, want %q", section.Body, "sprint body")
	}
	if section.SourceRef != "jira-sprint:active" {
		t.Errorf("SourceRef = %q, want %q", section.SourceRef, "jira-sprint:active")
	}
}

func TestJiraSprintSourceNoRefIsEmptyNotError(t *testing.T) {
	src := &jiraSprintSource{priority: 8, adapter: &fakeJiraSprintAdapter{content: "should not be used"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "" || section.SourceRef != "" {
		t.Errorf("expected empty section for unconfigured sprint ref, got %#v", section)
	}
}

// TestJiraSourcesRegisteredAndNotDefault verifies jira.issue/jira.sprint are
// registered on the default registry but not part of the default enabled set
// (Task-229, mirroring TestMcpDriverSourceNotInDefaultSet).
func TestJiraSourcesRegisteredAndNotDefault(t *testing.T) {
	for _, id := range defaultContextSourceIDs {
		if id == string(ContextSourceJiraIssue) || id == string(ContextSourceJiraSprint) {
			t.Fatalf("jira sources must not be in defaultContextSourceIDs, got %v", defaultContextSourceIDs)
		}
	}
	r := NewDefaultContextSourceRegistry()
	if _, err := r.Resolve(string(ContextSourceJiraIssue)); err != nil {
		t.Errorf("expected jira.issue to be registered on the default registry: %v", err)
	}
	if _, err := r.Resolve(string(ContextSourceJiraSprint)); err != nil {
		t.Errorf("expected jira.sprint to be registered on the default registry: %v", err)
	}
}

// TestFlowContextPackageStillHasNoVectorDependencyWithJiraSources verifies the
// no-vector guard holds even when a flow explicitly enables jira.issue/
// jira.sprint (mirrors CP-44 DOD-6's Drive-flavored equivalent).
func TestFlowContextPackageStillHasNoVectorDependencyWithJiraSources(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackageWithSources(context.Background(), workspace, FlowContextHints{
		WorkflowRunID: "run-229",
		PlanStepRunID: "plan-229",
		UserPrompt:    "agent-flow-engine",
		JiraIssueRef:  "", // no adapter wired on the process-wide default registry; unconfigured ref keeps this warning-free
		JiraSprintRef: "",
	}, []string{"feature.history", "jira.issue", "jira.sprint"})
	if err != nil {
		t.Fatalf("BuildFlowContextPackageWithSources: %v", err)
	}
	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "No vector retrieval used") {
		t.Error("rendered package must still contain 'No vector retrieval used' with jira sources enabled")
	}
}

// TestNormalizeJiraIssueTarget / TestNormalizeJiraSprintTarget exercise the
// pure normalization helpers directly (flow_executor.go), mirroring
// TestNormalizeGoogleDriveTarget's coverage shape.
func TestNormalizeJiraIssueTarget(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"__skip__":    "",
		"  SCRUM-1  ": "SCRUM-1",
		"SCRUM-42":    "SCRUM-42",
	}
	for in, want := range cases {
		if got := normalizeJiraIssueTarget(in); got != want {
			t.Errorf("normalizeJiraIssueTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeJiraSprintTarget(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"__skip__":            "",
		"__active_sprint__":   "active",
		"  42  ":              "42",
		"Sprint 7 (Board 12)": "Sprint 7 (Board 12)",
	}
	for in, want := range cases {
		if got := normalizeJiraSprintTarget(in); got != want {
			t.Errorf("normalizeJiraSprintTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAppendJiraIssueTargetPromptBoundedScope / sprint variant verify the
// target note only mentions the selected issue/sprint and never dumps
// content, mirroring appendGoogleDriveTargetPrompt's contract (CP-05-06 P-4).
func TestAppendJiraIssueTargetPromptBoundedScope(t *testing.T) {
	result := appendJiraIssueTargetPrompt("Task prompt.", "SCRUM-123")
	if !strings.Contains(result, "issue key: SCRUM-123") {
		t.Errorf("expected issue key in note, got: %s", result)
	}
	if !strings.Contains(result, "`flowpilot_jira`") {
		t.Errorf("expected jira server name mentioned, got: %s", result)
	}
	if !strings.Contains(result, "Do not search or read other Jira issues") {
		t.Errorf("expected scope-limiting rule, got: %s", result)
	}
}

func TestAppendJiraIssueTargetPromptNoOpWhenEmpty(t *testing.T) {
	result := appendJiraIssueTargetPrompt("Task prompt.", "")
	if result != "Task prompt." {
		t.Errorf("expected prompt unchanged for empty target, got: %s", result)
	}
}

func TestAppendJiraSprintTargetPromptBoundedScope(t *testing.T) {
	result := appendJiraSprintTargetPrompt("Task prompt.", "active")
	if !strings.Contains(result, "the active sprint") {
		t.Errorf("expected 'the active sprint' scope wording, got: %s", result)
	}
	explicit := appendJiraSprintTargetPrompt("Task prompt.", "42")
	if !strings.Contains(explicit, "sprint 42") {
		t.Errorf("expected explicit sprint id in scope wording, got: %s", explicit)
	}
}
