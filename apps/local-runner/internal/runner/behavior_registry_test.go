package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBehaviorRegistryResolveUnknownID(t *testing.T) {
	r := NewBehaviorRegistry()
	if _, err := r.Resolve("not_a_behavior"); err == nil {
		t.Fatal("expected error for unknown behavior id, got nil")
	}
}

func TestBehaviorRegistryResolveKnownAliasWithoutHandler(t *testing.T) {
	r := NewBehaviorRegistry()
	// "coding" normalizes to the canonical "agent.delegate" id, but no handler
	// is registered in an empty registry, so resolution must still fail.
	if _, err := r.Resolve("coding"); err == nil {
		t.Fatal("expected error when canonical id has no registered handler")
	}
}

func TestBehaviorRegistryRegisterDuplicateFails(t *testing.T) {
	r := NewBehaviorRegistry()
	spec := BehaviorSpec{ID: BehaviorFlowControl, Scope: BehaviorScopeControl, Handler: behaviorFlowControl}
	if err := r.Register(spec); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(spec); err == nil {
		t.Fatal("expected error registering duplicate behavior id")
	}
}

func TestBehaviorRegistryDispatchUnknownFailsBeforeHandler(t *testing.T) {
	r := NewDefaultBehaviorRegistry()
	if _, err := r.Dispatch(context.Background(), "totally_unknown", BehaviorInput{}); err == nil {
		t.Fatal("expected dispatch to fail for unknown behavior id")
	}
}

func TestDefaultRegistryResolvesAllAliasesFromPack(t *testing.T) {
	r := NewDefaultBehaviorRegistry()
	aliases := []string{
		"agent.delegate", "coding", "implementation", "code",
		"hub.inline", "hub.synthesize",
		"context.produce", "context.deterministic_feature_package", "plan", "planning", "design",
		"context.render", "context.prompt_handoff",
		"command.validate", "validation.command",
		"validation.summarize",
		"artifact.audit_draft", "audit.draft",
		"telegram.notify", "notify.telegram",
		"hub.notify", "notify.hub",
		"flow.control", "flow.control_tool",
		"user.confirm",
	}
	for _, alias := range aliases {
		if _, err := r.Resolve(alias); err != nil {
			t.Errorf("Resolve(%q) failed: %v", alias, err)
		}
	}
}

