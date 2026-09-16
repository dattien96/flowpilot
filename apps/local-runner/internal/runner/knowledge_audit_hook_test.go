package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/knowledge"
	"flowpilot-runner/internal/structure"
)

// TestAuditNodeUpdatesKnowledgeBaseIncrementally pins Task-375 AC-1 through
// the exact composition runAuditNode's completion paths use: filter the
// audit turn's changed files, then run the incremental worker. A changed
// file in flow X rewrites only section X (fresh content), every other
// section stays byte-identical, and index.json syncs.
func TestAuditNodeUpdatesKnowledgeBaseIncrementally(t *testing.T) {
	ws := t.TempDir()
	kb1, err := knowledge.Distill(context.Background(), ws, stubKnowledgeLister{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := knowledge.WriteFull(ws, kb1); err != nil {
		t.Fatal(err)
	}
	flowsPath := filepath.Join(knowledge.KnowledgeDir(ws), "execution-flows.md")
	before, err := os.ReadFile(flowsPath)
	if err != nil {
		t.Fatal(err)
	}
	// The audit turn changed one file of Checkout_Pipeline; the worker's
	// re-distill sees the new purpose.
	lister2 := &refreshedLister{}
	kb2, err := knowledge.Distill(context.Background(), ws, lister2, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := auditKnowledgePaths([]string{"shop/pay/charge.go", "docs/notes.md", "shop/pay/charge_test.go"})
	if len(paths) != 2 || paths[0] != "shop/pay/charge.go" || paths[1] != "shop/pay/charge_test.go" {
		t.Fatalf("hook paths = %v, want [shop/pay/charge.go shop/pay/charge_test.go]", paths)
	}
	UpdateAsync(ws, paths, func(context.Context) (*knowledge.KnowledgeBase, error) { return kb2, nil })
	after, err := os.ReadFile(flowsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sectionSlice(string(after), "## Flow: Checkout_Pipeline"), "WITH refunds") {
		t.Errorf("audit-affected section not refreshed:\n%s", after)
	}
	if sectionSlice(string(before), "## Flow: Login_Flow") != sectionSlice(string(after), "## Flow: Login_Flow") {
		t.Errorf("unaffected section changed by the audit update")
	}
	idx, err := knowledge.LoadIndex(ws)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Flows["Checkout_Pipeline"].Tokens != kb2.Index.Flows["Checkout_Pipeline"].Tokens {
		t.Errorf("index not synced to the audit refresh")
	}
}

// refreshedLister mirrors the stub flows with a new checkout purpose —
// what the worker's re-distill returns after the audited code changed.
type refreshedLister struct{}

func (refreshedLister) ListProcesses(ctx context.Context) ([]structure.FlowSummary, error) {
	flows, err := stubKnowledgeLister{}.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}
	flows[0].Label = "cart to order WITH refunds"
	return flows, nil
}

func (refreshedLister) ListModelCandidates(ctx context.Context) ([]structure.ModelInfo, error) {
	return nil, nil
}

// TestAuditNodeNonBlockingOnKnowledgeError pins Task-375 AC-2 + Review
// Protocol #2: a corrupted knowledge dir never fails, blocks, or panics the
// audit path — the worker logs and returns, on-disk bytes untouched.
func TestAuditNodeNonBlockingOnKnowledgeError(t *testing.T) {
	ws := t.TempDir()
	kb1, err := knowledge.Distill(context.Background(), ws, stubKnowledgeLister{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := knowledge.WriteFull(ws, kb1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowledge.KnowledgeDir(ws), "index.json"), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	flowsPath := filepath.Join(knowledge.KnowledgeDir(ws), "execution-flows.md")
	before, _ := os.ReadFile(flowsPath)
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Direct worker call (what the hook fires): must return normally.
		UpdateAsync(ws, []string{"shop/cart/service.go"}, func(context.Context) (*knowledge.KnowledgeBase, error) {
			return nil, errors.New("boom: distill unavailable")
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("knowledge worker blocked the audit path")
	}
	after, _ := os.ReadFile(flowsPath)
	if string(before) != string(after) {
		t.Errorf("failed update mutated the flows file")
	}
	// Hook-level: the seam itself returns immediately on a corrupt base.
	svc := &InteractiveService{}
	start := time.Now()
	svc.onAuditNodeCompleted(ws, []string{"shop/cart/service.go"})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("hook blocked %v (must be fire-and-forget)", elapsed)
	}
}

// TestAuditKnowledgePathsFilter pins the T-3 path source: only concrete
// code targets arm the worker; doc/test/empty turns are complete no-ops
// (UpdateAsync never even calls redistill).
func TestAuditKnowledgePathsFilter(t *testing.T) {
	if got := auditKnowledgePaths(nil); len(got) != 0 {
		t.Errorf("nil input = %v, want empty", got)
	}
	if got := auditKnowledgePaths([]string{"docs/notes.md", "README.md"}); len(got) != 0 {
		t.Errorf("doc-only turn = %v, want empty", got)
	}
	// Test files ARE concrete code targets (shared locus predicate): a test
	// rename tracks an API change, so it may stale a flow section.
	if got := auditKnowledgePaths([]string{"shop/pay/charge_test.go"}); len(got) != 1 {
		t.Errorf("test-file turn = %v, want [shop/pay/charge_test.go]", got)
	}
	got := auditKnowledgePaths([]string{"shop/pay/charge.go", "shop/pay/charge.go", "shop/cart/service.go"})
	if len(got) != 2 {
		t.Errorf("code turn = %v, want 2 deduped paths", got)
	}
}

// TestAuditHookSkipsUnbootstrappedWorkspace pins Task-375 AC-4: a workspace
// that never bootstrapped gets no mid-flow full rebuild — the hook is a
// silent no-op and creates nothing.
func TestAuditHookSkipsUnbootstrappedWorkspace(t *testing.T) {
	ws := t.TempDir()
	svc := &InteractiveService{}
	svc.onAuditNodeCompleted(ws, []string{"shop/cart/service.go"})
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(knowledge.KnowledgeDir(ws)); !os.IsNotExist(err) {
		t.Errorf("hook created knowledge state mid-flow (or errored stat): %v", err)
	}
}

func sectionSlice(content, heading string) string {
	start := strings.Index(content, heading)
	if start < 0 {
		return ""
	}
	rest := content[start:]
	next := strings.Index(rest[len(heading):], "\n## Flow: ")
	if next < 0 {
		return rest
	}
	return rest[:len(heading)+next]
}
