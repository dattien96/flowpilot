package runner

// BUG-295: a multi-turn Claude run silently rotated its provider session mid-run.
// Root cause was a regression (b288982) that broadened a Codex-only override in
// runTurn to ALL providers: from turn 2 on it fed Claude's adapter the REAL session
// id, but claudeAdapter.SendTurn treats req.ProviderSessionID as the SYNTHETIC pool
// KEY. The pool lookup missed, --resume was dropped, and Claude started a fresh
// session — so restart replay (which loads the single persisted session file) lost
// every turn after the first.
//
// Two additive, complementary fixes are pinned here:
//
//   F-2 (interactive_service.go runTurn): exclude Claude from the real-id promotion
//   so live turns keep passing the synthetic pool key (Codex/Grok still get the real
//   id, preserving the b288982 Grok cross-account fix). Verified across all three
//   providers below.
//
//   F-1 (claude_adapter.go SendTurn): when the pool lookup misses AND the handle is
//   already a real id (not "thread-*"), resume it directly. This covers the
//   after-restart case, where reconstruct seeds providerSessionID from the persisted
//   REAL id (interactive_resume.go:833-834) into a fresh, empty pool.
//
// additive-tests-only: the pre-existing TestClaudeAdapterResumeUsesRealSessionID
// (which reuses the SYNTHETIC id for both turns) is left untouched; these are new.

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// --- F-2: runTurn promotes the real id for Codex/Grok but NOT for Claude ---------

// bug295RunTurnCapturedSessionID drives runTurn with a capturing adapter for the
// given providerKey and returns the ProviderSessionID the adapter actually received.
// The run carries a synthetic providerSessionID plus a promoted real id, exactly the
// state a follow-up turn has after turn 1 captured a real session.
func bug295RunTurnCapturedSessionID(t *testing.T, providerKey ProviderKey, synthetic, real string) string {
	t.Helper()
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	rs := &interactiveRun{
		id:                    "run-295",
		providerKey:           providerKey,
		providerSessionID:     synthetic,
		realProviderSessionID: real,
		workspaceCwd:          t.TempDir(),
		runKind:               "chat",
		turnCount:             2,
	}
	svc.runTurn(context.Background(), rs, capture, TurnInput{StepID: "step-1", Prompt: "follow up"}, "", "turn-2", nil)

	select {
	case req := <-capture.ch:
		return req.ProviderSessionID
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for captured turn request (%s)", providerKey)
		return ""
	}
}

func TestBug295RunTurnClaudeKeepsSyntheticSessionID(t *testing.T) {
	const (
		synthetic = "thread-12261"
		real      = "af68de79-398c-4653-85a1-75c9fd60ac51"
	)
	got := bug295RunTurnCapturedSessionID(t, ProviderKeyClaude, synthetic, real)
	// Regression guard: feeding the adapter the REAL id is exactly the type confusion
	// that dropped --resume. Claude must receive the SYNTHETIC pool key.
	if got != synthetic {
		t.Fatalf("Claude follow-up turn: req.ProviderSessionID = %q, want synthetic %q (BUG-295 F-2)", got, synthetic)
	}
}

func TestBug295RunTurnCodexPromotesRealSessionID(t *testing.T) {
	const (
		synthetic = "thread-500"
		real      = "codex-rollout-0001"
	)
	got := bug295RunTurnCapturedSessionID(t, ProviderKeyCodex, synthetic, real)
	// Non-regression: Codex resumes by the real rollout id directly. The F-2 Claude
	// exclusion must NOT touch this path.
	if got != real {
		t.Fatalf("Codex follow-up turn: req.ProviderSessionID = %q, want real %q", got, real)
	}
}

func TestBug295RunTurnGrokPromotesRealSessionID(t *testing.T) {
	const (
		synthetic = "thread-700"
		real      = "019f60f6-1111-2222-3333-444455556666"
	)
	got := bug295RunTurnCapturedSessionID(t, ProviderKeyGrok, synthetic, real)
	// Non-regression: b288982 (Grok cross-account relocate) relies on runTurn feeding
	// the real ACP id so the adapter calls session/load. F-2 must keep that intact.
	if got != real {
		t.Fatalf("Grok follow-up turn: req.ProviderSessionID = %q, want real %q (b288982 must survive)", got, real)
	}
}

// --- F-1: adapter resumes a real id directly when the pool misses ----------------

// bug295ClaudeSpawnArgs runs one Claude turn with the given ProviderSessionID against
// a FRESH (empty) pool and returns the spawn args, so a test can assert whether
// --resume was emitted. The empty pool models a post-restart process.
func bug295ClaudeSpawnArgs(t *testing.T, providerSessionID string) []string {
	t.Helper()
	script := shellReadLine() + shellOutputLines(
		`{"type":"system","subtype":"init","session_id":"`+providerSessionID+`"}`,
		`{"type":"result","subtype":"success","result":"ok"}`,
	)
	var mu sync.Mutex
	var captured []string
	original := commandContextFn
	t.Cleanup(func() { commandContextFn = original })
	commandContextFn = func(ctx context.Context, _ string, arg ...string) *exec.Cmd {
		mu.Lock()
		captured = append([]string{}, arg...)
		mu.Unlock()
		return testShellCommand(ctx, script)
	}

	pool := newClaudeProcessPool() // fresh + empty: no synthetic→real mapping (post-restart)
	a := newClaudeAdapter(pool, ".", "acct", nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", ProviderSessionID: providerSessionID}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	return captured
}

func TestBug295ClaudeAdapterResumesRealIDWhenPoolMisses(t *testing.T) {
	// Post-restart: reconstruct seeds providerSessionID from the persisted REAL id
	// (interactive_resume.go:833-834) into a fresh pool, so realSession() misses.
	const real = "af68de79-398c-4653-85a1-75c9fd60ac51"
	args := bug295ClaudeSpawnArgs(t, real)
	if !flagHasValue(args, "--resume", real) {
		t.Fatalf("pool-miss on a real id must --resume it directly (BUG-295 F-1), got: %v", args)
	}
}

func TestBug295ClaudeAdapterDoesNotResumeSyntheticOnFirstTurn(t *testing.T) {
	// Guard: F-1 must NOT over-trigger. A synthetic "thread-*" id with an empty pool is
	// a genuine first turn — it must never be passed to --resume.
	args := bug295ClaudeSpawnArgs(t, "thread-9")
	if argIndex(args, "--resume") >= 0 {
		t.Fatalf("a synthetic thread-* id must never be resumed on a pool miss, got: %v", args)
	}
}
