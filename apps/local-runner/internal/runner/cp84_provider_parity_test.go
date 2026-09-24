package runner

// CP-84 84-o gap-fill (KR-005): spec D-9 names six providers — the existing
// parity test covers claude/codex/grok. This extends the same projection
// assertion to gemini/opencode/devin so all six land identical lane shapes.
// Projection is provider-agnostic by construction (decisionPayloadsForRun
// reads rs.providerKey only as a label), this test locks that claim.

import (
	"fmt"
	"testing"
)

func TestRunUpdates_AllProvidersSameProjection(t *testing.T) {
	svc := NewInteractiveService()
	providers := []ProviderKey{
		ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok,
		ProviderKeyGemini, ProviderKeyOpencode, ProviderKeyDevin,
	}
	for i, pk := range providers {
		rs := mkDecisionRun(svc, fmt.Sprintf("run-parity-%d", i), "proj-1")
		rs.providerKey = pk
		svc.mu.Lock()
		svc.approvals["appr-parity-"+string(pk)] = pendingApprovalRec("appr-parity-"+string(pk), rs.id)
		svc.mu.Unlock()
	}

	_, _, snapshot := svc.subscribeRunUpdates()
	if len(snapshot) != len(providers) {
		t.Fatalf("expected %d provider lanes, got %d", len(providers), len(snapshot))
	}
	seen := map[string]bool{}
	for _, p := range snapshot {
		seen[p.ProviderKey] = true
		if len(p.Decisions) != 1 || p.Decisions[0].Kind != DecisionKindApproval {
			t.Fatalf("provider lane malformed: %+v", p)
		}
		if string(p.Decisions[0].ProviderKey) != p.ProviderKey {
			t.Fatalf("decision providerKey label mismatch: %+v", p.Decisions[0])
		}
	}
	for _, pk := range providers {
		if !seen[string(pk)] {
			t.Fatalf("provider %s lane missing from snapshot", pk)
		}
	}
}
