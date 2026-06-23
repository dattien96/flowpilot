package changeledger

import (
	"os"
	"path/filepath"
	"testing"
)

// ── parse.go ──────────────────────────────────────────────────────────────────

func TestParseRecord_FullCommit(t *testing.T) {
	raw := "abc123\x1f2026-01-15T10:00:00Z\x1f[Feature]: Task-040 Add chat feed\x1fsome body"
	e, ok := parseRecord(raw)
	if !ok {
		t.Fatal("expected ok")
	}
	if e.CommitHash != "abc123" {
		t.Errorf("hash: got %q", e.CommitHash)
	}
	if e.ChangeType != "feature" {
		t.Errorf("changeType: got %q", e.ChangeType)
	}
	if e.SourceDocID != "Task-040" {
		t.Errorf("sourceDocID: got %q", e.SourceDocID)
	}
	if e.Summary != "Add chat feed" {
		t.Errorf("summary: got %q", e.Summary)
	}
	if e.CommittedAt != "2026-01-15T10:00:00Z" {
		t.Errorf("committedAt: got %q", e.CommittedAt)
	}
	if e.Confidence != ConfidenceLow {
		t.Errorf("confidence: got %q", e.Confidence)
	}
}

func TestParseRecord_BugFix(t *testing.T) {
	raw := "def456\x1f2026-03-01T08:30:00Z\x1f[BugFix]: BUG-060 fix history replay\x1f"
	e, ok := parseRecord(raw)
	if !ok {
		t.Fatal("expected ok")
	}
	if e.ChangeType != "bugfix" {
		t.Errorf("changeType: got %q", e.ChangeType)
	}
	if e.SourceDocID != "BUG-060" {
		t.Errorf("sourceDocID: got %q", e.SourceDocID)
	}
}

func TestParseRecord_NoTag(t *testing.T) {
	raw := "aaa111\x1f2026-02-01T00:00:00Z\x1fsome commit with no tag\x1f"
	e, ok := parseRecord(raw)
	if !ok {
		t.Fatal("expected ok")
	}
	if e.ChangeType != "other" {
		t.Errorf("changeType: got %q", e.ChangeType)
	}
	if e.SourceDocID != "" {
		t.Errorf("sourceDocID: got %q", e.SourceDocID)
	}
	if e.Summary != "some commit with no tag" {
		t.Errorf("summary: got %q", e.Summary)
	}
}

func TestParseRecord_EmptyInput(t *testing.T) {
	_, ok := parseRecord("")
	if ok {
		t.Fatal("expected not ok for empty input")
	}
	_, ok = parseRecord("   \n  ")
	if ok {
		t.Fatal("expected not ok for whitespace-only input")
	}
}

func TestExtractChangeType(t *testing.T) {
	cases := []struct {
		subject string
		want    string
	}{
		{"[Feature]: add thing", "feature"},
		{"[BugFix]: fix thing", "bugfix"},
		{"[Bug]: fix thing", "bugfix"},
		{"[Refactor]: cleanup", "refactor"},
		{"[Docs]: update readme", "docs"},
		{"[Hotfix]: urgent patch", "hotfix"},
		{"plain commit message", "other"},
		{"[Unknown]: something", "other"},
	}
	for _, tc := range cases {
		got := extractChangeType(tc.subject)
		if got != tc.want {
			t.Errorf("extractChangeType(%q) = %q, want %q", tc.subject, got, tc.want)
		}
	}
}

func TestExtractSourceDocID(t *testing.T) {
	cases := []struct {
		subject string
		body    string
		want    string
	}{
		{"[Feature]: Task-087 desc", "", "Task-087"},
		{"[BugFix]: BUG-060 desc", "", "BUG-060"},
		{"[Docs]: update readme", "see Task-023 for context", "Task-023"},
		{"plain commit", "", ""},
		{"[Feature]: CP-35 rollout", "", "CP-35"},
	}
	for _, tc := range cases {
		got := extractSourceDocID(tc.subject, tc.body)
		if got != tc.want {
			t.Errorf("extractSourceDocID(%q, %q) = %q, want %q", tc.subject, tc.body, got, tc.want)
		}
	}
}

