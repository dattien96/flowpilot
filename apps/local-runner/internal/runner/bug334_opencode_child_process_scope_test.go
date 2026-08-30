package runner

import (
	"context"
	"io"
	"strings"
	"testing"
)

// BUG-334 (guide section F, operator report): spawn_agent from an opencode chat
// failed with "MCP error -32000: Connection closed" within ~400ms while the
// spawned child kept running in the background. Runner log proof: the child's
// session/new (fresh per-turn MCP token) on the SHARED `opencode acp` process
// replaced the process-level MCP client (opencode keys MCP clients by server
// NAME), and the parent's in-flight tools/call died with the connection. The
// retry even landed on the CHILD's bridge ("request parent=run-<child>") and
// spawned a nested grandchild.
//
// Fix: child runs (and variants probes) isolate on their own process segment;
// the account-switch reclaim closes by BASE so isolating never tears down a
// live parent; child processes are closed on the child's terminal event.

func bug334LiveHandle(base, segment, model string) (*Runner, *opencodeProcessHandle, string) {
	r := &Runner{opencodeProcesses: map[string]*opencodeProcessHandle{}}
	scopeKey := opencodeSegmentedScope(base, segment)
	h := &opencodeProcessHandle{
		scopeKey:     scopeKey,
		scopeBase:    base,
		scopeSegment: segment,
		model:        model,
		variant:      "high",
		auto:         false,
		dispatcher:   newOpencodeDispatcher(io.Discard, nil),
	}
	r.opencodeProcesses[opencodeProcessKey(scopeKey, model, "high", false)] = h
	return r, h, scopeKey
}

func TestBug334_ChildSegmentSpawnsOwnProcessWithoutTouchingParent(t *testing.T) {
	r, parent, parentScope := bug334LiveHandle("acct-1", "", "opencode/muse-spark")

	got, err := r.ensureOpencodeProcessSegmented(context.Background(), "acct-1", "run-child-1", "/tmp",
		map[string]string{}, "opencode-go/deepseek-v4-flash", "", false)
	if err != nil {
		t.Fatalf("child ensure: %v", err)
	}
	if got == parent {
		t.Fatal("child run must NOT reuse the parent's shared process — its session/new would reset the parent's MCP client")
	}
	if got.scopeSegment != "run-child-1" || !strings.HasSuffix(got.scopeKey, "|child:run-child-1") {
		t.Fatalf("child handle scope = %q segment = %q", got.scopeKey, got.scopeSegment)
	}
	if r.opencodeProcesses[opencodeProcessKey(parentScope, parent.model, "high", false)] != parent {
		t.Fatal("parent handle must stay alive when a child segment spawns")
	}
}

func TestBug334_SameChildScopeReusesHandle(t *testing.T) {
	r, first, _ := bug334LiveHandle("acct-1", "run-child-1", "opencode/muse-spark")
	got, err := r.ensureOpencodeProcessSegmented(context.Background(), "acct-1", "run-child-1", "/tmp",
		map[string]string{}, "opencode-go/deepseek-v4-flash", "xhigh", true)
	if err != nil {
		t.Fatalf("second child-turn ensure: %v", err)
	}
	if got != first {
		t.Fatal("a child run's follow-up turn must reuse its own process (BUG-329 within the segment)")
	}
}

func TestBug334_AccountBaseSwitchStillReclaims(t *testing.T) {
	r, _, _ := bug334LiveHandle("acct-1", "", "opencode/muse-spark")
	got, err := r.ensureOpencodeProcessSegmented(context.Background(), "acct-2", "", "/tmp",
		map[string]string{}, "opencode/muse-spark", "", false)
	if err != nil {
		t.Fatalf("ensure after account switch: %v", err)
	}
	if opencodeScopeBaseOf(got) != "acct-2" {
		t.Fatalf("new handle base = %q", opencodeScopeBaseOf(got))
	}
	if len(r.opencodeProcesses) != 1 {
		t.Fatalf("account switch must reclaim the other base, got %d handles", len(r.opencodeProcesses))
	}
}

