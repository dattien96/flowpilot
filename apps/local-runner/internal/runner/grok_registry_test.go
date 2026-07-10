package runner

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// Task-212 (CP-46 T-4): live registry enablement, on by default via
// grokAgentEnabled, with FLOWPILOT_GROK_AGENT=0/false/no as an explicit
// opt-out escape hatch.

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

// mockGrokInitProcessCapturingArgs is mockGrokInitProcess but also records the
// exact argv ensureGrokProcess launched `grok` with, so callers can assert
// --model/--reasoning-effort actually reached the spawned process.
func mockGrokInitProcessCapturingArgs(t *testing.T, captured *[]string) func() {
	t.Helper()
	orig := commandContextFn
	initResult, _ := json.Marshal(liveGrokInitializeResult())
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		*captured = args
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

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "")
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

	// Reuse: same scope+model+effort returns the same handle (one shared process).
	h2, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "")
	if err != nil || h2 != h {
		t.Fatalf("same-scope ensure should reuse the handle (h2==h=%v, err=%v)", h2 == h, err)
	}

	// Account switch: a different scope tears down + recreates (new handle).
	h3, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "", "")
	if err != nil {
		t.Fatalf("recreate ensure: %v", err)
	}
	defer h3.close()
	if h3 == h {
		t.Fatal("expected a new handle for a different scope")
	}

	// Model change on the SAME scope also tears down + recreates (Grok's CLI only
	// accepts --model/--reasoning-effort as `grok agent` launch flags, so a
	// turn-level model switch can only take effect via respawn).
	h4, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "grok-4.5", "")
	if err != nil {
		t.Fatalf("model-change ensure: %v", err)
	}
	defer h4.close()
	if h4 == h3 {
		t.Fatal("expected a new handle when model changes for the same scope")
	}
	if h4.model != "grok-4.5" {
		t.Fatalf("handle.model = %q, want grok-4.5", h4.model)
	}

	// Reasoning-effort change on the SAME scope+model also tears down + recreates.
	h5, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "grok-4.5", "high")
	if err != nil {
		t.Fatalf("reasoning-effort-change ensure: %v", err)
	}
	defer h5.close()
	if h5 == h4 {
		t.Fatal("expected a new handle when reasoningEffort changes for the same scope+model")
	}
}

// TestEnsureGrokProcessPassesModelAndReasoningEffortAsLaunchFlags is a
// regression test for the live-verified fix (2026-07-10): Grok's CLI only
// accepts model/reasoning-effort as `grok agent` LAUNCH flags (`grok agent
// --help`: -m/--model, --reasoning-effort are options on the `agent`
// subcommand, before `stdio`) -- there is no ACP session-level way to select a
// model. Before this fix, ensureGrokProcess spawned `grok agent stdio` with no
// model/effort flags at all, so a user's model/reasoning-effort selection in
// the UI never reached Grok, unlike Codex (thread/start's model param) and
// Claude (--model on each per-turn spawn).
func TestEnsureGrokProcessPassesModelAndReasoningEffortAsLaunchFlags(t *testing.T) {
	var args []string
	defer mockGrokInitProcessCapturingArgs(t, &args)()
	r, _ := New(".")

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "grok-4.5", "high")
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	want := []string{"agent", "--model", "grok-4.5", "--reasoning-effort", "high", "stdio"}
	if len(args) != len(want) {
		t.Fatalf("launch args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("launch args = %v, want %v", args, want)
		}
	}
}

// TestEnsureGrokProcessOmitsEmptyModelAndReasoningEffortFlags asserts the
// base case (no model/effort selected) still launches with a bare `agent
// stdio` -- no dangling empty --model/--reasoning-effort flags.
func TestEnsureGrokProcessOmitsEmptyModelAndReasoningEffortFlags(t *testing.T) {
	var args []string
	defer mockGrokInitProcessCapturingArgs(t, &args)()
	r, _ := New(".")

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "")
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	want := []string{"agent", "stdio"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Fatalf("launch args = %v, want %v", args, want)
	}
}

func TestProviderRegistryForGrokUsesPlaceholderWhenExplicitlyDisabled(t *testing.T) {
	t.Setenv(grokAgentEnvFlag, "0")
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	if _, err := reg.Adapter(ProviderKeyGrok, "", ""); err == nil {
		t.Fatal("expected grok to be unavailable when FLOWPILOT_GROK_AGENT=0")
	}
}

func TestProviderRegistryForGrokUsesLiveByDefaultWhenAccountResolvable(t *testing.T) {
	t.Setenv("XAI_API_KEY", "test-key") // account resolution fallback path
	defer mockGrokInitProcess(t)()
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	adapter, err := reg.Adapter(ProviderKeyGrok, "", "")
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

// TestProviderRegistryForGrokThreadsModelAndReasoningEffortIntoLaunch proves
// the full wiring end to end: ProviderRegistry.Adapter's model/reasoningEffort
// args reach Grok's registration (newAdapterForTurn) and, from there, the
// actual `grok agent` launch flags -- the same guarantee Codex/Claude already
// have via their own per-turn model params.
func TestProviderRegistryForGrokThreadsModelAndReasoningEffortIntoLaunch(t *testing.T) {
	t.Setenv("XAI_API_KEY", "test-key")
	var args []string
	defer mockGrokInitProcessCapturingArgs(t, &args)()
	r, _ := New(".")
	reg := ProviderRegistryFor(r)

	adapter, err := reg.Adapter(ProviderKeyGrok, "grok-4.5", "high")
	if err != nil {
		t.Fatalf("grok adapter: %v", err)
	}
	if _, ok := adapter.(*grokAdapter); !ok {
		t.Fatalf("expected live grok adapter, got %T", adapter)
	}
	want := []string{"agent", "--model", "grok-4.5", "--reasoning-effort", "high", "stdio"}
	if len(args) != len(want) {
		t.Fatalf("launch args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("launch args = %v, want %v", args, want)
		}
	}
	if r.grokProcess != nil {
		r.grokProcess.close()
	}
}

func TestProviderRegistryForGrokWithoutAccountReturnsErrorAdapter(t *testing.T) {
	t.Setenv(grokAgentEnvFlag, "1")
	// Isolate from this machine's real ~/.grok (a live-authenticated account may
	// genuinely exist on a dev box that has run the CP-46/Task-206 live probes) —
	// otherwise ResolveProviderAccount finds it for real and this test spawns an
	// actual grok process instead of exercising the no-account error path.
	isolatedHome := t.TempDir()
	t.Setenv("HOME", isolatedHome)
	t.Setenv("USERPROFILE", isolatedHome)
	t.Setenv("XAI_API_KEY", "")
	// Also isolate the persisted provider-accounts.json (normally
	// %AppData%/FlowPilot/..., unaffected by the HOME/USERPROFILE override
	// above) so a real account saved during earlier live-probe work on this
	// machine can't be loaded from disk either.
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", filepath.Join(isolatedHome, "provider-accounts.json"))
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	adapter, err := reg.Adapter(ProviderKeyGrok, "", "")
	if err != nil {
		t.Fatalf("Adapter() itself should not error for an available registration: %v", err)
	}
	if err := adapter.SendTurn(context.Background(), TurnRequest{}, nil); err == nil {
		t.Fatal("expected SendTurn to fail cleanly when no grok account/env is resolvable")
	}
}
