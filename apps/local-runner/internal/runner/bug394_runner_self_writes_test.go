package runner

import (
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// BUG-394 (live runs 11/4472/2284/2737): the frozen-contract scope-drift check
// counted runner/provider-owned bookkeeping the agent never wrote as coder
// drift: the gate's own baseline capture (.flowpilot/guard/test_baseline.json),
// first-init gate config (.flowpilot/settings/gate-config.json), and the Devin
// session's MCP config (.devin/mcp_config.local.json). Exact-path exemptions
// only — CA-427 forbids a .flowpilot/**-wide pass (flow-rules.json must still
// drift).
func TestBug394_RunnerOwnedGateBookkeepingPathsExempt(t *testing.T) {
	for _, p := range []string{
		".flowpilot/guard/test_baseline.json",
		".flowpilot/settings/gate-config.json",
		".devin/mcp_config.local.json",
	} {
		if !changecontract.IsRunnerOwnedConfigPath(p) {
			t.Fatalf("%q is runner-owned bookkeeping and must be drift-exempt", p)
		}
	}
	// Neighboring files an agent could plausibly forge must still drift.
	for _, p := range []string{
		".flowpilot/settings/flow-rules.json",
		".flowpilot/contracts/frozen_contracts.ndjson",
		".flowpilot/guard/other.json",
		".devin/other.json",
		"calc/calc.go",
	} {
		if changecontract.IsRunnerOwnedConfigPath(p) {
			t.Fatalf("%q must NOT be exempt (CA-427 narrow-exemption rule)", p)
		}
	}
}
