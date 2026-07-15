package runner

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

var (
	defaultBehaviorRegistryOnce sync.Once
	defaultBehaviorRegistry     *BehaviorRegistry
)

// DefaultBehaviorRegistry returns the process-wide registry of core CP-42
// behaviors, built once on first use. Call sites dispatch through this
// instead of constructing their own registry so behavior handlers stay a
// single source of truth.
func DefaultBehaviorRegistry() *BehaviorRegistry {
	defaultBehaviorRegistryOnce.Do(func() {
		defaultBehaviorRegistry = NewDefaultBehaviorRegistry()
	})
	return defaultBehaviorRegistry
}

// NewDefaultBehaviorRegistry returns a registry pre-populated with the core
// CP-42 behavior IDs (P-5). Each handler enforces its own contract in Go;
// pack/definition data only ever selects a behavior ID, never the handler
// implementation.
func NewDefaultBehaviorRegistry() *BehaviorRegistry {
	r := NewBehaviorRegistry()
	mustRegister(r, BehaviorSpec{ID: BehaviorAgentDelegate, Scope: BehaviorScopeDelegate, Handler: behaviorAgentDelegate})
	mustRegister(r, BehaviorSpec{ID: BehaviorHubInline, Scope: BehaviorScopeInline, Handler: behaviorHubInline})
	mustRegister(r, BehaviorSpec{ID: BehaviorContextProduce, Scope: BehaviorScopeInline, Handler: behaviorContextProduce})
	mustRegister(r, BehaviorSpec{ID: BehaviorContextRender, Scope: BehaviorScopeInline, Handler: behaviorContextRender})
	mustRegister(r, BehaviorSpec{ID: BehaviorCommandValidate, Scope: BehaviorScopeInline, Handler: behaviorCommandValidate})
	mustRegister(r, BehaviorSpec{ID: BehaviorValidationSummarize, Scope: BehaviorScopeInline, Handler: behaviorValidationSummarize})
	mustRegister(r, BehaviorSpec{ID: BehaviorArtifactAuditDraft, Scope: BehaviorScopeInline, Handler: behaviorArtifactAuditDraft})
	mustRegister(r, BehaviorSpec{ID: BehaviorTelegramNotify, Scope: BehaviorScopeInline, Handler: behaviorTelegramNotify})
	mustRegister(r, BehaviorSpec{ID: BehaviorHubNotify, Scope: BehaviorScopeInline, Handler: behaviorHubNotify})
	mustRegister(r, BehaviorSpec{ID: BehaviorFlowControl, Scope: BehaviorScopeControl, Handler: behaviorFlowControl})
	mustRegister(r, BehaviorSpec{ID: BehaviorUserConfirm, Scope: BehaviorScopeControl, Handler: behaviorUserConfirm})
	return r
}

// mustRegister panics on a registration conflict among the fixed, compile-time
// set of core behaviors; a duplicate here is a programming error, not runtime
// input, so failing fast at construction is correct.
func mustRegister(r *BehaviorRegistry, spec BehaviorSpec) {
	if err := r.Register(spec); err != nil {
		panic(err)
	}
}

// behaviorAgentDelegate assembles the provider-bound prompt for a delegate
// node. It only prepares the prompt fragment; the executor remains
// responsible for the actual provider call (Task-176 is additive-only and
// does not move the provider dispatch path).
func behaviorAgentDelegate(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	if in.Prompt == "" {
		return BehaviorOutput{}, fmt.Errorf("agent.delegate: missing prompt for node %q", in.NodeID)
	}
	return BehaviorOutput{
		Status:              "continue",
		NextPromptFragments: []string{in.Prompt},
	}, nil
}

