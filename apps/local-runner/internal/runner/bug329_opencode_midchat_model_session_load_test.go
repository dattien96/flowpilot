package runner

import (
	"context"
	"io"
	"strings"
	"testing"
)

// BUG-329 (run-307050): mid-chat model switch on Opencode killed the turn with
// "opencode session/load returned no sessionId". Live probe (1.18.18): sessions
// live inside the creating `opencode acp` process instance — a fresh process
// answers session/load RPC-OK with a config-only result and NO sessionId, while
// session/prompt on the loaded id still works (sessions persist in the shared
// opencode.db). ensureOpencodeProcess keyed by scope+model+variant+auto spawned
// a second process on every model change, orphaning the live session.
//
// Fix under test:
//  1. ensureOpencodeProcess reuses any live same-scope handle on exact-key miss
//     (opencodeProcessKey itself is unchanged — old tests keep passing).
//  2. ensureSession adopts the requested ses_* id when session/load answers OK
//     without a sessionId (never session/new — that loses history); real RPC
//     errors still fail.
//  3. turnResumeProviderSessionID falls back to lastOpencodeTurnSessionID like
//     the Grok twin (run-92955 analog).
//
// Additive: no pre-existing test is edited. Claude/Codex/Grok paths untouched.

// bug329LiveHandle returns a Runner with one live fake handle bound to scopeKey.
func bug329LiveHandle(scopeKey, model string) (*Runner, *opencodeProcessHandle, string) {
	r := &Runner{opencodeProcesses: map[string]*opencodeProcessHandle{}}
	h := &opencodeProcessHandle{
		scopeKey:   scopeKey,
		model:      model,
		variant:    "high",
		auto:       false,
		dispatcher: newOpencodeDispatcher(io.Discard, nil),
	}
	key := opencodeProcessKey(scopeKey, model, "high", false)
	r.opencodeProcesses[key] = h
	return r, h, key
}

func TestBug329EnsureProcessReusesSameScopeOnModelChange(t *testing.T) {
	r, h1, _ := bug329LiveHandle("acct-1", "opencode/muse-spark-1.2-contributor-free")
	got, err := r.ensureOpencodeProcess(context.Background(), "acct-1", "/tmp", map[string]string{},
		"opencode-go/deepseek-v4-flash", "medium", false)
	if err != nil {
		t.Fatalf("ensure after model change: %v", err)
	}
	if got != h1 {
		t.Fatal("model change must reuse the live same-scope process, not spawn a second one")
	}
	if len(r.opencodeProcesses) != 1 {
		t.Fatalf("map must not grow on reuse, got %d entries", len(r.opencodeProcesses))
	}
}

func TestBug329EnsureProcessReusesOnEffortAndYoloChange(t *testing.T) {
	r, h1, _ := bug329LiveHandle("acct-1", "opencode/muse-spark-1.2-contributor-free")
	for _, tc := range []struct {
		variant string
		auto    bool
	}{{"low", false}, {"high", true}, {"minimal", true}} {
		got, err := r.ensureOpencodeProcess(context.Background(), "acct-1", "/tmp", map[string]string{},
			"opencode/muse-spark-1.2-contributor-free", tc.variant, tc.auto)
		if err != nil {
			t.Fatalf("ensure variant=%s auto=%v: %v", tc.variant, tc.auto, err)
		}
		if got != h1 {
			t.Fatalf("variant/auto change (variant=%s auto=%v) must reuse the same process", tc.variant, tc.auto)
		}
	}
	if len(r.opencodeProcesses) != 1 {
		t.Fatalf("map must not grow on variant/auto reuse, got %d", len(r.opencodeProcesses))
	}
}