func TestBug334_CloseOpencodeProcessesForChildRunOnlyClosesThatChild(t *testing.T) {
	r := &Runner{opencodeProcesses: map[string]*opencodeProcessHandle{}}
	seed := func(base, segment, model string) *opencodeProcessHandle {
		h := &opencodeProcessHandle{
			scopeKey: opencodeSegmentedScope(base, segment), scopeBase: base, scopeSegment: segment,
			model: model, variant: "high", auto: false,
			dispatcher: newOpencodeDispatcher(io.Discard, nil),
		}
		r.opencodeProcesses[opencodeProcessKey(h.scopeKey, model, "high", false)] = h
		return h
	}
	parent := seed("acct-1", "", "opencode/muse-spark")
	seed("acct-1", "run-child-a", "opencode/muse-spark")
	seed("acct-1", "run-child-b", "opencode/muse-spark")
	if len(r.opencodeProcesses) != 3 {
		t.Fatalf("setup: expected 3 handles, got %d", len(r.opencodeProcesses))
	}

	r.CloseOpencodeProcessesForChildRun("run-child-a")
	if len(r.opencodeProcesses) != 2 {
		t.Fatalf("child-a cleanup must remove exactly its handle, got %d", len(r.opencodeProcesses))
	}
	if r.opencodeProcesses[opencodeProcessKey(opencodeSegmentedScope("acct-1", ""), parent.model, "high", false)] != parent {
		t.Fatal("chat-scope parent handle must never be touched by child cleanup")
	}
	if r.opencodeProcesses[opencodeProcessKey(opencodeSegmentedScope("acct-1", "run-child-b"), "opencode/muse-spark", "high", false)] == nil {
		t.Fatal("child-b handle must survive child-a cleanup")
	}
	r.CloseOpencodeProcessesForChildRun("")
	r.CloseOpencodeProcessesForChildRun("run-child-b")
	if len(r.opencodeProcesses) != 1 {
		t.Fatalf("empty id must no-op and child-b cleanup must remove its handle, got %d", len(r.opencodeProcesses))
	}
}

func TestBug334_ChildScopeHintOnlyForSpawnedChildren(t *testing.T) {
	if got := opencodeChildScopeHint(nil); got != "" {
		t.Fatalf("nil run hint = %q", got)
	}
	if got := opencodeChildScopeHint(&interactiveRun{id: "run-1"}); got != "" {
		t.Fatalf("chat run (no parent) hint = %q, want empty shared scope", got)
	}
	if got := opencodeChildScopeHint(&interactiveRun{id: "run-child-9", parentRunID: "run-parent"}); got != "run-child-9" {
		t.Fatalf("child run hint = %q, want its own run id", got)
	}
}

func TestBug334_RegistryThreadsChildScopeHint(t *testing.T) {
	var seen string
	reg := &ProviderRegistry{regs: map[ProviderKey]ProviderRegistration{}}
	reg.regs["prov"] = ProviderRegistration{
		Key:    "prov",
		Status: ProviderStatusAvailable,
		newAdapterForTurn: func(model, effort, childScope string) ProviderRuntimeAdapter {
			seen = childScope
			return fakeRuntimeAdapter{}
		},
	}
	if _, err := reg.AdapterWithScope("prov", "m", "", "run-child-7"); err != nil {
		t.Fatalf("AdapterWithScope: %v", err)
	}
	if seen != "run-child-7" {
		t.Fatalf("factory childScope = %q, want run-child-7", seen)
	}
	if _, err := reg.Adapter("prov", "m", ""); err != nil {
		t.Fatalf("legacy Adapter: %v", err)
	}
	if seen != "" {
		t.Fatalf("legacy Adapter must pass empty hint, got %q", seen)
	}
}

// fakeRuntimeAdapter is the minimal ProviderRuntimeAdapter for registry wiring tests.
type fakeRuntimeAdapter struct{}

func (fakeRuntimeAdapter) Key() ProviderKey                                        { return "prov" }
func (fakeRuntimeAdapter) Capabilities() ProviderCapabilities                      { return ProviderCapabilities{} }
func (fakeRuntimeAdapter) SendTurn(context.Context, TurnRequest, TurnBridge) error { return nil }

func TestBug334_ProbeSegmentIsolatedFromChatScope(t *testing.T) {
	r, parent, parentScope := bug334LiveHandle("acct-1", "", "opencode/muse-spark")
	got, err := r.ensureOpencodeProcessSegmented(context.Background(), "acct-1", "probe", "/tmp",
		map[string]string{}, "opencode-go/hy3", "", false)
	if err != nil {
		t.Fatalf("probe ensure: %v", err)
	}
	if got == parent {
		t.Fatal("variants probe must not run on the shared chat process (empty-mcpServers session/new resets it)")
	}
	if r.opencodeProcesses[opencodeProcessKey(parentScope, parent.model, "high", false)] != parent {
		t.Fatal("probe spawn must leave the parent process alive")
	}
}
