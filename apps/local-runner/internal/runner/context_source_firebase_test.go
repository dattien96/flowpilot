package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeFirebaseCrashlyticsAdapter struct {
	content string
	err     error
}

func (f *fakeFirebaseCrashlyticsAdapter) Fetch(ctx context.Context, crashRef string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.content, nil
}

func TestFirebaseCrashlyticsSourceProducesBoundedSectionWithSourceRef(t *testing.T) {
	src := &firebaseCrashlyticsSource{priority: 9, adapter: &fakeFirebaseCrashlyticsAdapter{content: "crash body"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{FirebaseCrashRef: "crash-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "crash body" {
		t.Errorf("Body = %q, want %q", section.Body, "crash body")
	}
	if section.SourceRef != "firebase:crash-123" {
		t.Errorf("SourceRef = %q, want %q", section.SourceRef, "firebase:crash-123")
	}
}

func TestFirebaseCrashlyticsSourceNoRefIsEmptyNotError(t *testing.T) {
	src := &firebaseCrashlyticsSource{priority: 9, adapter: &fakeFirebaseCrashlyticsAdapter{content: "should not be used"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "" || section.SourceRef != "" {
		t.Errorf("expected empty section for unconfigured crash ref, got %#v", section)
	}
}

// TestFirebaseCrashlyticsSourceNoAdapterDegrades verifies the intentional
// "no production adapter wired" design (Task-231 package doc): a configured
// crash ref with no adapter degrades to a Collect warning, not a hard
// failure — matching mcp.driver/jira.issue's own nil-adapter contract.
func TestFirebaseCrashlyticsSourceNoAdapterDegrades(t *testing.T) {
	src := &firebaseCrashlyticsSource{priority: 9}
	r := NewContextSourceRegistry()
	if err := r.Register(src); err != nil {
		t.Fatalf("register: %v", err)
	}
	sections, warnings := r.Collect(context.Background(), []string{"firebase.crashlytics"}, FlowContextHints{FirebaseCrashRef: "crash-123"})
	if len(sections) != 0 {
		t.Errorf("expected no sections, got %#v", sections)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no adapter configured") {
		t.Errorf("expected 'no adapter configured' warning, got %v", warnings)
	}
}

func TestFirebaseCrashlyticsSourceDegradesOnAdapterError(t *testing.T) {
	src := &firebaseCrashlyticsSource{priority: 9, adapter: &fakeFirebaseCrashlyticsAdapter{err: errors.New("firebase unreachable")}}
	r := NewContextSourceRegistry()
	if err := r.Register(src); err != nil {
		t.Fatalf("register: %v", err)
	}
	sections, warnings := r.Collect(context.Background(), []string{"firebase.crashlytics"}, FlowContextHints{FirebaseCrashRef: "crash-123"})
	if len(sections) != 0 {
		t.Errorf("expected no sections, got %#v", sections)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "firebase unreachable") {
		t.Errorf("expected warning mentioning the adapter error, got %v", warnings)
	}
}

func TestFirebaseCrashlyticsSourceRegisteredAndNotDefault(t *testing.T) {
	for _, id := range defaultContextSourceIDs {
		if id == string(ContextSourceFirebaseCrashlytics) {
			t.Fatalf("firebase.crashlytics must not be in defaultContextSourceIDs, got %v", defaultContextSourceIDs)
		}
	}
	r := NewDefaultContextSourceRegistry()
	if _, err := r.Resolve(string(ContextSourceFirebaseCrashlytics)); err != nil {
		t.Errorf("expected firebase.crashlytics to be registered on the default registry: %v", err)
	}
}

func TestFlowContextPackageStillHasNoVectorDependencyWithFirebaseSource(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackageWithSources(context.Background(), workspace, FlowContextHints{
		WorkflowRunID:    "run-231",
		PlanStepRunID:    "plan-231",
		UserPrompt:       "agent-flow-engine",
		FirebaseCrashRef: "",
	}, []string{"feature.history", "firebase.crashlytics"})
	if err != nil {
		t.Fatalf("BuildFlowContextPackageWithSources: %v", err)
	}
	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "## Context") {
		t.Error("rendered package must contain context header with firebase.crashlytics enabled")
	}
}

func TestNormalizeFirebaseCrashTarget(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"__skip__":       "",
		"  crash-abc  ":  "crash-abc",
		"crash-xyz-9000": "crash-xyz-9000",
	}
	for in, want := range cases {
		if got := normalizeFirebaseCrashTarget(in); got != want {
			t.Errorf("normalizeFirebaseCrashTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppendFirebaseCrashTargetPromptBoundedScope(t *testing.T) {
	result := appendFirebaseCrashTargetPrompt("Task prompt.", "crash-123")
	if !strings.Contains(result, "crash issue id: crash-123") {
		t.Errorf("expected crash issue id in note, got: %s", result)
	}
	if !strings.Contains(result, "`flowpilot_firebase`") {
		t.Errorf("expected firebase server name mentioned, got: %s", result)
	}
	if !strings.Contains(result, "Do not search or read other Crashlytics issues") {
		t.Errorf("expected scope-limiting rule, got: %s", result)
	}
}

func TestAppendFirebaseCrashTargetPromptNoOpWhenEmpty(t *testing.T) {
	result := appendFirebaseCrashTargetPrompt("Task prompt.", "")
	if result != "Task prompt." {
		t.Errorf("expected prompt unchanged for empty target, got: %s", result)
	}
}
