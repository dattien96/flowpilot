package runner

import (
	"strings"
	"testing"
)

// TestPreflightContractPlanTurnMarkerAnchoredAgainstPrefixCollision is the
// CP-55 P-8 Claude-agent review Important Finding 2 regression test: the
// original marker ("role: contract-planner", an unanchored substring) would
// also match a custom agent whose own role merely STARTS WITH that text
// (e.g. a user clones the built-in flow and names a custom agent role
// "contract-planner-v2") — composeAgentIdentityLine's own " | "-joined parts
// mean the real contract-planner's identity line always has a pipe
// immediately after "contract-planner", which a same-prefixed-but-different
// role never does. Anchoring the marker on both surrounding pipes closes it.
func TestPreflightContractPlanTurnMarkerAnchoredAgainstPrefixCollision(t *testing.T) {
	similar := composeAgentIdentityLine(&AgentDefinition{Name: "custom", Role: "contract-planner-v2", Path: "agents/custom.md"})
	if strings.Contains(similar, preflightContractPlanTurnMarker) {
		t.Fatalf("marker matched a custom role %q that merely starts with contract-planner — identity line: %q", "contract-planner-v2", similar)
	}

	exact := composeAgentIdentityLine(&AgentDefinition{Name: "contract-planner", Role: "contract-planner", Path: "flow-pack/agents/contract-planner.md"})
	if !strings.Contains(exact, preflightContractPlanTurnMarker) {
		t.Fatalf("marker must still match the real contract-planner identity line: %q", exact)
	}
}
