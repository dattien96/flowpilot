// BUG-354 C2 (run-540927): the model's ask_user MCP call outlived its client —
// opencode's MCP client timed out (~60s) while the runner-side question TTL is
// 10 minutes, so the card stayed answerable after the model already escalated
// and ended its turn; the late human answer stamped a ghost RUNNING on a dead
// turn. Contract after the fix: when the MCP HTTP request dies, the pending
// question is EXPIRED (card gone, late AnswerQuestion → 409 question_expired)
// and the tool call returns an error result so the provider never hangs.
//
// Cross-provider (R2): all three providers route model ask_user through this
// ONE shared MCP server (claude: --mcp-config http; grok: reused
// claudeMCPServer, Task-209; opencode: session/load mcpServers http — live
// run-540927 log). Fixing the shared server fixes all three by construction.
package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func new540927AskUserFixture(t *testing.T) (*InteractiveService, *turnBridge) {
	t.Helper()
	svc, _ := newTestServer(t)
	svc.questionTTL = 30 * time.Second // must not fire before the disconnect does

	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	spawned, spawnErr := svc.spawnChildRun(context.Background(), parentH.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	svc.mu.Lock()
	child := svc.runs[spawned.RunID]
	svc.mu.Unlock()
	if child == nil {
		t.Fatal("child run missing")
	}
	return svc, &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-t"}
}

// Repro: the MCP HTTP client disconnects while the question is pending →
// dispatch returns an error result promptly, the question expires (card gone,
// pendingQuestionID cleared), and a late AnswerQuestion gets 409
// question_expired instead of stamping ghost RUNNING.
func Test540927AskUserMCPDisconnectExpiresQuestion(t *testing.T) {
	svc, bridge := new540927AskUserFixture(t)
	mcp := newClaudeMCPServer()
	tok := mcp.register(bridge, false)
	defer mcp.unregister(tok)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	resultCh := make(chan any, 1)
	go func() {
		result, _ := mcp.dispatchCtx(reqCtx, "tools/call", map[string]any{
			"params": map[string]any{
				"name":      "ask_user",
				"arguments": map[string]any{"prompt": "Fix contract to 6?", "options": []any{"Fix to 6", "Keep 18"}},
			},
		}, tok)
		resultCh <- result
	}()

	var qid string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for id, rec := range svc.questions {
			if rec.status == "pending" {
				qid = id
				return true
			}
		}
		return false
	}, "pending question")

	start := time.Now()
	cancelReq()
	var result map[string]any
	select {
	case raw := <-resultCh:
		result = raw.(map[string]any)
	case <-time.After(3 * time.Second):
		t.Fatal("dispatchCtx hung after client disconnect (BUG-354 C2)")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("disconnect handling took %s, want prompt", elapsed)
	}
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	if txt, _ := block["text"].(string); !strings.Contains(txt, "ask_user aborted") {
		t.Fatalf("tool result = %q, want ask_user aborted notice", txt)
	}

	svc.mu.Lock()
	rec := svc.questions[qid]
	pending := bridge.rs.pendingQuestionID
	svc.mu.Unlock()
	if rec == nil || rec.status != "expired" {
		t.Fatalf("question %s not expired after disconnect: %+v", qid, rec)
	}
	if pending != "" {
		t.Fatalf("pendingQuestionID not cleared: %q", pending)
	}

	// Late human answer after the model gave up → 409 question_expired, no
	// ghost RUNNING (assert the exact code, review F4).
	if apiE := svc.AnswerQuestion(qid, []string{"Fix to 6"}); apiE == nil {
		t.Fatal("late AnswerQuestion must be rejected after MCP disconnect")
	} else if apiE.code != "question_expired" {
		t.Fatalf("late AnswerQuestion err = %v, want code=question_expired", apiE)
	}
}

// Near-miss: a question answered BEFORE the client disconnects still resolves
// normally through the ctx-bound path — the bound must not break the happy
// path.
func Test540927AskUserAnsweredBeforeDisconnectStillResolves(t *testing.T) {
	svc, bridge := new540927AskUserFixture(t)
	mcp := newClaudeMCPServer()
	tok := mcp.register(bridge, false)
	defer mcp.unregister(tok)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	resultCh := make(chan any, 1)
	go func() {
		result, _ := mcp.dispatchCtx(reqCtx, "tools/call", map[string]any{
			"params": map[string]any{
				"name":      "ask_user",
				"arguments": map[string]any{"prompt": "Pick", "options": []any{"A", "B"}},
			},
		}, tok)
		resultCh <- result
	}()

	var qid string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for id, rec := range svc.questions {
			if rec.status == "pending" {
				qid = id
				return true
			}
		}
		return false
	}, "pending question")

	if apiE := svc.AnswerQuestion(qid, []string{"B"}); apiE != nil {
		t.Fatalf("AnswerQuestion: %v", apiE)
	}
	select {
	case raw := <-resultCh:
		result := raw.(map[string]any)
		content, _ := result["content"].([]any)
		block, _ := content[0].(map[string]any)
		if txt, _ := block["text"].(string); txt != "B" {
			t.Fatalf("resolved answer = %q, want B", txt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("answered ask_user never returned to the MCP caller")
	}
}

// Compat (R1): a bridge WITHOUT the AskQuestionCtx capability (the pre-existing
// TurnBridge fakes) falls back to legacy AskQuestion through the same ctx-bound
// dispatch — compile+behavior parity.
func Test540927AskUserLegacyBridgeFallbackStillWorks(t *testing.T) {
	b := &fakeClaudeBridge{answer: []string{"A"}}
	mcp := newClaudeMCPServer()
	tok := mcp.register(b, false)
	defer mcp.unregister(tok)

	rawResult, rpcErr := mcp.dispatchCtx(context.Background(), "tools/call", map[string]any{
		"params": map[string]any{
			"name":      "ask_user",
			"arguments": map[string]any{"prompt": "Pick", "options": []any{"A", "B"}},
		},
	}, tok)
	if rpcErr != nil {
		t.Fatalf("legacy fallback rpcErr: %+v", rpcErr)
	}
	result := rawResult.(map[string]any)
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	if txt, _ := block["text"].(string); txt != "A" {
		t.Fatalf("legacy fallback result = %q, want A", txt)
	}
}
