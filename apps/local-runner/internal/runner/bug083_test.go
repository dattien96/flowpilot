package runner

// BUG-083: Desktop Chat Resume Replays Composed Prompt, Not User Input
// (Codex Also Drops Turns and Renders Injected Context As Prompt)
//
// Three defects covered:
//   F-1  raw prompt override via turn log
//   F-2  CLI-injected context frame filtered from Codex replay
//   F-3  all Codex per-turn rollout files loaded on resume

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// --- F-2: injected-context filtering in the Codex loader ----------------------

func TestCodexLoaderFiltersInjectedContextFrame(t *testing.T) {
	// A rollout where the first role:user entry is a CLI-injected AGENTS.md preamble.
	// Only the real user prompt must appear in the output.
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system prompt"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for C:\\repo\n<INSTRUCTIONS>\nsome rules\n</INSTRUCTIONS>\n<environment_context>\ncwd: C:\\repo\n</environment_context>"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello there"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi!"}]}}`,
	}
	events, _ := loadCodexTranscriptEvents(writeLines083(t, lines))

	var prompts []string
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			prompts = append(prompts, e.Prompt)
		}
	}
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt event, got %d: %v", len(prompts), prompts)
	}
	if prompts[0] != "hello there" {
		t.Fatalf("prompt = %q, want %q", prompts[0], "hello there")
	}
}

func TestCodexLoaderFiltersInstructionsTag(t *testing.T) {
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<INSTRUCTIONS>\ndo stuff\n</INSTRUCTIONS>"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"real prompt"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}}`,
	}
	events, _ := loadCodexTranscriptEvents(writeLines083(t, lines))
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" && e.Prompt != "real prompt" {
			t.Fatalf("<INSTRUCTIONS> frame must not be emitted as a prompt bubble; got %q", e.Prompt)
		}
	}
}

func TestCodexLoaderDoesNotFilterNormalMentionOfAgentsMd(t *testing.T) {
	// A user message that merely mentions AGENTS.md in natural language must not
	// be mistaken for a CLI injection frame.
	text := "Can you read the AGENTS.md file and summarise it?"
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"sure"}]}}`,
	}
	events, _ := loadCodexTranscriptEvents(writeLines083(t, lines))
	found := false
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt == text {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected turn_started with prompt %q; not found in %+v", text, events)
	}
}

// --- F-1: raw prompt override via turn log (Claude) ---------------------------

func TestSeedTranscriptUsesRawPromptFromTurnLog(t *testing.T) {
	root := t.TempDir()
	acctHome := filepath.Join(root, "user-home") // Claude uses the user home dir

	// Claude JSONL with the COMPOSED prompt (includes the reinforcement suffix).
	composedPrompt := `hello claude\n\n---\nComplete the clear, unambiguous parts of the task directly.`
	rawPrompt := "hello claude"
	claudeLines := []string{
		`{"type":"system","subtype":"init","session_id":"s-abc","cwd":"/repo"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"` + composedPrompt + `"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hi there!"}]}}`,
		`{"type":"result","subtype":"success","result":"Hi there!"}`,
	}
	sessionID := "ses-abc123"
	claudeDir := filepath.Join(acctHome, ".claude", "projects", "proj-hash")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeLinesToPath083(t, filepath.Join(claudeDir, sessionID+".jsonl"), claudeLines)

	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-c", ProviderKey: "claude", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), IsActive: true},
	})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-bug083-f1"
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             runID,
		ProjectID:         "proj-1",
		ProviderKey:       ProviderKeyClaude,
		ProviderSessionID: sessionID,
		ProviderAccountID: "acct-c",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	})
	// Turn log records the RAW prompt — what the user actually typed.
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: rawPrompt})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-c"

	rs := &interactiveRun{
		id:                    runID,
		providerKey:           ProviderKeyClaude,
		providerSessionID:     sessionID,
		realProviderSessionID: sessionID,
		providerAccountID:     "acct-c",
		workspaceCwd:          "/repo",
		runKind:               "chat",
		status:                RunStatusCompleted,
		createdAt:             time.Now().UTC().Format(time.RFC3339Nano),
		resumedFromDisk:       true,
		subs:                  map[int64]chan ProviderEvent{},
		idempotency:           map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var promptEvents []ProviderEvent
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			promptEvents = append(promptEvents, e)
		}
	}
	if len(promptEvents) != 1 {
		t.Fatalf("expected 1 prompt event, got %d", len(promptEvents))
	}
	if promptEvents[0].Prompt != rawPrompt {
		t.Fatalf("Prompt = %q, want %q (composed prompt must not be shown)", promptEvents[0].Prompt, rawPrompt)
	}
}

// --- F-3: multi-rollout Codex replay ------------------------------------------

