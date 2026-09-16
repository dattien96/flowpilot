package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/structure"
)

// liveLister wraps the real structure queries (Task-373 manual §7 evidence).
type liveLister struct{ repoDir string }

func (l *liveLister) ListProcesses(ctx context.Context) ([]structure.FlowSummary, error) {
	return structure.Processes(ctx, l.repoDir, 10)
}

func (l *liveLister) ListModelCandidates(ctx context.Context) ([]structure.ModelInfo, error) {
	return structure.ModelCandidates(ctx, l.repoDir, 10)
}

// TestDistillLiveFlowPilot distills the FlowPilot repo itself through the
// real `gitnexus cypher` path. Opt-in only (needs the index + CLI):
// FLOWPILOT_KNOWLEDGE_LIVE=1. Manual §7 proof for Task-373, never CI.
func TestDistillLiveFlowPilot(t *testing.T) {
	if os.Getenv("FLOWPILOT_KNOWLEDGE_LIVE") != "1" {
		t.Skip("live gitnexus distill: set FLOWPILOT_KNOWLEDGE_LIVE=1")
	}
	repoDir := os.Getenv("FLOWPILOT_REPO")
	if repoDir == "" {
		t.Fatal("FLOWPILOT_REPO must point at the indexed flowpilot checkout")
	}
	kb, err := Distill(context.Background(), repoDir, &liveLister{repoDir: repoDir}, nil)
	if err != nil {
		t.Fatalf("live Distill: %v", err)
	}
	if len(kb.Flows) == 0 {
		t.Fatal("live distill found zero flows — index or query shape changed")
	}
	out := t.TempDir()
	if err := WriteFull(out, kb); err != nil {
		t.Fatalf("live WriteFull: %v", err)
	}
	idx, err := LoadIndex(out)
	if err != nil {
		t.Fatal(err)
	}
	for id, entry := range idx.Flows {
		if _, err := os.Stat(filepath.Join(KnowledgeDir(out), entry.File)); err != nil {
			t.Errorf("flow %s points at missing file %s", id, entry.File)
		}
		if !strings.HasPrefix(entry.Heading, "## Flow: ") {
			t.Errorf("flow %s bad heading %q", id, entry.Heading)
		}
	}
	t.Logf("live distill: %d flows, %d path keys, %d symbol keys; overview %d bytes",
		len(kb.Flows), len(idx.Paths), len(idx.Symbols), len(kb.Overview))
}
