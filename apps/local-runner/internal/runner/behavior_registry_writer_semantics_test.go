package runner

import (
	"context"
	"testing"
)

// CP-55 P-1 (Task-263): registry coverage for the new explicit writer/freeze
// behavior ids. Kept in a new file — behavior_registry_test.go is untouched.

func TestDefaultRegistryResolvesAgentCode(t *testing.T) {
	r := NewDefaultBehaviorRegistry()
	spec, err := r.Resolve("agent.code")
	if err != nil {
		t.Fatalf("Resolve(agent.code) failed: %v", err)
	}
	if spec.ID != BehaviorAgentCode {
		t.Fatalf("resolved ID = %q, want %q", spec.ID, BehaviorAgentCode)
	}
	if spec.Scope != BehaviorScopeDelegate {
		t.Fatalf("scope = %q, want %q", spec.Scope, BehaviorScopeDelegate)
	}
}

func TestDefaultRegistryResolvesContractFreeze(t *testing.T) {
	r := NewDefaultBehaviorRegistry()
	spec, err := r.Resolve("contract.freeze")
	if err != nil {
		t.Fatalf("Resolve(contract.freeze) failed: %v", err)
	}
	if spec.ID != BehaviorContractFreeze {
		t.Fatalf("resolved ID = %q, want %q", spec.ID, BehaviorContractFreeze)
	}
	if spec.Scope != BehaviorScopeInline {
		t.Fatalf("scope = %q, want %q", spec.Scope, BehaviorScopeInline)
	}
}

// TestAgentCodeReusesDelegateDispatchWithoutChangingProviderPayload proves
// agent.code and agent.delegate produce byte-identical dispatch output for
// the same input, since agent.code registers behaviorAgentDelegate verbatim
// (CP-55 3.5: "may delegate to the existing provider prompt assembly without
// changing provider payload semantics").
func TestAgentCodeReusesDelegateDispatchWithoutChangingProviderPayload(t *testing.T) {
	in := BehaviorInput{NodeID: "writer", Prompt: "implement the fix"}
	delegateOut, err := DefaultBehaviorRegistry().Dispatch(context.Background(), "agent.delegate", in)
	if err != nil {
		t.Fatalf("agent.delegate dispatch: %v", err)
	}
	codeOut, err := DefaultBehaviorRegistry().Dispatch(context.Background(), "agent.code", in)
	if err != nil {
		t.Fatalf("agent.code dispatch: %v", err)
	}
	if codeOut.Status != delegateOut.Status {
		t.Fatalf("agent.code status = %q, want it to match agent.delegate status %q", codeOut.Status, delegateOut.Status)
	}
	if len(codeOut.NextPromptFragments) != len(delegateOut.NextPromptFragments) {
		t.Fatalf("agent.code prompt fragments = %#v, want to match agent.delegate %#v", codeOut.NextPromptFragments, delegateOut.NextPromptFragments)
	}
	for i := range delegateOut.NextPromptFragments {
		if codeOut.NextPromptFragments[i] != delegateOut.NextPromptFragments[i] {
			t.Fatalf("agent.code prompt fragment[%d] = %q, want %q (identical provider payload)", i, codeOut.NextPromptFragments[i], delegateOut.NextPromptFragments[i])
		}
	}
}

// TestContractFreezePlaceholderFailsClosed guards the P-1 constraint that the
// placeholder handler "must not silently succeed or mutate state": any
// dispatch must return an error, never a done/continue success status.
func TestContractFreezePlaceholderFailsClosed(t *testing.T) {
	out, err := DefaultBehaviorRegistry().Dispatch(context.Background(), "contract.freeze", BehaviorInput{NodeID: "freeze"})
	if err == nil {
		t.Fatalf("expected contract.freeze P-1 placeholder to fail closed, got success output %#v", out)
	}
}

func TestIsCodeWritingBehaviorClassifiesOnlyAgentCode(t *testing.T) {
	cases := []struct {
		id   BehaviorID
		want bool
	}{
		{BehaviorAgentCode, true},
		{BehaviorAgentDelegate, false},
		{BehaviorContractFreeze, false},
		{BehaviorHubInline, false},
		{BehaviorID(""), false},
	}
	for _, tc := range cases {
		if got := IsCodeWritingBehavior(tc.id); got != tc.want {
			t.Errorf("IsCodeWritingBehavior(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// TestExistingBehaviorAliasesStillResolveUnchanged pins every pre-existing
// alias's canonical target through the runner registry. Codex flagged the
// underlying agentpack.NormalizeBehaviorID CRITICAL (11 direct callers, 56
// impacted symbols, 5 processes) before the CP-55 P-1 self-mapping edit; this
// is the mandatory regression guard that pairs with it.
func TestExistingBehaviorAliasesStillResolveUnchanged(t *testing.T) {
	r := NewDefaultBehaviorRegistry()
	cases := map[string]BehaviorID{
		"agent.delegate":                        BehaviorAgentDelegate,
		"coding":                                BehaviorAgentDelegate,
		"implementation":                        BehaviorAgentDelegate,
		"code":                                  BehaviorAgentDelegate,
		"hub.inline":                            BehaviorHubInline,
		"hub.synthesize":                        BehaviorHubInline,
		"context.produce":                       BehaviorContextProduce,
		"context.deterministic_feature_package": BehaviorContextProduce,
		"context.deterministic_feature_context": BehaviorContextProduce,
		"plan":                                  BehaviorContextProduce,
		"planning":                              BehaviorContextProduce,
		"design":                                BehaviorContextProduce,
		"context.render":                        BehaviorContextRender,
		"context.prompt_handoff":                BehaviorContextRender,
		"command.validate":                      BehaviorCommandValidate,
		"validation.command":                    BehaviorCommandValidate,
		"validation.summarize":                  BehaviorValidationSummarize,
		"artifact.audit_draft":                  BehaviorArtifactAuditDraft,
		"audit.draft":                           BehaviorArtifactAuditDraft,
		"telegram.notify":                       BehaviorTelegramNotify,
		"notify.telegram":                       BehaviorTelegramNotify,
		"hub.notify":                            BehaviorHubNotify,
		"notify.hub":                            BehaviorHubNotify,
		"flow.control":                          BehaviorFlowControl,
		"flow.control_tool":                     BehaviorFlowControl,
		"user.confirm":                          BehaviorUserConfirm,
	}
	for alias, want := range cases {
		spec, err := r.Resolve(alias)
		if err != nil {
			t.Errorf("Resolve(%q) failed: %v", alias, err)
			continue
		}
		if spec.ID != want {
			t.Errorf("Resolve(%q).ID = %q, want %q (existing alias resolution must not change)", alias, spec.ID, want)
		}
	}
}