func TestSeedTranscriptLoadsAllCodexRolloutFiles(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home") // Codex HomePath is the .codex dir

	ts1 := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(16 * time.Second)
	sid1 := "aaa-turn1-sid"
	sid2 := "bbb-turn2-sid"

	// Turn 1 rollout file: meta + injected context (filtered) + composed prompt + assistant.
	turn1Lines := []string{
		`{"payload":{"id":"` + sid1 + `","cwd":"/repo","timestamp":"` + ts1.Format(time.RFC3339Nano) + `"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<INSTRUCTIONS>\nrules\n</INSTRUCTIONS>"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello codex 1 --- reinforcement"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi from turn 1"}]}}`,
	}
	// Turn 2 rollout file: meta + injected context (filtered) + composed prompt + assistant.
	turn2Lines := []string{
		`{"payload":{"id":"` + sid2 + `","cwd":"/repo","timestamp":"` + ts2.Format(time.RFC3339Nano) + `"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<INSTRUCTIONS>\nrules\n</INSTRUCTIONS>"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello codex 2 --- reinforcement"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi from turn 2"}]}}`,
	}

	writeCodexRolloutLines(t, codexHome, sid1, "/repo", ts1, turn1Lines)
	writeCodexRolloutLines(t, codexHome, sid2, "/repo", ts2, turn2Lines)

	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-x", ProviderKey: "codex", HomePath: codexHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-bug083-f3"
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             runID,
		ProjectID:         "proj-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: sid1, // stored id is turn 1's id (the bug scenario)
		ProviderAccountID: "acct-x",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         ts1.Format(time.RFC3339Nano),
		UpdatedAt:         ts2.Format(time.RFC3339Nano),
	})
	// Turn log: prompts (raw) and session ids for both turns.
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "hello codex 1"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindCodexSession, SessionID: sid1})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "hello codex 2"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindCodexSession, SessionID: sid2})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-x"

	rs := &interactiveRun{
		id:                    runID,
		providerKey:           ProviderKeyCodex,
		providerSessionID:     sid1,
		realProviderSessionID: sid1,
		providerAccountID:     "acct-x",
		workspaceCwd:          "/repo",
		runKind:               "chat",
		status:                RunStatusCompleted,
		createdAt:             ts1.Format(time.RFC3339Nano),
		resumedFromDisk:       true,
		subs:                  map[int64]chan ProviderEvent{},
		idempotency:           map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	// Both turns must appear with raw prompts and both assistant responses.
	var prompts, assistants []string
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			prompts = append(prompts, e.Prompt)
		}
		if e.Type == EventMessageCompleted && e.Text != "" {
			assistants = append(assistants, e.Text)
		}
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompt events, got %d: %v", len(prompts), prompts)
	}
	if prompts[0] != "hello codex 1" {
		t.Fatalf("prompts[0] = %q, want raw %q", prompts[0], "hello codex 1")
	}
	if prompts[1] != "hello codex 2" {
		t.Fatalf("prompts[1] = %q, want raw %q", prompts[1], "hello codex 2")
	}
	if len(assistants) != 2 {
		t.Fatalf("expected 2 assistant messages, got %d: %v", len(assistants), assistants)
	}
}

func TestSeedTranscriptFallsBackToTurnLogForSyntheticCodexFlowHub(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}

	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-x", ProviderKey: "codex", HomePath: codexHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-bug083-fallback"
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             runID,
		ProjectID:         "proj-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-123",
		ProviderAccountID: "acct-x",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "workflow",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder"}},
	})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "fix bug 1+1 != 2"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "[flow-engine] Agent results ready."})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-x"

	rs := &interactiveRun{
		id:                runID,
		providerKey:       ProviderKeyCodex,
		providerSessionID: "thread-123",
		providerAccountID: "acct-x",
		workspaceCwd:      "/repo",
		runKind:           "workflow",
		status:            RunStatusCancelled,
		createdAt:         time.Now().UTC().Format(time.RFC3339Nano),
		resumedFromDisk:   true,
		activeFlowNodes:   []agentpack.FlowNode{{ID: "coder"}},
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var prompts []string
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			prompts = append(prompts, e.Prompt)
		}
	}
	if !reflect.DeepEqual(prompts, []string{"fix bug 1+1 != 2"}) {
		t.Fatalf("prompt-only fallback replay = %#v, want user-facing turn-log prompts only", prompts)
	}
}