func TestBehaviorAgentDelegateAssemblesPrompt(t *testing.T) {
	out, err := behaviorAgentDelegate(context.Background(), BehaviorInput{NodeID: "coder", Prompt: "do the task"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "continue" {
		t.Fatalf("status = %q, want continue", out.Status)
	}
	if len(out.NextPromptFragments) != 1 || out.NextPromptFragments[0] != "do the task" {
		t.Fatalf("unexpected prompt fragments: %#v", out.NextPromptFragments)
	}
}

func TestBehaviorAgentDelegateRejectsEmptyPrompt(t *testing.T) {
	if _, err := behaviorAgentDelegate(context.Background(), BehaviorInput{NodeID: "coder"}); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestBehaviorFlowControlParsesValidInput(t *testing.T) {
	out, err := behaviorFlowControl(context.Background(), BehaviorInput{
		RawArgs: map[string]any{"status": "done", "summary": "all clear"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "done" || out.Summary != "all clear" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestBehaviorFlowControlRejectsInvalidStatus(t *testing.T) {
	if _, err := behaviorFlowControl(context.Background(), BehaviorInput{
		RawArgs: map[string]any{"status": "bogus"},
	}); err == nil {
		t.Fatal("expected error for invalid flow_control status")
	}
}

func TestBehaviorHubInlineDelegatesToFlowControlWhenStatusPresent(t *testing.T) {
	out, err := behaviorHubInline(context.Background(), BehaviorInput{
		RawArgs: map[string]any{"status": "escalate", "summary": "cap hit"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "escalate" {
		t.Fatalf("status = %q, want escalate", out.Status)
	}
}

func TestBehaviorHubInlineDefaultsToContinueWithoutStatus(t *testing.T) {
	out, err := behaviorHubInline(context.Background(), BehaviorInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "continue" {
		t.Fatalf("status = %q, want continue", out.Status)
	}
}

func TestBehaviorContextRenderRequiresBoundPackage(t *testing.T) {
	if _, err := behaviorContextRender(context.Background(), BehaviorInput{NodeID: "coding", Prompt: "implement it"}); err == nil {
		t.Fatal("expected error when no context package is bound")
	}
}

func TestBehaviorContextProduceThenRenderRoundTrip(t *testing.T) {
	produced, err := behaviorContextProduce(context.Background(), BehaviorInput{
		WorkspaceCwd:  t.TempDir(),
		WorkflowRunID: "run-1",
		StepRunID:     "plan-1",
		Prompt:        "build the widget",
	})
	if err != nil {
		t.Fatalf("context.produce: %v", err)
	}
	if produced.Status != "done" {
		t.Fatalf("status = %q, want done", produced.Status)
	}
	pkg, ok := produced.Payload["package"].(FlowContextPackage)
	if !ok {
		t.Fatalf("expected FlowContextPackage in payload, got %#v", produced.Payload["package"])
	}

	rendered, err := behaviorContextRender(context.Background(), BehaviorInput{
		NodeID:  "coding",
		Prompt:  "implement it",
		Payload: map[string]any{"package": pkg},
	})
	if err != nil {
		t.Fatalf("context.render: %v", err)
	}
	if len(rendered.NextPromptFragments) != 1 {
		t.Fatalf("expected one rendered prompt fragment, got %d", len(rendered.NextPromptFragments))
	}
	if got := rendered.NextPromptFragments[0]; !containsAll(got, flowContextHandoffPrefix, "implement it") {
		t.Fatalf("rendered prompt missing expected markers: %s", got)
	}
}

func TestBehaviorValidationSummarizeNoIssuesIsDone(t *testing.T) {
	out, err := behaviorValidationSummarize(context.Background(), BehaviorInput{RawArgs: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "done" {
		t.Fatalf("status = %q, want done", out.Status)
	}
}

func TestBehaviorValidationSummarizeWithIssuesContinues(t *testing.T) {
	out, err := behaviorValidationSummarize(context.Background(), BehaviorInput{
		RawArgs: map[string]any{"issues": []any{"issue-1", "issue-2"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "continue" {
		t.Fatalf("status = %q, want continue", out.Status)
	}
}

func TestBehaviorCommandValidateMapsExitCode(t *testing.T) {
	ok, err := behaviorCommandValidate(context.Background(), BehaviorInput{RawArgs: map[string]any{"exitCode": 0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok.Status != "done" {
		t.Fatalf("status = %q, want done", ok.Status)
	}

	failed, err := behaviorCommandValidate(context.Background(), BehaviorInput{RawArgs: map[string]any{"exitCode": 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if failed.Status != "continue" {
		t.Fatalf("status = %q, want continue", failed.Status)
	}
}

// TestBehaviorCommandValidateHandlesJSONDecodedFloat64ExitCode is the
// regression test for BUG-NOTE-CP42 #27: RawArgs is often populated by
// decoding JSON into map[string]any, where every number (including a plain
// exit code) decodes as float64, not int. The old `v.(int)` type assertion
// silently failed for that shape and fell back to 0 — a real, nonzero exit
// code was reported as "command validation passed".
func TestBehaviorCommandValidateHandlesJSONDecodedFloat64ExitCode(t *testing.T) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"exitCode":1}`), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := decoded["exitCode"].(float64); !ok {
		t.Fatalf("test setup assumption broken: exitCode decoded as %T, want float64", decoded["exitCode"])
	}

	out, err := behaviorCommandValidate(context.Background(), BehaviorInput{RawArgs: decoded})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "continue" {
		t.Fatalf("status = %q, want continue (a JSON-decoded exitCode:1 must not read as 0/pass)", out.Status)
	}
}

// TestBehaviorValidationSummarizeCountsNonAnySliceIssues is the regression
// test for BUG-NOTE-CP42 #30: a bare `v.([]any)` type assertion only matches
// a JSON-decoded issue list. An internal Go caller passing a []string (or
// any other typed slice) used to read as empty and wrongly report "no open
// issues" / status=done.
func TestBehaviorValidationSummarizeCountsNonAnySliceIssues(t *testing.T) {
	out, err := behaviorValidationSummarize(context.Background(), BehaviorInput{
		RawArgs: map[string]any{"issues": []string{"issue-1", "issue-2"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "continue" {
		t.Fatalf("status = %q, want continue (a non-empty []string issues list must not read as empty)", out.Status)
	}
}

func TestBehaviorArtifactAuditDraftRequiresSummary(t *testing.T) {
	if _, err := behaviorArtifactAuditDraft(context.Background(), BehaviorInput{NodeID: "audit", RawArgs: map[string]any{}}); err == nil {
		t.Fatal("expected error for missing summary")
	}
	out, err := behaviorArtifactAuditDraft(context.Background(), BehaviorInput{
		NodeID:  "audit",
		RawArgs: map[string]any{"summary": "did the thing"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "continue" {
		t.Fatalf("status = %q, want continue (must wait for user.confirm)", out.Status)
	}
}

func TestBehaviorUserConfirmGatesOnExplicitConfirmation(t *testing.T) {
	pending, err := behaviorUserConfirm(context.Background(), BehaviorInput{RawArgs: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending.Status != "continue" {
		t.Fatalf("status = %q, want continue while unconfirmed", pending.Status)
	}

	confirmed, err := behaviorUserConfirm(context.Background(), BehaviorInput{RawArgs: map[string]any{"confirmed": true}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed.Status != "done" {
		t.Fatalf("status = %q, want done once confirmed", confirmed.Status)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}
