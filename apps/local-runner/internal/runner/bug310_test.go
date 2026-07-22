package runner

// BUG-310: Drive sync always failed for Grok chats.
//
// Root cause: LocateSessionFile intentionally returns the session DIRECTORY
// for Grok (it stores chat_history.jsonl plus sidecar files there), unlike
// Codex/Claude which return a single transcript file directly. resolveChatSessionTranscript
// called os.ReadFile directly on whatever LocateSessionFile returned, so every
// Grok sync attempt tried to read a directory as a file -- confirmed live via
// a direct API call against a running dev server: "read
// .../.grok/sessions/<encoded-cwd>/<id>: Incorrect function." (Windows'
// ERROR_INVALID_FUNCTION for ReadFile on a directory handle; other platforms
// would see EISDIR instead, so this was never just a Windows-only symptom).

import (
	"testing"
)

// TestResolveChatSessionTranscriptReadsGrokChatHistoryFile reproduces BUG-310.
//
// Expected (pre-fix): apiErr != nil, wrapping the directory-read error.
// Expected (post-fix): the actual chat_history.jsonl bytes come back cleanly.
func TestResolveChatSessionTranscriptReadsGrokChatHistoryFile(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	svc := newInteractiveService(registry, catalog, newFakeWorkflowStore())

	grokHome := t.TempDir()
	const cwd = `D:\working\gate-sandbox`
	const sessionID = "019f8722-c5fc-7f11-8d9e-4154bf38d338"
	const transcript = `{"type":"assistant","content":"hello from grok"}` + "\n"
	writeGrokSessionTree(t, grokHome, cwd, sessionID, map[string]string{"chat_history.jsonl": transcript})

	session := ProviderSessionState{
		RunID:             "run-grok-sync",
		ProjectID:         "proj-grok",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: sessionID,
		WorkingDirectory:  cwd,
		RunKind:           "chat",
	}

	providerFile, body, resolvedID, hasTranscript, apiErr := svc.resolveChatSessionTranscript(session, grokHome)
	if apiErr != nil {
		t.Fatalf("BUG-310: resolveChatSessionTranscript failed reading Grok session dir as a file: %v", apiErr)
	}
	if !hasTranscript {
		t.Fatal("BUG-310: expected hasTranscript=true, got false")
	}
	if resolvedID != sessionID {
		t.Errorf("resolvedID = %q, want %q", resolvedID, sessionID)
	}
	if string(body) != transcript {
		t.Errorf("body = %q, want %q", body, transcript)
	}
	if providerFile.RelativePath == "" {
		t.Fatal("expected a non-empty RelativePath")
	}
	if got := providerFile.RelativePath[len(providerFile.RelativePath)-len("chat_history.jsonl"):]; got != "chat_history.jsonl" {
		t.Errorf("RelativePath = %q, want it to end in chat_history.jsonl (a file, not the session directory)", providerFile.RelativePath)
	}
	if providerFile.SHA256 != hashBytesSHA256([]byte(transcript)) {
		t.Errorf("SHA256 does not match the transcript body")
	}
}