func TestSeedTranscriptIncludesStoredCodexSessionWhenTurnLogIsPartial(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	ts1 := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(time.Minute)
	sid1 := "legacy-stored-sid"
	sid2 := "new-turn-sid"

	writeCodexRolloutLines(t, codexHome, sid1, "/repo", ts1, []string{
		`{"payload":{"id":"` + sid1 + `","cwd":"/repo","timestamp":"` + ts1.Format(time.RFC3339Nano) + `"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"legacy prompt --- reinforcement"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"legacy answer"}]}}`,
	})
	writeCodexRolloutLines(t, codexHome, sid2, "/repo", ts2, []string{
		`{"payload":{"id":"` + sid2 + `","cwd":"/repo","timestamp":"` + ts2.Format(time.RFC3339Nano) + `"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"new prompt --- reinforcement"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"new answer"}]}}`,
	})

	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-x", ProviderKey: "codex", HomePath: codexHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-bug083-partial"
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             runID,
		ProjectID:         "proj-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: sid1,
		ProviderAccountID: "acct-x",
		WorkingDirectory:  "/repo",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         ts1.Format(time.RFC3339Nano),
		UpdatedAt:         ts2.Format(time.RFC3339Nano),
	})
	// Legacy run: the stored session predates BUG-083, so only the later turn was
	// written to the sidecar. Replay must still include the persisted baseline id.
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "legacy prompt"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "new prompt"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindCodexSession, SessionID: sid2})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-x"
	rs := &interactiveRun{
		id:                    runID,
		providerKey:           ProviderKeyCodex,
		providerSessionID:     sid1,
		realProviderSessionID: sid1,
		providerAccountID:     "acct-x",
		workspaceCwd:          "/repo",
		runKind:               "chat",
		status:                RunStatusCompleted,
		createdAt:             ts1.Format(time.RFC3339Nano),
		resumedFromDisk:       true,
		subs:                  map[int64]chan ProviderEvent{},
		idempotency:           map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var prompts, assistants []string
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			prompts = append(prompts, e.Prompt)
		}
		if e.Type == EventMessageCompleted && e.Text != "" {
			assistants = append(assistants, e.Text)
		}
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompt events, got %d: %v", len(prompts), prompts)
	}
	if prompts[0] != "legacy prompt" || prompts[1] != "new prompt" {
		t.Fatalf("unexpected prompts: %v", prompts)
	}
	if len(assistants) != 2 {
		t.Fatalf("expected 2 assistant messages, got %d: %v", len(assistants), assistants)
	}
}

// --- TurnLogStore unit tests ---------------------------------------------------

func TestTurnLogStoreRoundTrip(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-tl-1"

	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "first prompt"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindCodexSession, SessionID: "sess-1"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindAssistant, Assistant: "first full answer"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindTranscriptTurn, TurnID: "turn-1", Prompt: "first prompt", Assistant: "first full answer"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "second prompt"})
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindCodexSession, SessionID: "sess-2"})

	entries, err := store.ReadTurnLog(context.Background(), runID)
	if err != nil {
		t.Fatalf("ReadTurnLog: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(entries))
	}
	if entries[0].Prompt != "first prompt" || entries[1].SessionID != "sess-1" || entries[2].Assistant != "first full answer" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if entries[3].TurnID != "turn-1" || entries[3].Prompt != "first prompt" || entries[3].Assistant != "first full answer" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestTurnLogStoreMissingFileReturnsNil(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	entries, err := store.ReadTurnLog(context.Background(), "nonexistent-run")
	if err != nil {
		t.Fatalf("ReadTurnLog for missing file: %v", err)
	}
	if entries != nil {
		t.Fatalf("expected nil entries for missing file, got %+v", entries)
	}
}

func TestTurnLogStoreDeleteRemovesFile(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-tl-del"
	_ = store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: "hi"})
	if err := store.DeleteTurnLog(context.Background(), runID); err != nil {
		t.Fatalf("DeleteTurnLog: %v", err)
	}
	// Second delete should be a no-op (not an error).
	if err := store.DeleteTurnLog(context.Background(), runID); err != nil {
		t.Fatalf("DeleteTurnLog (already deleted): %v", err)
	}
	entries, _ := store.ReadTurnLog(context.Background(), runID)
	if entries != nil {
		t.Fatalf("expected nil after delete, got %+v", entries)
	}
}

// --- helpers ------------------------------------------------------------------

func writeLines083(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	writeLinesToPath083(t, path, lines)
	return path
}

func writeLinesToPath083(t *testing.T, path string, lines []string) {
	t.Helper()
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func writeProviderAccountsConfig083(t *testing.T, root string, accounts []ProviderAccount) {
	t.Helper()
	configPath := filepath.Join(root, "provider-accounts.json")
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", configPath)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	payload, err := json.Marshal(providerAccountState{Accounts: accounts})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// writeCodexRolloutLines writes a Codex rollout file whose FIRST line is the
// standard session-meta payload and whose subsequent lines are the provided
// response_item / event_msg entries.
func writeCodexRolloutLines(t *testing.T, home, sessionID, cwd string, ts time.Time, bodyLines []string) string {
	t.Helper()
	dir := filepath.Join(home, "sessions", ts.Format("2006"), ts.Format("01"), ts.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+ts.Format("20060102T150405")+"-"+sessionID+".jsonl")
	content := ""
	for _, l := range bodyLines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}