func TestBug329EnsureProcessStillReusesExactKeyFirst(t *testing.T) {
	r, h1, key := bug329LiveHandle("acct-1", "opencode/muse-spark-1.2-contributor-free")
	h2 := &opencodeProcessHandle{
		scopeKey:   "acct-1",
		model:      "opencode-go/deepseek-v4-flash",
		dispatcher: newOpencodeDispatcher(io.Discard, nil),
	}
	r.opencodeProcesses[opencodeProcessKey("acct-1", "opencode-go/deepseek-v4-flash", "high", false)] = h2
	got, err := r.ensureOpencodeProcess(context.Background(), "acct-1", "/tmp", map[string]string{},
		"opencode-go/deepseek-v4-flash", "high", false)
	if err != nil {
		t.Fatalf("ensure exact key: %v", err)
	}
	if got != h2 {
		t.Fatal("exact-key hit must win over same-scope scan")
	}
	if r.opencodeProcesses[key] != h1 {
		t.Fatal("existing exact-key entry must stay untouched")
	}
}

func TestBug329EnsureProcessClosesOtherScopeWithoutSpawn(t *testing.T) {
	r, hOther, _ := bug329LiveHandle("acct-2", "opencode/muse-spark-1.2-contributor-free")
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "0") // no real spawn in unit test
	_, err := r.ensureOpencodeProcess(context.Background(), "acct-1", "/tmp", map[string]string{},
		"opencode/muse-spark-1.2-contributor-free", "high", false)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected no-spawn gate error, got %v", err)
	}
	if !hOther.dispatcher.isClosed() {
		t.Fatal("account switch must close the other scope's process")
	}
	if len(r.opencodeProcesses) != 0 {
		t.Fatalf("other scope entry must be removed, got %d", len(r.opencodeProcesses))
	}
}

// bug329ServeLoadConfigOnly replies to session/load with the live-probed
// config-only shape (no sessionId) and answers config RPCs promptly.
func bug329ServeLoadConfigOnly(fg *fakeOpencode, loadedID string) {
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/load":
			// Live 1.18.18 shape on a fresh process: {"configOptions":[...]} — no sessionId.
			fg.reply(m["id"], map[string]any{"configOptions": []any{map[string]any{"id": "model"}}})
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_shouldnevernew"})
		case "session/set_config_option", "session/set_config":
			fg.reply(m["id"], map[string]any{})
		case "session/prompt":
			go func() {
				fg.notify("session/update", map[string]any{"sessionId": loadedID, "update": map[string]any{
					"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ok"}}})
				fg.reply(m["id"], map[string]any{"stopReason": "end_turn"})
			}()
		}
	})
}

func TestBug329SendTurnAdoptsSessionIdOnConfigOnlyLoad(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	const live = "ses_bug329live"
	bug329ServeLoadConfigOnly(fg, live)
	a := newOpencodeAdapter(d, "/tmp")
	store := &recordingSessionStore{}
	a.sessionStore = store

	bridge := &fakeOpencodeBridge{}
	req := TurnRequest{RunID: "run-307050", Prompt: "hello banj laf model fi", Cwd: "/tmp", ProviderSessionID: live}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn with config-only session/load: %v", err)
	}
	completed := false
	for _, ev := range bridge.events {
		if ev.Type == EventTurnCompleted {
			completed = true
		}
	}
	if !completed {
		t.Fatalf("expected turn_completed, got %+v", bridge.events)
	}
	if len(store.records) == 0 || store.records[0].ProviderSessionID != live {
		t.Fatalf("adopted live id must be persisted, got %+v", store.records)
	}
	if a.lookupRunSession("run-307050") != live {
		t.Fatalf("run session index must hold the adopted id, got %q", a.lookupRunSession("run-307050"))
	}
}

func TestBug329EnsureSessionAdoptsIdButNeverSessionNew(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	const live = "ses_bug329abc"
	sawNew := false
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/load":
			fg.reply(m["id"], map[string]any{"configOptions": []any{}})
		case "session/new":
			sawNew = true
			fg.reply(m["id"], map[string]any{"sessionId": "ses_wrong"})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	id, err := a.ensureSession(context.Background(), TurnRequest{RunID: "run-1", ProviderSessionID: live}, "/tmp", nil)
	if err != nil {
		t.Fatalf("ensureSession: %v", err)
	}
	if id != live {
		t.Fatalf("config-only load must adopt the resumed id, got %q", id)
	}
	if sawNew {
		t.Fatal("session/load ok must never fall back to session/new (history loss)")
	}
}

