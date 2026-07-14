package featurecatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/changeledger"
)

// --- tokenize ---

func TestTokenize_RemovesStopwords(t *testing.T) {
	tokens := tokenize("the quick brown fox")
	for _, tok := range tokens {
		if tok == "the" {
			t.Errorf("stopword 'the' should be removed")
		}
	}
}

func TestTokenize_RemovesShortTokens(t *testing.T) {
	tokens := tokenize("a go is ok")
	for _, tok := range tokens {
		if len(tok) < 3 {
			t.Errorf("token %q shorter than 3 chars should be removed", tok)
		}
	}
}

func TestTokenize_UniqueTokens(t *testing.T) {
	tokens := tokenize("chat chat chat")
	if len(tokens) != 1 {
		t.Errorf("expected 1 unique token, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenize_SplitsOnNonAlpha(t *testing.T) {
	tokens := tokenize("chat-ui feature")
	found := make(map[string]bool)
	for _, tok := range tokens {
		found[tok] = true
	}
	if !found["chat"] {
		t.Error("expected 'chat' in tokens")
	}
	if !found["feature"] {
		t.Error("expected 'feature' in tokens")
	}
}

// --- Build from FEATURE-KEYS.md ---

type stubLedger struct{ keys []string }

func (s *stubLedger) ListFeatures() []string { return s.keys }

func writeTempFeatureKeys(t *testing.T, dir, content string) {
	t.Helper()
	auditDir := filepath.Join(dir, "change-audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auditDir, "FEATURE-KEYS.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_FromFeatureKeys(t *testing.T) {
	repoDir := t.TempDir()
	dotDir := t.TempDir()

	writeTempFeatureKeys(t, repoDir, `# Feature Keys

- chat-ui — Real-time chat user interface
- auth-flow — Authentication and authorization flow
`)

	ledger := &stubLedger{}
	cat, err := Build(repoDir, ledger, dotDir)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	f, ok := cat.Get("chat-ui")
	if !ok {
		t.Fatal("expected 'chat-ui' feature in catalog")
	}
	if !strings.Contains(f.Summary, "Real-time") {
		t.Errorf("unexpected summary: %q", f.Summary)
	}

	_, ok = cat.Get("auth-flow")
	if !ok {
		t.Error("expected 'auth-flow' feature in catalog")
	}
}

func TestBuild_PersistsNDJSON(t *testing.T) {
	repoDir := t.TempDir()
	dotDir := t.TempDir()

	writeTempFeatureKeys(t, repoDir, "- my-feature — My great feature\n")
	ledger := &stubLedger{}

	if _, err := Build(repoDir, ledger, dotDir); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	cat2, err := LoadCatalog(dotDir)
	if err != nil {
		t.Fatalf("LoadCatalog error: %v", err)
	}
	if _, ok := cat2.Get("my-feature"); !ok {
		t.Error("expected 'my-feature' after round-trip load")
	}
}

func TestBuild_AddsLedgerKeysNotInFeatureKeys(t *testing.T) {
	repoDir := t.TempDir()
	dotDir := t.TempDir()

	writeTempFeatureKeys(t, repoDir, "- chat-ui — Chat UI\n")
	ledger := &stubLedger{keys: []string{"new-from-ledger"}}

	cat, err := Build(repoDir, ledger, dotDir)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if _, ok := cat.Get("new-from-ledger"); !ok {
		t.Error("expected ledger-only key in catalog")
	}
}

func TestBuild_SupersededFeatureKeyUsesSuccessor(t *testing.T) {
	repoDir := t.TempDir()
	dotDir := t.TempDir()

	writeTempFeatureKeys(t, repoDir, `- old-chat — superseded by chat-ui
- chat-ui — Chat UI
`)

	cat, err := Build(repoDir, &stubLedger{}, dotDir)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if _, ok := cat.Get("old-chat"); ok {
		t.Fatal("superseded key should not be offered as an active feature")
	}
	if _, ok := cat.Get("chat-ui"); !ok {
		t.Fatal("successor key should be active")
	}
}

// TestBuild_GoverningDocAttachesToDeclaredFeature verifies BUG-280: a governing
// doc that declares a "Feature Keys:" metadata line has its stem attached to
// that real feature's DocRefs (so the feature becomes spec-backed), instead of
// only ever matching a doc-stem-named phantom feature.
func TestBuild_GoverningDocAttachesToDeclaredFeature(t *testing.T) {
	repoDir := t.TempDir()
	dotDir := t.TempDir()

	writeTempFeatureKeys(t, repoDir, "- calc-core — divide function\n")

	specDir := filepath.Join(repoDir, "requirements", "05-System-Specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "SS-99-Calc.md"),
		[]byte("# SS-99 Calc\n\n## Metadata\n\n- Feature Keys: `calc-core`\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := Build(repoDir, &stubLedger{}, dotDir)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	feat, ok := cat.Get("calc-core")
	if !ok {
		t.Fatal("expected calc-core feature")
	}
	found := false
	for _, ref := range feat.DocRefs {
		if ref == "SS-99-Calc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SS-99-Calc attached to calc-core DocRefs, got %v", feat.DocRefs)
	}
}

func TestParseFeatureKeysLine(t *testing.T) {
	cases := map[string][]string{
		"- Feature Keys: `change-contract`":         {"change-contract"},
		"Feature Keys: change-contract, other-feat": {"change-contract", "other-feat"},
		"- Feature Key: `calc-core`":                {"calc-core"},
		"- Parent Documents: [CP-43](x)":            nil,
		"regular text":                              nil,
	}
	for line, want := range cases {
		got := parseFeatureKeysLine(line)
		if len(got) != len(want) {
			t.Fatalf("%q → %v, want %v", line, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%q → %v, want %v", line, got, want)
			}
		}
	}
}

// --- ResolveFeature ---

func catalogWithFeatures(features ...Feature) *Catalog {
	c := New()
	for _, f := range features {
		c.Add(f)
	}
	return c
}

func TestResolveFeature_TopCandidateIsChatUI(t *testing.T) {
	cat := catalogWithFeatures(
		Feature{Key: "chat-ui", Title: "Chat UI", Keywords: tokenize("chat-ui chat ui")},
		Feature{Key: "auth-flow", Title: "Auth Flow", Keywords: tokenize("auth-flow authentication")},
		Feature{Key: "billing", Title: "Billing", Keywords: tokenize("billing payment invoice")},
	)

	candidates, err := ResolveFeature("update the chat ui", cat)
	if err != nil {
		t.Fatalf("ResolveFeature error: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}
	if candidates[0].Key != "chat-ui" {
		t.Errorf("expected top candidate 'chat-ui', got %q (score %.1f)", candidates[0].Key, candidates[0].Score)
	}
}

func TestResolveFeature_UnrelatedNLReturnsEmpty(t *testing.T) {
	cat := catalogWithFeatures(
		Feature{Key: "chat-ui", Title: "Chat UI", Keywords: tokenize("chat ui")},
	)

	candidates, err := ResolveFeature("xyz completely unrelated qqq", cat)
	if err != nil {
		t.Fatalf("ResolveFeature error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates for unrelated NL, got %d: %v", len(candidates), candidates)
	}
}

// --- TopCandidate ---

func TestTopCandidate_AboveThreshold(t *testing.T) {
	candidates := []Candidate{
		{Key: "chat-ui", Score: 8.0},
		{Key: "auth-flow", Score: 3.0},
	}
	top, ok := TopCandidate(candidates, 5.0)
	if !ok {
		t.Fatal("expected TopCandidate to return true")
	}
	if top.Key != "chat-ui" {
		t.Errorf("expected 'chat-ui', got %q", top.Key)
	}
}

func TestTopCandidate_BelowThreshold(t *testing.T) {
	candidates := []Candidate{
		{Key: "chat-ui", Score: 2.0},
	}
	_, ok := TopCandidate(candidates, 5.0)
	if ok {
		t.Error("expected TopCandidate to return false for score below threshold")
	}
}

func TestTopCandidate_EmptyCandidates(t *testing.T) {
	_, ok := TopCandidate(nil, 5.0)
	if ok {
		t.Error("expected TopCandidate to return false for empty candidates")
	}
}

// --- HistorySlot ---

func TestHistorySlot_CurrentTruthMarker(t *testing.T) {
	entries := []changeledger.Entry{
		{CommitHash: "aabbccdd1234", FeatureKey: "chat-ui", SourceDocID: "Task-010", Summary: "initial chat", CommittedAt: "2024-01-15T10:00:00Z"},
		{CommitHash: "eeff99881234", FeatureKey: "chat-ui", SourceDocID: "Task-020", Summary: "add reactions", CommittedAt: "2024-03-22T08:00:00Z"},
	}

	stub := &historyStub{entries: entries}
	result := HistorySlot("chat-ui", stub)

	if result == "" {
		t.Fatal("expected non-empty HistorySlot output")
	}
	if !strings.Contains(result, "← current truth") {
		t.Errorf("expected '← current truth' marker in output:\n%s", result)
	}

	lines := strings.Split(strings.TrimSpace(result), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "← current truth") {
		t.Errorf("'← current truth' marker should be on the last entry line, got:\n%s", lastLine)
	}

	firstEntryLine := lines[1]
	if strings.Contains(firstEntryLine, "← current truth") {
		t.Error("'← current truth' marker should NOT appear on first entry")
	}
}

func TestHistorySlot_EmptyReturnsEmpty(t *testing.T) {
	stub := &historyStub{entries: nil}
	result := HistorySlot("nonexistent", stub)
	if result != "" {
		t.Errorf("expected empty string for missing feature, got %q", result)
	}
}

func TestHistorySlot_UsesCommitHashWhenNoDocID(t *testing.T) {
	entries := []changeledger.Entry{
		{CommitHash: "deadbeef1234", FeatureKey: "chat-ui", SourceDocID: "", Summary: "bare commit", CommittedAt: "2024-05-01T00:00:00Z"},
	}
	stub := &historyStub{entries: entries}
	result := HistorySlot("chat-ui", stub)
	if !strings.Contains(result, "deadbeef") {
		t.Errorf("expected commit hash prefix in output:\n%s", result)
	}
}

// A feature with many commits must inject a bounded block: only the most recent
// recentHistoryEntryCount entries render, prefixed with an omission marker, and
// the newest is still tagged "← current truth".
func TestHistorySlot_CapsLongHistory(t *testing.T) {
	var entries []changeledger.Entry
	for i := 0; i < 100; i++ {
		entries = append(entries, changeledger.Entry{
			CommitHash:  fmt.Sprintf("hash%08d", i),
			FeatureKey:  "calc-core",
			SourceDocID: fmt.Sprintf("Task-%03d", i),
			Summary:     fmt.Sprintf("commit number %d", i),
			CommittedAt: fmt.Sprintf("2024-01-%02dT10:00:00Z", (i%27)+1),
		})
	}

	result := HistorySlot("calc-core", &historyStub{entries: entries})
	entryLines := 0
	for _, ln := range strings.Split(result, "\n") {
		if strings.HasPrefix(ln, "- [") {
			entryLines++
		}
	}
	if entryLines != recentHistoryEntryCount {
		t.Fatalf("rendered %d entry lines, want %d (capped)", entryLines, recentHistoryEntryCount)
	}
	if !strings.Contains(result, "older entries omitted") {
		t.Errorf("expected omission marker for the dropped entries:\n%s", result)
	}
	if !strings.Contains(result, "← current truth") {
		t.Error("newest entry should still carry the current-truth marker")
	}
}

type historyStub struct {
	entries []changeledger.Entry
}

func (h *historyStub) GetFeatureHistory(key string) ([]changeledger.Entry, error) {
	var out []changeledger.Entry
	for _, e := range h.entries {
		if e.FeatureKey == key {
			out = append(out, e)
		}
	}
	return out, nil
}
