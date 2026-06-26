package changeledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnrichAllKeepsVerifiedFeatureKeyAndAttachesCAExcerpt(t *testing.T) {
	repoDir := t.TempDir()
	auditDir := filepath.Join(repoDir, "change-audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auditDir, "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(auditDir, "CA-001.md")
	if err := os.WriteFile(caPath, []byte(`# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: Task-001
# --->8---

## Scope
Chat UI

## Residual Notes
Keep summary
`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := EnrichAll([]Entry{{CommitHash: "abc", FeatureKey: "chat-ui", SourceDocID: "Task-001", Summary: "keep", Confidence: ConfidenceLow}}, repoDir)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].FeatureKey != "chat-ui" || got[0].Confidence != ConfidenceHigh {
		t.Fatalf("verified key should remain high: %+v", got[0])
	}
	if got[0].CAExcerpt == "" {
		t.Fatal("expected CA excerpt to be attached for verified key")
	}
}
