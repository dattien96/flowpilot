package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGrokSessionsCwdDirName pins the workspace-path encoding to the two real
// directory names observed in this machine's ~/.grok/sessions (B1 inspection).
func TestGrokSessionsCwdDirName(t *testing.T) {
	cases := map[string]string{
		`D:\working\gate-sandbox`: "D%3A%5Cworking%5Cgate-sandbox",
		`C:\Users\dat.nguyen`:     "C%3A%5CUsers%5Cdat.nguyen",
	}
	for cwd, want := range cases {
		if got := grokSessionsCwdDirName(cwd); got != want {
			t.Errorf("grokSessionsCwdDirName(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func writeGrokChatHistoryFixture(t *testing.T, grokHome, cwd, sessionID string, lines []string) {
	t.Helper()
	dir := filepath.Join(grokHome, "sessions", grokSessionsCwdDirName(cwd), sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "chat_history.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write chat_history.jsonl: %v", err)
	}
}

// grokFixtureLines is a minimal Claude-shaped Grok chat_history.jsonl covering
// every frame kind the loader handles: system (skip), the <user_info> context
// frame (skip), the real <user_query> prompt, an assistant message with a tool
// call, and the paired tool_result.
func grokFixtureLines() []string {
	return []string{
		`{"type":"system","content":"You are an AI coding assistant."}`,
		`{"type":"user","content":[{"type":"text","text":"<user_info>\nOS: windows\n</user_info>"}]}`,
		`{"type":"user","content":[{"type":"text","text":"<user_query>\nlist the files\n</user_query>"}]}`,
		`{"type":"reasoning","id":"r1","encrypted_content":"xxx","status":"done"}`,
		`{"type":"assistant","content":"Listing files now.","tool_calls":[{"id":"call-1","name":"Glob","arguments":"{\"glob_pattern\":\"**/*\"}"}]}`,
		`{"type":"tool_result","tool_call_id":"call-1","content":"a.txt\nb.txt"}`,
		`{"type":"assistant","content":"There are two files: a.txt and b.txt."}`,
	}
}

func TestLoadGrokTranscriptEvents(t *testing.T) {
	grokHome := t.TempDir()
	cwd := `D:\working\gate-sandbox`
	writeGrokChatHistoryFixture(t, grokHome, cwd, "sess-1", grokFixtureLines())

	events, _ := loadGrokTranscriptEvents(grokChatHistoryPath(grokHome, cwd, "sess-1"))

	var prompts, messages int
	var toolStarted, toolCompleted int
	for _, e := range events {
		switch e.Type {
		case EventTurnStarted:
			if e.Prompt != "list the files" {
				t.Errorf("turn_started prompt = %q, want %q (must be the <user_query> text, not <user_info>)", e.Prompt, "list the files")
			}
			prompts++
		case EventMessageCompleted:
			messages++
		case EventToolStarted:
			if e.ToolName != "Glob" {
				t.Errorf("tool_started name = %q, want Glob", e.ToolName)
			}
			toolStarted++
		case EventToolCompleted:
			toolCompleted++
		}
	}
	if prompts != 1 {
		t.Errorf("prompt events = %d, want 1 (only the <user_query>, never the <user_info> frame)", prompts)
	}
	if messages != 2 {
		t.Errorf("message_completed events = %d, want 2", messages)
	}
	if toolStarted != 1 || toolCompleted != 1 {
		t.Errorf("tool events = (%d started, %d completed), want (1,1)", toolStarted, toolCompleted)
	}
}

func TestDiscoverGrokSessionDirsOrdersByMtime(t *testing.T) {
	grokHome := t.TempDir()
	cwd := `D:\working\gate-sandbox`
	writeGrokChatHistoryFixture(t, grokHome, cwd, "sess-older", []string{`{"type":"system","content":"x"}`})
	writeGrokChatHistoryFixture(t, grokHome, cwd, "sess-newer", []string{`{"type":"system","content":"y"}`})

	// Force sess-newer to have a strictly later mtime than sess-older.
	older := filepath.Join(grokHome, "sessions", grokSessionsCwdDirName(cwd), "sess-older", "chat_history.jsonl")
	newer := filepath.Join(grokHome, "sessions", grokSessionsCwdDirName(cwd), "sess-newer", "chat_history.jsonl")
	base := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, base, base); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	dirs := discoverGrokSessionDirs(grokHome, cwd)
	if len(dirs) != 2 || dirs[0] != "sess-older" || dirs[1] != "sess-newer" {
		t.Fatalf("discoverGrokSessionDirs = %v, want [sess-older sess-newer] (oldest-first)", dirs)
	}
}

// TestSeedGrokTranscriptFromDiskReplaysViaTurnLog proves the end-to-end restart
// path: a resumed Grok run with per-turn session ids in its turn log replays
// each session's chat_history.jsonl, concatenated in log order — the fix for
// "grok mất cả chat sau restart".
func TestSeedGrokTranscriptFromDiskReplaysViaTurnLog(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := `D:\working\gate-sandbox`
	writeGrokChatHistoryFixture(t, grokHome, cwd, "sess-1", grokFixtureLines())
	writeGrokAuthFileFixture(t, grokHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-grok", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	ctx := context.Background()
	if err := store.AppendTurnLog(ctx, "run-grok-1", turnLogLine{Kind: turnLogKindGrokSession, SessionID: "sess-1"}); err != nil {
		t.Fatalf("AppendTurnLog: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id:                "run-grok-1",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-grok-1",
		providerAccountID: "acct-grok",
		workspaceCwd:      cwd,
		runKind:           "chat",
		status:            RunStatusCompleted,
		createdAt:         time.Now().UTC().Format(time.RFC3339Nano),
		resumedFromDisk:   true,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var sawPrompt, sawAssistant bool
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt == "list the files" {
			sawPrompt = true
		}
		if e.Type == EventMessageCompleted {
			sawAssistant = true
		}
	}
	if !sawPrompt {
		t.Fatal("resumed Grok run did not replay the user prompt from chat_history.jsonl")
	}
	if !sawAssistant {
		t.Fatal("resumed Grok run did not replay the assistant message from chat_history.jsonl")
	}
}