// behaviorHubInline runs the deterministic synthesis decision a hub node
// makes after its join completes. When RawArgs carries a flow_control-shaped
// payload it is parsed through the same contract as behaviorFlowControl;
// otherwise the hub defaults to continuing the loop.
func behaviorHubInline(ctx context.Context, in BehaviorInput) (BehaviorOutput, error) {
	if in.RawArgs != nil {
		if _, hasStatus := in.RawArgs["status"]; hasStatus {
			return behaviorFlowControl(ctx, in)
		}
	}
	return BehaviorOutput{Status: "continue"}, nil
}

// behaviorContextProduce wraps BuildFlowContextPackage so a context-producing
// node can be selected by behavior ID instead of a hardcoded "Plan step"
// check. The built package is returned in Payload["package"] for a
// context.render node to consume.
func behaviorContextProduce(ctx context.Context, in BehaviorInput) (BehaviorOutput, error) {
	hints := FlowContextHints{
		WorkflowRunID: in.WorkflowRunID,
		PlanStepRunID: in.StepRunID,
		UserPrompt:    in.Prompt,
		MCPDriverRef:  in.MCPDriverRef,
		JiraIssueRef:     in.JiraIssueRef,
		JiraSprintRef:    in.JiraSprintRef,
		FirebaseCrashRef: in.FirebaseCrashRef,
	}
	if in.Payload != nil {
		if sourceDocID, ok := in.Payload["sourceDocId"].(string); ok {
			hints.SourceDocID = sourceDocID
		}
	}
	pkg, err := BuildFlowContextPackageWithSources(ctx, in.WorkspaceCwd, hints, in.ContextSourceIDs)
	if err != nil {
		return BehaviorOutput{}, fmt.Errorf("context.produce: %w", err)
	}
	summary := "flow context package produced"
	if len(pkg.Warnings) > 0 {
		summary = pkg.Warnings[0]
	}
	return BehaviorOutput{
		Status:  "done",
		Summary: summary,
		Payload: map[string]any{"package": pkg},
	}, nil
}

// behaviorContextRender wraps ComposeFlowCodingPrompt so a delegate node can
// consume a previously produced context artifact through a declared input
// binding instead of an implicit "Coding step" assumption.
func behaviorContextRender(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	pkg, ok := in.Payload["package"].(FlowContextPackage)
	if !ok {
		return BehaviorOutput{}, fmt.Errorf("context.render: node %q missing bound context package", in.NodeID)
	}
	rendered := ComposeFlowCodingPrompt(pkg, in.Prompt)
	return BehaviorOutput{
		Status:              "done",
		NextPromptFragments: []string{rendered},
	}, nil
}

// toIntArg coerces a RawArgs numeric value to int. BehaviorInput.RawArgs is
// often populated by decoding JSON into map[string]any, where every number
// (including a plain exit code) decodes as float64, not int — a bare
// `v.(int)` type assertion silently fails and falls back to the zero value
// for a real, nonzero exit code (BUG-NOTE-CP42 #27: {"exitCode":1} over JSON
// read back as 0 and reported "command validation passed"). Handles the
// direct-Go-caller int shapes too, so callers that already have a real int
// keep working unchanged.
func toIntArg(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// behaviorCommandValidate maps a generic command-result payload onto the
// flow_control contract. Callers supply exitCode; any other field is passed
// through in Payload for downstream behaviors.
func behaviorCommandValidate(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	exitCode, _ := toIntArg(in.RawArgs["exitCode"])
	if exitCode == 0 {
		return BehaviorOutput{Status: "done", Summary: "command validation passed"}, nil
	}
	return BehaviorOutput{
		Status:  "continue",
		Summary: fmt.Sprintf("command validation failed with exit code %d", exitCode),
		Payload: in.RawArgs,
	}, nil
}

// issuesLen counts an "issues" RawArgs value regardless of its concrete
// slice type. BUG-NOTE-CP42 #30: a bare `v.([]any)` type assertion only
// matches JSON-decoded issue lists; an internal Go caller passing a []string
// or a typed issue slice (e.g. []ReviewIssue) would silently read as empty
// and behaviorValidationSummarize would wrongly report "no open issues".
func issuesLen(v any) int {
	switch t := v.(type) {
	case []any:
		return len(t)
	case []string:
		return len(t)
	case []ReviewIssue:
		return len(t)
	}
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		return rv.Len()
	}
	return 0
}

