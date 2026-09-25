package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// BUG-377 (live cp37 run-1, devin chat run): a queued gate reprompt was never
// delivered. The post-gate dispatch path only fires when pendingFlowGateSettle
// was armed — which requires a file_changed event or a flow-driven run — and
// the tail idle-flush is a single shot: when the same-gen claim lease is held
// by a wedged/slow delivery goroutine, every later flush silently returns and
// nothing re-arms delivery. The durable intent then sits until the next user
// turn or a restart.
//
// These tests pin the contract: a queued durable reprompt must be delivered
// even when the prior claim lease is outstanding, by re-checking on a bounded
// probe and reclaiming the lease once it expires.

// TestBug377_WedgedClaimLeaseRepromptReclaimed seeds a queued reprompt intent
// plus a wedged same-gen claim (a startTurn goroutine that never returned).
// Pre-fix the flush claim fails and nothing re-arms → turns stays 0.
func TestBug377_WedgedClaimLeaseRepromptReclaimed(t *testing.T) {
	reg := newProviderRegistry()
	var turns atomic.Int32
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				turns.Add(1)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "remediated"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: status=%d code=%q msg=%q", err.status, err.code, err.msg)
	}

	old := durableIntentRearmProbe
	durableIntentRearmProbe = 20 * time.Millisecond
	t.Cleanup(func() { durableIntentRearmProbe = old })

	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.pendingGateRepromptPrompt = "add the missing gate artifact"
	rs.pendingGateRepromptStepID = "chat-" + run.RunID
	rs.pendingGateRepromptGen = 1
	rs.stepID = "chat-" + run.RunID
	rs.lastTurnStepID = "chat-" + run.RunID
	// Wedged owner: same-gen claim held well past the next probe tick. The
	// holder never comes back (models a startTurn goroutine stuck in the
	// durable persist/handshake path).
	rs.intentClaimKind = "reprompt"
	rs.intentClaimGen = 1
	rs.intentClaimUntil = time.Now().Add(150 * time.Millisecond)
	svc.mu.Unlock()

	svc.flushDurableTurnIntents(run.RunID)

	waitLoop(t, "reprompt delivered after wedged lease expires", 5*time.Second, func() bool {
		return turns.Load() > 0
	})
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if got := svc.runs[run.RunID].pendingGateRepromptPrompt; got != "" {
		t.Fatalf("durable reprompt intent must be cleared after delivery, still %q", got)
	}
}

// TestBug377_UnarmedSettleRepromptStillDispatches drives the real runTurn
// post-gate path on a chat run whose turn produced a git change but NO
// file_changed events (Devin before BUG-375 — and any provider/event loss).
// pendingFlowGateSettle is never armed, so the queued reprompt previously fell
// through to a single tail flush with no watchdog. A wedged claim lease (held
// briefly) makes the strand deterministic: pre-fix the reprompt never launches.
func TestBug377_UnarmedSettleRepromptStillDispatches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir := t.TempDir()
	if physical, err := filepath.EvalSymlinks(dir); err == nil {
		dir = physical
	}
	gitRun := func(args ...string) {
		t.Helper()
		var out []byte
		var err error
		// The service's async gitnexus indexer may hold .git/index.lock on this
		// fixture repo while the fake adapter commits — retry briefly instead
		// of failing the test on a transient lock.
		for attempt := 0; attempt < 20; attempt++ {
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
				"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
				"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
				"HOME="+dir,
			)
			out, err = cmd.CombinedOutput()
			if err == nil {
				return
			}
			if !strings.Contains(string(out), "index.lock") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun("init")
	gitRun("config", "user.email", "t@e")
	gitRun("config", "user.name", "t")
	writeFile("go.mod", "module calcapp\n\ngo 1.21\n")
	writeFile("calc.go", "package calcapp\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile("sanity_test.go", "package calcapp\n\nimport \"testing\"\n\nfunc TestSanity(t *testing.T) {\n\tif Add(1, 1) != 2 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	writeFile("change-audit/FEATURE-KEYS.md", "# Feature Keys\n\n- calc-core — arithmetic in calc.go\n")
	gitRun("add", "-A")
	gitRun("commit", "-m", "seed")
	seedHead, err := captureGitHead(dir)
	if err != nil || seedHead == "" {
		t.Fatalf("captureGitHead: %q, %v", seedHead, err)
	}
	writeBugFixBaseline(t, dir, seedHead, "go test .", []string{"TestSanity"})
	gitRun("add", ".flowpilot/guard/test_baseline.json")
	gitRun("commit", "-m", "seed baseline")

	var turns atomic.Int32
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				n := turns.Add(1)
				if n == 1 {
					// Simulate the Devin/BUG-375 shape: the workspace is mutated
					// (real git diff for the gate) but NO file_changed event is
					// emitted, so pendingFlowGateSettle is never armed.
					writeFile("calc.go", "package calcapp\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Min(a, b int) int { if a < b { return a }; return b }\n")
					gitRun("add", "calc.go")
					gitRun("commit", "-m", "[Feature][bogus][logic] add min")
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, apiE := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiE != nil {
		t.Fatalf("createRun: status=%d code=%q msg=%q", apiE.status, apiE.code, apiE.msg)
	}

	old := durableIntentRearmProbe
	durableIntentRearmProbe = 20 * time.Millisecond
	t.Cleanup(func() { durableIntentRearmProbe = old })

	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.workspaceCwd = dir
	rs.stepID = "chat-" + run.RunID
	rs.lastTurnStepID = "chat-" + run.RunID
	// Wedged same-gen claim: the gate will queue gen=1; the first delivery
	// attempt loses the claim and must re-arm until the lease expires.
	rs.intentClaimKind = "reprompt"
	rs.intentClaimGen = 1
	rs.intentClaimUntil = time.Now().Add(150 * time.Millisecond)
	svc.mu.Unlock()

	turnID, apiErr := svc.startTurn(run.RunID, TurnInput{
		StepID: "chat-" + run.RunID,
		Prompt: "add a Min helper and commit it",
	}, "", "")
	if apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	if turnID == "" {
		t.Fatal("empty turn id")
	}

	waitLoop(t, "gate reprompt turn dispatched despite wedged claim", 10*time.Second, func() bool {
		return turns.Load() >= 2
	})
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if got := svc.runs[run.RunID].pendingGateRepromptPrompt; got != "" {
		t.Fatalf("reprompt intent must clear once the remediation turn is accepted, still %q", got)
	}
}

// TestBug377_ExpiredLeaseReclaimsImmediately is the complement: once the
// outstanding lease has expired, the very next flush must reclaim and deliver
// without waiting for another turn boundary.
func TestBug377_ExpiredLeaseReclaimsImmediately(t *testing.T) {
	reg := newProviderRegistry()
	var turns atomic.Int32
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				turns.Add(1)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "remediated"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.pendingGateRepromptPrompt = "add the missing gate artifact"
	rs.pendingGateRepromptStepID = "chat-" + run.RunID
	rs.pendingGateRepromptGen = 1
	rs.stepID = "chat-" + run.RunID
	rs.lastTurnStepID = "chat-" + run.RunID
	rs.intentClaimKind = "reprompt"
	rs.intentClaimGen = 1
	rs.intentClaimUntil = time.Now().Add(-time.Second) // already expired
	svc.mu.Unlock()

	svc.flushDurableTurnIntents(run.RunID)

	waitLoop(t, "reprompt delivered on expired lease reclaim", 3*time.Second, func() bool {
		return turns.Load() > 0
	})
}