func TestBug329EnsureSessionLoadRPCErrorStillFails(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	sawNew := false
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/load":
			fg.send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "error": map[string]any{"code": -32603, "message": "boom"}})
		case "session/new":
			sawNew = true
			fg.reply(m["id"], map[string]any{"sessionId": "ses_wrong"})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	if _, err := a.ensureSession(context.Background(), TurnRequest{RunID: "run-1", ProviderSessionID: "ses_broken"}, "/tmp", nil); err == nil {
		t.Fatal("a real session/load RPC error must fail the turn")
	}
	if sawNew {
		t.Fatal("load RPC error must not silently session/new")
	}
}

func TestBug329EnsureSessionMissingIdWithoutResumeStillErrors(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		if m["method"] == "session/new" {
			fg.reply(m["id"], map[string]any{"configOptions": []any{}})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	if _, err := a.ensureSession(context.Background(), TurnRequest{RunID: "run-1"}, "/tmp", nil); err == nil ||
		!strings.Contains(err.Error(), "returned no sessionId") {
		t.Fatalf("session/new without sessionId must still error, got %v", err)
	}
}

func TestBug329FirstTurnThreadIDStillSessionsNew(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	sawLoad := false
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_fresh123"})
		case "session/load":
			sawLoad = true
			fg.reply(m["id"], map[string]any{"sessionId": "ses_wrong"})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	id, err := a.ensureSession(context.Background(), TurnRequest{RunID: "run-1", ProviderSessionID: "thread-42"}, "/tmp", nil)
	if err != nil {
		t.Fatalf("ensureSession thread-*: %v", err)
	}
	if sawLoad {
		t.Fatal("thread-* must never session/load")
	}
	if id != "ses_fresh123" {
		t.Fatalf("first turn must adopt session/new id, got %q", id)
	}
}

func TestBug329TurnResumeProviderSessionIDOpencodeFallback(t *testing.T) {
	// Synthetic thread-* on the run + real id captured last turn → the real id.
	rs := &interactiveRun{
		providerKey:               ProviderKeyOpencode,
		providerSessionID:         "thread-307051",
		lastOpencodeTurnSessionID: "ses_bug329real",
	}
	if got := turnResumeProviderSessionID(rs); got != "ses_bug329real" {
		t.Fatalf("opencode fallback must return the real ses id, got %q", got)
	}
	// Real id already on the run wins.
	rs.realProviderSessionID = "ses_current"
	if got := turnResumeProviderSessionID(rs); got != "ses_current" {
		t.Fatalf("realProviderSessionID must win, got %q", got)
	}
	// No fallback available → keep the synthetic id (never fabricate).
	rs2 := &interactiveRun{providerKey: ProviderKeyOpencode, providerSessionID: "thread-307051"}
	if got := turnResumeProviderSessionID(rs2); got != "thread-307051" {
		t.Fatalf("without fallback the synthetic id is kept, got %q", got)
	}
	// Non-real fallback ids are ignored.
	rs3 := &interactiveRun{providerKey: ProviderKeyOpencode, providerSessionID: "thread-x", lastOpencodeTurnSessionID: "thread-y"}
	if got := turnResumeProviderSessionID(rs3); got != "thread-x" {
		t.Fatalf("synthetic fallback must be ignored, got %q", got)
	}
	// Grok branch unchanged (parity guard).
	rs4 := &interactiveRun{providerKey: ProviderKeyGrok, providerSessionID: "thread-g", lastGrokTurnSessionID: "01grokreal"}
	if got := turnResumeProviderSessionID(rs4); got != "01grokreal" {
		t.Fatalf("grok fallback must keep working, got %q", got)
	}
	// Codex untouched.
	rs5 := &interactiveRun{providerKey: ProviderKeyCodex, providerSessionID: "thread-c", lastCodexTurnSessionID: "ignored"}
	if got := turnResumeProviderSessionID(rs5); got != "thread-c" {
		t.Fatalf("codex must be untouched, got %q", got)
	}
}
