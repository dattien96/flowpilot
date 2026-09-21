package runner

import (
	"strings"
	"testing"
)

// CP-67 P-3 (Task-380) + P-2 (Task-379) runner-side contracts, additive.

func TestScaffoldBehaviorRegistration(t *testing.T) {
	spec, err := DefaultBehaviorRegistry().Resolve("agent.scaffold")
	if err != nil {
		t.Fatalf("resolve agent.scaffold: %v", err)
	}
	if spec.Scope != BehaviorScopeDelegate {
		t.Fatalf("agent.scaffold scope = %q, want delegate", spec.Scope)
	}
	delegate, err := DefaultBehaviorRegistry().Resolve("agent.delegate")
	if err != nil {
		t.Fatalf("resolve agent.delegate: %v", err)
	}
	if spec.Handler == nil || delegate.Handler == nil {
		t.Fatal("agent.scaffold/agent.delegate handler missing")
	}
	if !IsScaffoldBehavior("agent.scaffold") || !IsScaffoldBehavior("scaffold") || !IsScaffoldBehavior("scaffold_tdd") {
		t.Fatal("IsScaffoldBehavior must resolve the behavior and its aliases")
	}
	if IsScaffoldBehavior("agent.code") || IsScaffoldBehavior("agent.reproduce") {
		t.Fatal("unrelated behaviors must never resolve as scaffold")
	}
}

func TestReproduceGateRetiredAlwaysOn(t *testing.T) {
	// CP-67 B-9: the CP-64 flag is retired — every env value leaves the gate
	// on, and the legacy degrade resolvers are gone.
	for _, val := range []string{"", "0", "false", "1", "true"} {
		t.Setenv(ReproduceGateEnv, val)
		if !ReproduceGateEnabled() {
			t.Fatalf("ReproduceGateEnabled() must always be true, env=%q", val)
		}
	}
}

func TestScaffoldCoderOutcomeBufferContract(t *testing.T) {
	// Task-378 test 5 (TestCoderOutcomeChildCallIsRecordOnlyAndBuffered):
	// the child coder's batch is buffered record-only; a renegotiation maps
	// to continue (never a terminal status); the hub can read and consume.
	svc, _ := newTestServer(t)
	svc.mu.Lock()
	svc.runs["run-parent"] = &interactiveRun{id: "run-parent", stepID: "implement"}
	svc.mu.Unlock()

	reqs := []CoderBatchSignatureRequest{{
		Symbol:            "GetUser",
		File:              "service.go",
		CurrentSignature:  "func GetUser(id string) (*User, error)",
		ProposedSignature: "func GetUser(id string, forceRefresh bool) (*User, error)",
		Rationale:         "cache bypass on refresh",
	}}
	svc.bufferCoderBatchSignatures("run-parent", reqs)

	got := svc.snapshotCoderBatchSignatures("run-parent")
	if len(got) != 1 || got[0].Symbol != "GetUser" || got[0].Rationale == "" {
		t.Fatalf("hub must read the buffered batch, got %+v", got)
	}

	// The renegotiate_signatures domain status maps to continue — the child
	// call alone cannot terminate the loop.
	face := coderOutcomeFace()
	if status, ok := resolveFaceStatus(face, "renegotiate_signatures"); !ok || status != "continue" {
		t.Fatalf("renegotiate_signatures must map to continue, got %q ok=%v", status, ok)
	}
	if status, _ := resolveFaceStatus(face, "completed"); status != "done" {
		t.Fatal("completed must map to done")
	}

	// Consume clears the buffer (a round is never replayed).
	if got := svc.consumeCoderBatchSignatures("run-parent"); len(got) != 1 {
		t.Fatalf("consume must return the batch, got %+v", got)
	}
	if got := svc.snapshotCoderBatchSignatures("run-parent"); got != nil {
		t.Fatalf("buffer must be empty after consume, got %+v", got)
	}
}

func TestParseCoderBatchSignatureRequestsValidation(t *testing.T) {
	ok := map[string]any{
		"batch_signature_requests": []any{
			map[string]any{
				"symbol": "GetUser", "file": "service.go",
				"current_signature": "func GetUser(id string) (*User, error)",
				"proposed_signature": "func GetUser(id string, forceRefresh bool) (*User, error)",
				"rationale":          "cache bypass",
			},
		},
	}
	reqs, err := parseCoderBatchSignatureRequests(ok)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("valid batch must parse, got %v err=%v", reqs, err)
	}
	// A row without rationale is rejected — the hub needs reasons to mediate.
	bad := map[string]any{
		"batch_signature_requests": []any{
			map[string]any{"symbol": "GetUser", "file": "service.go",
				"current_signature": "a", "proposed_signature": "b"},
		},
	}
	if _, err := parseCoderBatchSignatureRequests(bad); err == nil || !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("missing rationale must fail with guidance, got %v", err)
	}
}
