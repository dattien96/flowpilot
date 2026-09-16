package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/knowledge"
	"flowpilot-runner/internal/structure"
)

// stubKnowledgeLister is the P-1/P-3 double: two flows, no subprocess.
type stubKnowledgeLister struct{}

func (stubKnowledgeLister) ListProcesses(context.Context) ([]structure.FlowSummary, error) {
	return []structure.FlowSummary{
		{ID: "Checkout_Pipeline", Label: "cart to order", ProcessType: "cross_community", StepCount: 3,
			Symbols: []structure.FlowSymbol{
				{ID: "Method:shop/cart/service.go:Checkout", Kind: "Method", Path: "shop/cart/service.go", Name: "Checkout"},
				{ID: "Function:shop/pay/charge.go:Charge", Kind: "Function", Path: "shop/pay/charge.go", Name: "Charge"},
			}},
		{ID: "Login_Flow", Label: "session issue", ProcessType: "local", StepCount: 2,
			Symbols: []structure.FlowSymbol{
				{ID: "Function:shop/auth/login.go:Login", Kind: "Function", Path: "shop/auth/login.go", Name: "Login"},
			}},
	}, nil
}

func (stubKnowledgeLister) ListModelCandidates(context.Context) ([]structure.ModelInfo, error) {
	return nil, nil
}

// TestEnsureKnowledgeBaseRunsOnceInBackground pins Task-373 AC-5: two binds
// of the same workspace distill exactly once, and the scan never blocks the
// caller (the counting closure signals start, then blocks until released —
// the call must already have returned with zero completions).
func TestEnsureKnowledgeBaseRunsOnceInBackground(t *testing.T) {
	ws := t.TempDir()
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	distill := func(context.Context) error {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return nil
	}
	ensureKnowledgeBaseAsync(ws, distill)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("distill did not start in background")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls after first bind = %d, want 1", got)
	}
	// Second bind while the first scan is still running: no second scan.
	ensureKnowledgeBaseAsync(ws, distill)
	select {
	case <-started:
		t.Fatal("second bind started a duplicate distill scan")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("total distill calls = %d, want exactly 1", got)
	}
}

// TestEnsureKnowledgeBaseSkipsBootstrapped pins the fast path: an existing
// index.json means no scan at all (steady-state binds stay free).
func TestEnsureKnowledgeBaseSkipsBootstrapped(t *testing.T) {
	ws := t.TempDir()
	dir := knowledge.KnowledgeDir(ws)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"schemaVersion":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	ensureKnowledgeBaseAsync(ws, func(context.Context) error {
		calls.Add(1)
		return nil
	})
	time.Sleep(200 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("bootstrapped workspace distilled %d times, want 0", got)
	}
}

// TestUpdateAsyncSerializesPerWorkspace pins the audit-hook safety property:
// concurrent updates never interleave a merge (max observed redistill
// concurrency is 1) and both complete.
func TestUpdateAsyncSerializesPerWorkspace(t *testing.T) {
	ws := uniqueKnowledgeWorkspace(t)
	var concurrent, maxSeen atomic.Int32
	redistill := func(context.Context) (*knowledge.KnowledgeBase, error) {
		cur := concurrent.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		concurrent.Add(-1)
		kb, err := knowledge.Distill(context.Background(), ws, stubKnowledgeLister{}, nil)
		if err != nil {
			return nil, err
		}
		return kb, nil
	}
	done := make(chan struct{}, 2)
	go func() { UpdateAsync(ws, []string{"shop/cart/service.go"}, redistill); done <- struct{}{} }()
	go func() { UpdateAsync(ws, []string{"shop/pay/charge.go"}, redistill); done <- struct{}{} }()
	timeout := time.After(15 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("concurrent updates did not finish")
		}
	}
	if got := maxSeen.Load(); got != 1 {
		t.Fatalf("max redistill concurrency = %d, want 1", got)
	}
}

// TestUpdateAsyncSkipsMissingBase pins the P-3 guard: no knowledge dir, no
// scan, no error — a project that never bootstrapped stays untouched.
func TestUpdateAsyncSkipsMissingBase(t *testing.T) {
	var calls atomic.Int32
	UpdateAsync(t.TempDir(), []string{"shop/cart/service.go"}, func(context.Context) (*knowledge.KnowledgeBase, error) {
		calls.Add(1)
		return nil, nil
	})
	if got := calls.Load(); got != 0 {
		t.Fatalf("redistill called %d times on missing base, want 0", got)
	}
}

// uniqueKnowledgeWorkspace seeds a temp workspace with a minimal knowledge
// base under a process-unique leaf (the update locks are process-global).
func uniqueKnowledgeWorkspace(t *testing.T) string {
	t.Helper()
	ws, err := os.MkdirTemp("", "kbws-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(ws) })
	kb, err := knowledge.Distill(context.Background(), ws, stubKnowledgeLister{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := knowledge.WriteFull(ws, kb); err != nil {
		t.Fatal(err)
	}
	return ws
}