// behaviorValidationSummarize reduces a list of open issues into a single
// flow_control-shaped decision: no issues closes the node, any issue keeps
// the loop open.
func behaviorValidationSummarize(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	issues := in.RawArgs["issues"]
	count := issuesLen(issues)
	if count == 0 {
		return BehaviorOutput{Status: "done", Summary: "no open issues"}, nil
	}
	return BehaviorOutput{
		Status:  "continue",
		Summary: fmt.Sprintf("%d open issue(s) require another pass", count),
		Payload: map[string]any{"issues": issues},
	}, nil
}

// behaviorArtifactAuditDraft prepares a draft audit artifact. It never writes
// the artifact itself; the draft must pass through a user.confirm node before
// any audit write happens (CP-42 constraint: AI/pack data is advisory, the
// audit gate stays Go-enforced).
func behaviorArtifactAuditDraft(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	summary, _ := in.RawArgs["summary"].(string)
	if summary == "" {
		return BehaviorOutput{}, fmt.Errorf("artifact.audit_draft: node %q missing summary", in.NodeID)
	}
	return BehaviorOutput{
		Status:  "continue",
		Summary: "audit draft ready for confirmation",
		Payload: map[string]any{"draftSummary": summary},
	}, nil
}

// behaviorTelegramNotify is the registry stub for the telegram.notify inline
// behavior. The actual, side-effecting send runs in the executor dispatch
// (runTelegramNotifyNode, flow_validate_audit_dispatch.go) — exactly like
// command.validate/artifact.audit_draft, whose real work also lives in the
// dispatch layer rather than the registry handler. This entry exists so the
// inline-scope guard in tryAdvanceFlowFromNode (DefaultBehaviorRegistry().
// Resolve → BehaviorScopeInline) resolves; it is never the send path.
func behaviorTelegramNotify(_ context.Context, _ BehaviorInput) (BehaviorOutput, error) {
	return BehaviorOutput{Status: "done"}, nil
}

// behaviorHubNotify (Task-235) is the registry stub for hub.notify, mirroring
// behaviorTelegramNotify's shape: the real work (reinvoking the hub session
// with a write-contract prompt, via dispatchHubNotifyNode) runs in the
// executor dispatch layer (flow_validate_audit_dispatch.go / interactive_
// service.go), not this handler. This entry exists only so the inline-scope
// guard in tryAdvanceFlowFromNode (DefaultBehaviorRegistry().Resolve →
// BehaviorScopeInline) resolves for a hub.notify node.
func behaviorHubNotify(_ context.Context, _ BehaviorInput) (BehaviorOutput, error) {
	return BehaviorOutput{Status: "done"}, nil
}

// behaviorFlowControl wraps parseFlowControlInput so a declared tool face's
// generic flow_control call can be dispatched by behavior ID.
func behaviorFlowControl(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	parsed, err := parseFlowControlInput(in.RawArgs)
	if err != nil {
		return BehaviorOutput{}, err
	}
	return BehaviorOutput{
		Status:  parsed.Status,
		Summary: parsed.Summary,
		Payload: parsed.Payload,
	}, nil
}

// behaviorUserConfirm gates a prior draft behind explicit user confirmation.
// It never assumes confirmation; absent an explicit confirmed=true it keeps
// the node open.
func behaviorUserConfirm(_ context.Context, in BehaviorInput) (BehaviorOutput, error) {
	confirmed, _ := in.RawArgs["confirmed"].(bool)
	if !confirmed {
		return BehaviorOutput{Status: "continue", Summary: "awaiting user confirmation"}, nil
	}
	return BehaviorOutput{Status: "done", Summary: "user confirmed"}, nil
}
