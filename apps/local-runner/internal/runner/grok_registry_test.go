package runner

import (
	"context"
	"encoding/json"
	"os"
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

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", false)
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

	// Reuse: same scope+model+effort+approve returns the same handle (one process per key).
	h2, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", false)
	if err != nil || h2 != h {
		t.Fatalf("same-scope ensure should reuse the handle (h2==h=%v, err=%v)", h2 == h, err)
	}

	// Account switch: a different scope tears down + recreates (new handle).
	h3, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "", "", false)
	if err != nil {
		t.Fatalf("recreate ensure: %v", err)
	}
	defer h3.close()
	if h3 == h {
		t.Fatal("expected a new handle for a different scope")
	}

	// Model change on the SAME scope yields a NEW handle on its own key (Grok's CLI
	// only accepts --model/--reasoning-effort as `grok agent` launch flags). The
	// prior handle is left running (keyed processes coexist), not torn down.
	h4, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "grok-4.5", "", false)
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
	h5, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "grok-4.5", "high", false)
	if err != nil {
		t.Fatalf("reasoning-effort-change ensure: %v", err)
	}
	defer h5.close()
	if h5 == h4 {
		t.Fatal("expected a new handle when reasoningEffort changes for the same scope+model")
	}
}

// TestEnsureGrokProcessCoexistsAcrossModelsOnSameScope is the regression test for
// the "grok agent process torn down" bug: a child turn ensuring a DIFFERENT model
// on the SAME account must NOT tear down the parent turn's in-flight process.
// Before the keyed-process change, ensureGrokProcess held ONE shared handle, so the
// second ensure closed the first (dispatcher.fail("grok agent process torn down")),
// killing the parent's open session/prompt mid-turn while the child completed fine.
func TestEnsureGrokProcessCoexistsAcrossModelsOnSameScope(t *testing.T) {
	defer mockGrokInitProcess(t)()
	r, _ := New(".")
	defer r.closeAllGrokProcesses()

	parent, err := r.ensureGrokProcess(context.Background(), "acct-1", ".", nil, "grok-composer-2.5-fast", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (parent): %v", err)
	}
	child, err := r.ensureGrokProcess(context.Background(), "acct-1", ".", nil, "grok-4.5", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (child): %v", err)
	}
	if child == parent {
		t.Fatal("a different model on the same scope should get its own handle")
	}
	// The crux: the parent's process must still be alive after the child spawns.
	if parent.dispatcher.isClosed() {
		t.Fatal("parent grok process was torn down when a different-model child spawned (regression)")
	}
	if child.dispatcher.isClosed() {
		t.Fatal("child grok process should be live")
	}
	// Re-ensuring the parent's exact tuple reuses the still-live parent handle.
	again, err := r.ensureGrokProcess(context.Background(), "acct-1", ".", nil, "grok-composer-2.5-fast", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (parent re-ensure): %v", err)
	}
	if again != parent {
		t.Fatal("re-ensuring the parent tuple should reuse the live parent handle, not respawn")
	}
}

// TestEnsureGrokProcessAccountSwitchStillReclaims confirms a DIFFERENT scope still
// tears down prior-scope processes (the account-switch recreate), so keyed
// coexistence never leaks processes across accounts.
func TestEnsureGrokProcessAccountSwitchStillReclaims(t *testing.T) {
	defer mockGrokInitProcess(t)()
	r, _ := New(".")
	defer r.closeAllGrokProcesses()

	a, err := r.ensureGrokProcess(context.Background(), "acct-1", ".", nil, "grok-4.5", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (acct-1): %v", err)
	}
	if _, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", nil, "grok-4.5", "", false); err != nil {
		t.Fatalf("ensureGrokProcess (acct-2): %v", err)
	}
	if !a.dispatcher.isClosed() {
		t.Fatal("switching to a different account scope should tear down the prior scope's process")
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

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "grok-4.5", "high", false)
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

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	want := []string{"agent", "stdio"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Fatalf("launch args = %v, want %v", args, want)
	}
}

// TestEnsureGrokProcessPassesAlwaysApproveFlagWhenRequested is a regression
// test for Task-218: YOLO=true must actually reach the spawned process as
// --always-approve, not rely solely on the runner-side bridge auto-answering
// session/request_permission (which only helps if Grok happens to ask at
// all -- see Task-208 Open Question Q-1).
func TestEnsureGrokProcessPassesAlwaysApproveFlagWhenRequested(t *testing.T) {
	var args []string
	defer mockGrokInitProcessCapturingArgs(t, &args)()
	r, _ := New(".")

	h, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", true)
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	want := []string{"agent", "--always-approve", "stdio"}
	if len(args) != len(want) {
		t.Fatalf("launch args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("launch args = %v, want %v", args, want)
		}
	}
	if !h.alwaysApprove {
		t.Fatal("handle.alwaysApprove = false, want true")
	}
}

// TestEnsureGrokProcessRespawnsWhenAlwaysApproveFlips is a regression test for
// Task-218: a YOLO change on the SAME scope/model/effort must still force a
// respawn (mirroring the existing model/reasoningEffort respawn behavior),
// since a live process only picked up whatever posture it was launched with.
func TestEnsureGrokProcessRespawnsWhenAlwaysApproveFlips(t *testing.T) {
	defer mockGrokInitProcess(t)()
	r, _ := New(".")

	h1, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (false): %v", err)
	}
	defer h1.close()

	h2, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", true)
	if err != nil {
		t.Fatalf("ensureGrokProcess (true): %v", err)
	}
	defer h2.close()

	if h2 == h1 {
		t.Fatal("expected a new handle when alwaysApprove flips for the same scope/model/effort")
	}
	if !h2.alwaysApprove {
		t.Fatal("handle.alwaysApprove = false after flipping to true, want true")
	}

	// Flipping back to false is also a respawn, not a reuse of h2.
	h3, err := r.ensureGrokProcess(context.Background(), "default", ".", nil, "", "", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess (false again): %v", err)
	}
	defer h3.close()
	if h3 == h2 {
		t.Fatal("expected a new handle when alwaysApprove flips back to false")
	}
}

