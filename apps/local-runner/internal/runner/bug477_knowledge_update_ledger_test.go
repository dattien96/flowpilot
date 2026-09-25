package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/knowledge"
)

// BUG-477: the audit-completion knowledge update must survive a runner kill.
// The changed-path set is persisted before the audit hook acknowledges and
// replayed under a per-workspace lease; nothing may be silently dropped, and
// corrupt intent state fails closed to a full rebuild.

func bug477SeedKnowledgeBase(t *testing.T, workspace string) {
	t.Helper()
	kb := &knowledge.KnowledgeBase{
		Overview: "# Overview\n",
		Models:   "# Models\n",
		Flows: []knowledge.RenderedFlow{
			{
				ID: "flow-a", File: "execution-flows.md",
				Heading: "## Flow: flow-a", Body: "## Flow: flow-a\n\noriginal body A\n",
			},
			{
				ID: "flow-b", File: "execution-flows.md",
				Heading: "## Flow: flow-b", Body: "## Flow: flow-b\n\noriginal body B\n",
			},
		},
		Index: knowledge.KnowledgeIndex{
			SchemaVersion: knowledge.SchemaVersion,
			Flows: map[string]knowledge.FlowIndexEntry{
				"flow-a": {File: "execution-flows.md", Heading: "## Flow: flow-a"},
				"flow-b": {File: "execution-flows.md", Heading: "## Flow: flow-b"},
			},
			Paths: map[string][]string{
				"internal/runner/foo.go": {"flow-a"},
				"internal/runner/bar.go": {"flow-b"},
			},
			Symbols: map[string][]string{},
		},
	}
	if err := knowledge.WriteFull(workspace, kb); err != nil {
		t.Fatalf("seed knowledge base: %v", err)
	}
}

func bug477Redistill(body string) func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
	return func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
		return &knowledge.KnowledgeBase{
			Overview: "# Overview\n",
			Models:   "# Models\n",
			Flows: []knowledge.RenderedFlow{
				{
					ID: "flow-a", File: "execution-flows.md",
					Heading: "## Flow: flow-a", Body: "## Flow: flow-a\n\n" + body + "\n",
				},
				{
					ID: "flow-b", File: "execution-flows.md",
					Heading: "## Flow: flow-b", Body: "## Flow: flow-b\n\n" + body + "\n",
				},
			},
			Index: knowledge.KnowledgeIndex{
				SchemaVersion: knowledge.SchemaVersion,
				Flows: map[string]knowledge.FlowIndexEntry{
					"flow-a": {File: "execution-flows.md", Heading: "## Flow: flow-a"},
					"flow-b": {File: "execution-flows.md", Heading: "## Flow: flow-b"},
				},
				Paths: map[string][]string{
					"internal/runner/foo.go": {"flow-a"},
					"internal/runner/bar.go": {"flow-b"},
				},
				Symbols: map[string][]string{},
			},
		}, nil
	}
}

func bug477PendingCount(t *testing.T, workspace string) int {
	t.Helper()
	pending, err := loadKnowledgePending(workspace)
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	return len(pending.Intents)
}

func TestBUG477_AuditCompletionPersistsDurableUpdateIntent(t *testing.T) {
	ws := t.TempDir()
	bug477SeedKnowledgeBase(t, ws)

	var svc *InteractiveService
	svc.onAuditNodeCompleted(ws, []string{"internal/runner/foo.go", "docs/readme.md"})

	pending, err := loadKnowledgePending(ws)
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if len(pending.Intents) == 0 {
		t.Fatal("audit completion left no durable update intent — a kill here loses the refresh forever (BUG-477)")
	}
	if got := pending.Intents[0].Paths; len(got) != 1 || got[0] != "internal/runner/foo.go" {
		t.Fatalf("intent paths = %#v — doc noise must be filtered before persisting", got)
	}
}

func TestBUG477_ReplayAppliesPendingIntentsAfterCrash(t *testing.T) {
	ws := t.TempDir()
	bug477SeedKnowledgeBase(t, ws)
	if err := appendKnowledgeUpdateIntent(ws, []string{"internal/runner/foo.go"}); err != nil {
		t.Fatalf("append intent: %v", err)
	}
	// Simulate the kill: no worker ran. The next process replays the durable
	// intent and the knowledge files converge.
	replayKnowledgeUpdates(ws, bug477Redistill("refreshed body"))

	if n := bug477PendingCount(t, ws); n != 0 {
		t.Fatalf("%d intents still pending after successful replay", n)
	}
	doc, err := os.ReadFile(filepath.Join(knowledge.KnowledgeDir(ws), "execution-flows.md"))
	if err != nil {
		t.Fatalf("read flows: %v", err)
	}
	if !strings.Contains(string(doc), "refreshed body") {
		t.Fatalf("flow section not refreshed after replay:\n%s", doc)
	}
}