func TestCleanSummary(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"[Feature]: Task-087 Add slash commands", "Add slash commands"},
		{"[BugFix]: BUG-060 fix history replay", "fix history replay"},
		{"plain commit message", "plain commit message"},
		{"[Docs]: update readme", "update readme"},
		// Edge: stripping leaves nothing → fallback to original
		{"[Feature]:", "[Feature]:"},
	}
	for _, tc := range cases {
		got := cleanSummary(tc.input)
		if got != tc.want {
			t.Errorf("cleanSummary(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── ordering + cursor ─────────────────────────────────────────────────────────

func TestParseRecordsOrdering(t *testing.T) {
	// Simulate three records with out-of-order timestamps
	records := []string{
		"hash3\x1f2026-03-01T00:00:00Z\x1f[Feature]: Task-003 third\x1f",
		"hash1\x1f2026-01-01T00:00:00Z\x1f[Feature]: Task-001 first\x1f",
		"hash2\x1f2026-02-01T00:00:00Z\x1f[Feature]: Task-002 second\x1f",
	}

	var entries []Entry
	for _, raw := range records {
		if e, ok := parseRecord(raw); ok {
			entries = append(entries, e)
		}
	}

	// Apply the same sort ParseRepo uses
	for i := range entries {
		entries[i].OrderIndex = i
	}
	sortEntriesByTime(entries)
	for i := range entries {
		entries[i].OrderIndex = i
	}

	if entries[0].CommitHash != "hash1" {
		t.Errorf("expected hash1 first, got %s", entries[0].CommitHash)
	}
	if entries[1].CommitHash != "hash2" {
		t.Errorf("expected hash2 second, got %s", entries[1].CommitHash)
	}
	if entries[2].CommitHash != "hash3" {
		t.Errorf("expected hash3 last (newest), got %s", entries[2].CommitHash)
	}
	if entries[2].OrderIndex != 2 {
		t.Errorf("newest OrderIndex = %d, want 2", entries[2].OrderIndex)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor")

	if err := writeCursor(path, "abc123"); err != nil {
		t.Fatal(err)
	}
	got := readCursor(path)
	if got != "abc123" {
		t.Errorf("cursor round-trip: got %q, want %q", got, "abc123")
	}
}

func TestReadCursor_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor")
	got := readCursor(path)
	if got != "" {
		t.Errorf("missing cursor should return empty, got %q", got)
	}
}

// ── enrich.go ─────────────────────────────────────────────────────────────────

func TestParseCABlock(t *testing.T) {
	content := `# Some CA note

## Scope

Changed some stuff.

` + "```yaml" + `
# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: Task-087
entries:
  - symbol: ChatFeed
    layer: ui
    change: modified
    class: behavioral
# --->8---
` + "```"

	f, err := os.CreateTemp(t.TempDir(), "CA-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	key, docID := parseCABlock(f.Name())
	if key != "chat-ui" {
		t.Errorf("feature_key: got %q, want %q", key, "chat-ui")
	}
	if docID != "Task-087" {
		t.Errorf("source_doc_id: got %q, want %q", docID, "Task-087")
	}
}

func TestParseCABlock_NoBlock(t *testing.T) {
	content := "# CA note without ledger block\n\nJust prose."
	f, err := os.CreateTemp(t.TempDir(), "CA-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(content)
	f.Close()

	key, docID := parseCABlock(f.Name())
	if key != "" || docID != "" {
		t.Errorf("expected empty for no block, got key=%q docID=%q", key, docID)
	}
}

func TestLoadKnownKeys(t *testing.T) {
	content := `# Feature Key Registry

## Keys

- chat-ui — desktop chat workspace
- agent-spawn — child agent spawning
- workflow-runtime — workflow run engine
`
	f, err := os.CreateTemp(t.TempDir(), "FEATURE-KEYS*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(content)
	f.Close()

	keys := loadKnownKeys(f.Name())
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
	}
	if keys[0] != "chat-ui" {
		t.Errorf("keys[0]: got %q, want %q", keys[0], "chat-ui")
	}
	if keys[2] != "workflow-runtime" {
		t.Errorf("keys[2]: got %q, want %q", keys[2], "workflow-runtime")
	}
}

func TestEnrichEntry_CABlock(t *testing.T) {
	caDir := t.TempDir()
	caFile := filepath.Join(caDir, "CA-001.md")
	content := "```yaml\n# ---8<--- flowpilot:change-ledger\nfeature_key: chat-ui\nsource_doc_id: Task-087\nentries:\n  - symbol: Foo\n    layer: ui\n    change: modified\n    class: behavioral\n# --->8---\n```"
	os.WriteFile(caFile, []byte(content), 0o644)

	caIndex := map[string]string{"Task-087": "chat-ui"}
	e := Entry{CommitHash: "abc", SourceDocID: "Task-087", Summary: "add slash cmd", Confidence: ConfidenceLow}

	got := enrichEntry(e, t.TempDir(), caIndex, nil)
	if got.FeatureKey != "chat-ui" {
		t.Errorf("FeatureKey: got %q, want %q", got.FeatureKey, "chat-ui")
	}
	if got.Confidence != ConfidenceHigh {
		t.Errorf("Confidence: got %q, want %q", got.Confidence, ConfidenceHigh)
	}
}

func TestEnrichEntry_KeywordMatch(t *testing.T) {
	knownKeys := []string{"chat-ui", "agent-spawn", "workflow-runtime"}
	e := Entry{CommitHash: "abc", Summary: "update chat ui components", Confidence: ConfidenceLow}

	got := enrichEntry(e, t.TempDir(), nil, knownKeys)
	if got.FeatureKey != "chat-ui" {
		t.Errorf("FeatureKey: got %q, want %q", got.FeatureKey, "chat-ui")
	}
	if got.Confidence != ConfidenceLow {
		t.Errorf("Confidence: got %q, want %q", got.Confidence, ConfidenceLow)
	}
}

func TestEnrichEntry_FallbackDocID(t *testing.T) {
	e := Entry{CommitHash: "abc", SourceDocID: "Task-999", Summary: "some change", Confidence: ConfidenceLow}

	got := enrichEntry(e, t.TempDir(), map[string]string{}, nil)
	if got.FeatureKey != "task-999" {
		t.Errorf("FeatureKey: got %q, want %q", got.FeatureKey, "task-999")
	}
}

func TestTopLevelKey(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"apps/admin-web/src/chat/foo.tsx", "admin-web"},
		{"apps/local-runner/internal/runner/foo.go", "local-runner"},
		{"src/components/bar.go", "src"},
		{"packages/shared/utils.ts", "shared"},
		{"README.md", "readme"},
	}
	for _, tc := range cases {
		got := topLevelKey(tc.path)
		if got != tc.want {
			t.Errorf("topLevelKey(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// ── ledger store + query ───────────────────────────────────────────────────────

func TestLedgerUpsertAndQuery(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	entries := []Entry{
		{CommitHash: "h1", FeatureKey: "chat-ui", CommittedAt: "2026-01-01T00:00:00Z", Summary: "init", Confidence: ConfidenceHigh},
		{CommitHash: "h2", FeatureKey: "chat-ui", CommittedAt: "2026-03-01T00:00:00Z", Summary: "improve", Confidence: ConfidenceLow},
		{CommitHash: "h3", FeatureKey: "agent-spawn", CommittedAt: "2026-02-01T00:00:00Z", Summary: "spawn agents", Confidence: ConfidenceHigh},
	}
	if err := l.Upsert(entries); err != nil {
		t.Fatal(err)
	}

	// GetFeatureHistory: chat-ui ordered oldest→newest
	history, err := l.GetFeatureHistory("chat-ui")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 chat-ui entries, got %d", len(history))
	}
	if history[0].CommitHash != "h1" {
		t.Errorf("oldest should be h1, got %s", history[0].CommitHash)
	}
	if history[1].CommitHash != "h2" {
		t.Errorf("newest should be h2, got %s", history[1].CommitHash)
	}
	if history[0].OrderIndex != 0 {
		t.Errorf("oldest OrderIndex = %d, want 0", history[0].OrderIndex)
	}
	if history[1].OrderIndex != 1 {
		t.Errorf("newest OrderIndex = %d, want 1", history[1].OrderIndex)
	}

	// ListFeatures
	features := l.ListFeatures()
	if len(features) != 2 {
		t.Fatalf("expected 2 features, got %d: %v", len(features), features)
	}
	// Should be sorted
	if features[0] != "agent-spawn" || features[1] != "chat-ui" {
		t.Errorf("features: got %v", features)
	}
}

func TestLedgerPersistence(t *testing.T) {
	dir := t.TempDir()

	l1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l1.Upsert([]Entry{
		{CommitHash: "h1", FeatureKey: "chat-ui", CommittedAt: "2026-01-01T00:00:00Z", Summary: "init"},
	}); err != nil {
		t.Fatal(err)
	}

	// Re-open: entries should survive
	l2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	history, err := l2.GetFeatureHistory("chat-ui")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 persisted entry, got %d", len(history))
	}
}

func TestLedgerLastWins(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	first := Entry{CommitHash: "h1", FeatureKey: "chat-ui", CommittedAt: "2026-01-01T00:00:00Z", Summary: "old summary"}
	updated := Entry{CommitHash: "h1", FeatureKey: "chat-ui", CommittedAt: "2026-01-01T00:00:00Z", Summary: "new summary"}

	_ = l.Upsert([]Entry{first})
	_ = l.Upsert([]Entry{updated})

	history, _ := l.GetFeatureHistory("chat-ui")
	if len(history) != 1 {
		t.Fatalf("expected 1 entry (last-wins dedup), got %d", len(history))
	}
	if history[0].Summary != "new summary" {
		t.Errorf("summary: got %q, want %q", history[0].Summary, "new summary")
	}
}

func TestLatestEntry(t *testing.T) {
	dir := t.TempDir()
	l, _ := New(dir)
	_ = l.Upsert([]Entry{
		{CommitHash: "h1", FeatureKey: "chat-ui", CommittedAt: "2026-01-01T00:00:00Z", Summary: "old"},
		{CommitHash: "h2", FeatureKey: "chat-ui", CommittedAt: "2026-06-01T00:00:00Z", Summary: "latest"},
	})

	e, ok := l.LatestEntry("chat-ui")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.Summary != "latest" {
		t.Errorf("LatestEntry summary: got %q, want %q", e.Summary, "latest")
	}
}

func TestGetFeatureHistory_NotFound(t *testing.T) {
	dir := t.TempDir()
	l, _ := New(dir)
	history, err := l.GetFeatureHistory("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if history != nil {
		t.Errorf("expected nil for unknown feature, got %v", history)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// sortEntriesByTime sorts entries ascending by CommittedAt (helper exposed for tests).
func sortEntriesByTime(entries []Entry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].CommittedAt > entries[j].CommittedAt {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}
