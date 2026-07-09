package runner

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
)

// Task-212 (CP-46 T-4): live registry enablement, gated behind
// FLOWPILOT_GROK_AGENT exactly like Codex's FLOWPILOT_CODEX_APPSERVER
// (TestProviderRegistryForUsesFakeWhenFlagOff/OnLine, partd_test.go).

func mockGrokInitProcess(t *testing.T) func() {
	t.Helper()
	orig := commandContextFn
	initResult, _ := json.Marshal(liveGrokInitializeResult())
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		script := shellReadLine() +
			shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":`+string(initResult)+`}`) +
			"sleep 3\n"
		return testShellCommand(ctx, script)
	}
	return func() { commandContextFn = orig }
}

func TestEnsureGrokProcessInitializes(t *testing.T) {
	defer mockGrokInitProcess(t)()
	r, _ := New(".")

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil)
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()
	if h.adapter == nil || h.dispatcher == nil {
		t.Fatal("handle missing adapter/dispatcher")
	}
	if h.initResult["protocolVersion"] == nil {
		t.Fatalf("expected initResult to carry protocolVersion, got %+v", h.initResult)
	}

	// Reuse: same scope returns the same handle (one shared process).
	h2, err := r.ensureGrokProcess(context.Background(), "default", ".", nil)
	if err != nil || h2 != h {
		t.Fatalf("same-scope ensure should reuse the handle (h2==h=%v, err=%v)", h2 == h, err)
	}

	// Account switch: a different scope tears down + recreates (new handle).
	h3, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil)
	if err != nil {
		t.Fatalf("recreate ensure: %v", err)
	}
	defer h3.close()
	if h3 == h {
		t.Fatal("expected a new handle for a different scope")
	}
}

func TestProviderRegistryForGrokUsesPlaceholderWhenFlagOff(t *testing.T) {
	r, _ := New(".")
	reg := ProviderRegistryFor(r) // flag off by default
	if _, err := reg.Adapter(ProviderKeyGrok); err == nil {
		t.Fatal("expected grok to be unavailable when FLOWPILOT_GROK_AGENT is off")
	}
}

func TestProviderRegistryForGrokUsesLiveWhenFlagOnAndAccountResolvable(t *testing.T) {
	t.Setenv(grokAgentEnvFlag, "1")
	t.Setenv("XAI_API_KEY", "test-key") // account resolution fallback path
	defer mockGrokInitProcess(t)()
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	adapter, err := reg.Adapter(ProviderKeyGrok)
	if err != nil {
		t.Fatalf("grok adapter: %v", err)
	}
	live, ok := adapter.(*grokAdapter)
	if !ok {
		t.Fatalf("flag on should use the live grok adapter, got %T", adapter)
	}
	if live.mcpServer == nil {
		t.Fatal("expected the live registration to wire mcpServer (Task-209)")
	}
	if r.grokProcess != nil {
		r.grokProcess.close()
	}
}

func TestProviderRegistryForGrokWithoutAccountReturnsErrorAdapter(t *testing.T) {
	t.Setenv(grokAgentEnvFlag, "1")
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	adapter, err := reg.Adapter(ProviderKeyGrok)
	if err != nil {
		t.Fatalf("Adapter() itself should not error for an available registration: %v", err)
	}
	if err := adapter.SendTurn(context.Background(), TurnRequest{}, nil); err == nil {
		t.Fatal("expected SendTurn to fail cleanly when no grok account/env is resolvable")
	}
}