func TestBUG477_FailedUpdateLeavesIntentsRetryable(t *testing.T) {
	old := knowledgeUpdateRetryDelay
	knowledgeUpdateRetryDelay = 0
	defer func() { knowledgeUpdateRetryDelay = old }()

	ws := t.TempDir()
	bug477SeedKnowledgeBase(t, ws)
	if err := appendKnowledgeUpdateIntent(ws, []string{"internal/runner/foo.go"}); err != nil {
		t.Fatalf("append intent: %v", err)
	}

	failing := func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
		return nil, context.DeadlineExceeded
	}
	replayKnowledgeUpdates(ws, failing)
	if n := bug477PendingCount(t, ws); n != 1 {
		t.Fatalf("failed replay dropped intents: %d pending, want 1", n)
	}

	replayKnowledgeUpdates(ws, bug477Redistill("recovered"))
	if n := bug477PendingCount(t, ws); n != 0 {
		t.Fatalf("retry did not commit: %d pending", n)
	}
	doc, err := os.ReadFile(filepath.Join(knowledge.KnowledgeDir(ws), "execution-flows.md"))
	if err != nil {
		t.Fatalf("read flows: %v", err)
	}
	if !strings.Contains(string(doc), "recovered") {
		t.Fatal("recovered replay did not refresh the section")
	}
}

func TestBUG477_CorruptIntentFailsClosedToFullRebuild(t *testing.T) {
	old := knowledgeUpdateRetryDelay
	knowledgeUpdateRetryDelay = 0
	defer func() { knowledgeUpdateRetryDelay = old }()

	ws := t.TempDir()
	bug477SeedKnowledgeBase(t, ws)
	path := knowledgePendingPath(ws)
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write corrupt ledger: %v", err)
	}

	called := false
	redistill := func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
		called = true
		kb, _ := bug477Redistill("rebuilt")(ctx)
		kb.Flows = kb.Flows[:1]
		return kb, nil
	}
	replayKnowledgeUpdates(ws, redistill)
	if !called {
		t.Fatal("corrupt ledger did not trigger the fail-closed rebuild")
	}
	if n := bug477PendingCount(t, ws); n != 0 {
		t.Fatalf("corrupt ledger not cleared after rebuild: %d pending", n)
	}
}

func TestBUG477_MultipleAuditCompletionsCoalesce(t *testing.T) {
	ws := t.TempDir()
	bug477SeedKnowledgeBase(t, ws)
	if err := appendKnowledgeUpdateIntent(ws, []string{"internal/runner/foo.go"}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := appendKnowledgeUpdateIntent(ws, []string{"internal/runner/bar.go"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	if n := bug477PendingCount(t, ws); n != 2 {
		t.Fatalf("pending = %d, want 2 intents", n)
	}
	replayKnowledgeUpdates(ws, bug477Redistill("both refreshed"))
	if n := bug477PendingCount(t, ws); n != 0 {
		t.Fatalf("coalesced replay left %d pending", n)
	}
	doc, err := os.ReadFile(filepath.Join(knowledge.KnowledgeDir(ws), "execution-flows.md"))
	if err != nil {
		t.Fatalf("read flows: %v", err)
	}
	// Both sections must carry the refreshed body — intent 2's path must not
	// be dropped when intent 1 commits.
	count := 0
	for _, want := range []string{"flow-a", "flow-b"} {
		section := knowledge.ExtractFlowSection(string(doc), "## Flow: "+want)
		if strings.Contains(section, "both refreshed") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("only %d/2 sections refreshed — an intent's paths were dropped:\n%s", count, doc)
	}
}

func TestBUG477_MissingIndexLeavesPendingForBootstrap(t *testing.T) {
	ws := t.TempDir() // no knowledge base — Missing() == true
	if err := appendKnowledgeUpdateIntent(ws, []string{"internal/runner/foo.go"}); err != nil {
		t.Fatalf("append intent: %v", err)
	}
	called := false
	replayKnowledgeUpdates(ws, func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
		called = true
		return bug477Redistill("x")(ctx)
	})
	if called {
		t.Fatal("replay must not full-rebuild mid-flow when the index is missing")
	}
	if n := bug477PendingCount(t, ws); n != 1 {
		t.Fatalf("pending = %d, want 1 (left for bootstrap)", n)
	}
	// A successful full bootstrap subsumes every pending intent.
	bug477SeedKnowledgeBase(t, ws)
	clearKnowledgePending(ws)
	if n := bug477PendingCount(t, ws); n != 0 {
		t.Fatalf("bootstrap clear left %d pending", n)
	}
}