// TestEnsureGrokProcessAutoEnforcesGatingUnderYoloOff is the regression test for
// Task-218's auto-enforcement: spawning under YOLO=false (alwaysApprove=false)
// against an account whose config.toml bypasses the permission channel
// (permission_mode="always-approve") must rewrite it to "default" BEFORE the
// process starts — automatically, without the user ever toggling the desktop
// YOLO switch. YOLO=true must leave the file untouched.
func TestEnsureGrokProcessAutoEnforcesGatingUnderYoloOff(t *testing.T) {
	defer mockGrokInitProcess(t)()

	t.Run("yolo-off rewrites always-approve to default", func(t *testing.T) {
		grokHome := writeGrokConfigTomlFixture(t, "always-approve")
		r, _ := New(".")
		h, err := r.ensureGrokProcess(context.Background(), "acct-1", ".", map[string]string{"GROK_HOME": grokHome}, "", "", false)
		if err != nil {
			t.Fatalf("ensureGrokProcess: %v", err)
		}
		defer h.close()
		if mode, bypasses := grokConfigPermissionModeBypassesGating(grokHome); bypasses {
			t.Fatalf("config.toml still bypasses after YOLO=false spawn: mode=%q", mode)
		}
	})

	t.Run("yolo-on leaves always-approve untouched", func(t *testing.T) {
		grokHome := writeGrokConfigTomlFixture(t, "always-approve")
		r, _ := New(".")
		h, err := r.ensureGrokProcess(context.Background(), "acct-2", ".", map[string]string{"GROK_HOME": grokHome}, "", "", true)
		if err != nil {
			t.Fatalf("ensureGrokProcess: %v", err)
		}
		defer h.close()
		if _, bypasses := grokConfigPermissionModeBypassesGating(grokHome); !bypasses {
			t.Fatal("YOLO=true must not rewrite config.toml (--always-approve covers bypass), but permission_mode changed")
		}
	})

	t.Run("yolo-off leaves a non-bypassing config untouched", func(t *testing.T) {
		grokHome := writeGrokConfigTomlFixture(t, "default")
		info1, _ := os.Stat(filepath.Join(grokHome, "config.toml"))
		r, _ := New(".")
		h, err := r.ensureGrokProcess(context.Background(), "acct-3", ".", map[string]string{"GROK_HOME": grokHome}, "", "", false)
		if err != nil {
			t.Fatalf("ensureGrokProcess: %v", err)
		}
		defer h.close()
		info2, _ := os.Stat(filepath.Join(grokHome, "config.toml"))
		if info1 != nil && info2 != nil && !info1.ModTime().Equal(info2.ModTime()) {
			t.Fatal("a config that does not bypass must not be rewritten under YOLO=false")
		}
	})
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
	r.closeAllGrokProcesses()
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
	r.closeAllGrokProcesses()
}

// TestProviderRegistryForGrokMapsReasoningEffortToSupportedLaunchFlag proves
// Task-220 (GR-35): the canonical FlowPilot reasoning-effort a turn resolves is
// mapped to a Grok-supported effort id before it becomes the `grok agent
// --reasoning-effort` launch flag. grok-4.5 only accepts high/medium/low, so an
// unsupported value (xhigh/max) must degrade to the nearest supported id, and an
// unmappable value must omit the flag entirely (model default applies) rather
// than passing a value the CLI would reject.
func TestProviderRegistryForGrokMapsReasoningEffortToSupportedLaunchFlag(t *testing.T) {
	t.Setenv("XAI_API_KEY", "test-key")
	cases := []struct {
		name       string
		effortIn   string
		wantEffort string // "" means the --reasoning-effort flag must be absent
	}{
		{"high stays high", "high", "high"},
		{"xhigh degrades to high", "xhigh", "high"},
		{"max degrades to high", "max", "high"},
		{"medium stays medium", "medium", "medium"},
		{"low stays low", "low", "low"},
		{"minimal degrades to low", "minimal", "low"},
		{"none degrades to low", "none", "low"},
		{"empty omits the flag", "", ""},
		{"unknown omits the flag", "turbo", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			defer mockGrokInitProcessCapturingArgs(t, &args)()
			r, _ := New(".")
			reg := ProviderRegistryFor(r)

			adapter, err := reg.Adapter(ProviderKeyGrok, "grok-4.5", tc.effortIn)
			if err != nil {
				t.Fatalf("grok adapter: %v", err)
			}
			if _, ok := adapter.(*grokAdapter); !ok {
				t.Fatalf("expected live grok adapter, got %T", adapter)
			}
			defer func() {
				r.closeAllGrokProcesses()
			}()

			want := []string{"agent", "--model", "grok-4.5"}
			if tc.wantEffort != "" {
				want = append(want, "--reasoning-effort", tc.wantEffort)
			}
			want = append(want, "stdio")
			if len(args) != len(want) {
				t.Fatalf("launch args = %v, want %v", args, want)
			}
			for i := range want {
				if args[i] != want[i] {
					t.Fatalf("launch args = %v, want %v", args, want)
				}
			}
		})
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
