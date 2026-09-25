package runner

import "testing"

// BUG-383: /admin/providers reported opencode approvalEvents:false, mcp:false
// because registration evaluated Capabilities() on a zero-value adapter whose
// mcpServer is only wired per turn. Provider-level capability must reflect
// what the provider supports once wired — both approval events and MCP are
// live-verified on opencode (permission_required decisions applied,
// ask_user/spawn_agent round-trips).
func TestBug383_AdminProvidersOpencodeCapabilities(t *testing.T) {
	reg := ProviderRegistryFor(&Runner{})
	entry, ok := reg.Get(ProviderKeyOpencode)
	if !ok {
		t.Skip("opencode provider not registered")
	}
	if !entry.Capabilities.ApprovalEvents {
		t.Fatal("opencode registration reports approvalEvents=false; provider emits permission_required live")
	}
	if !entry.Capabilities.Mcp {
		t.Fatal("opencode registration reports mcp=false; session/new carries mcpServers and MCP tools round-trip")
	}
}

// Same zero-value-adapter defect on the devin registration (identical
// Capabilities() shape — mcpServer is wired per turn).
func TestBug383_AdminProvidersDevinCapabilities(t *testing.T) {
	reg := ProviderRegistryFor(&Runner{})
	entry, ok := reg.Get(ProviderKeyDevin)
	if !ok {
		t.Skip("devin provider not registered")
	}
	if !entry.Capabilities.ApprovalEvents {
		t.Fatal("devin registration reports approvalEvents=false; permission requests round-trip live")
	}
	if !entry.Capabilities.Mcp {
		t.Fatal("devin registration reports mcp=false; session carries mcpServers")
	}
}
