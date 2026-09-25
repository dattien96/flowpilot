package runner

// BUG-487: provider transcript/meta/progress readers used bufio.Scanner with
// a finite cap and never checked scanner.Err() — a line over the cap ended
// the scan exactly like EOF, so the parsed prefix was returned as though it
// were the whole file and resume replay seeded a silently-shortened
// transcript. Contract (same family as BUG-485): reads are uncapped and a
// mid-file read failure surfaces as an error to the caller.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bug487WriteJSONL(t *testing.T, lines ...[]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if _, err := f.Write(append(l, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func bug487Line(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A >cap line that is itself valid JSON — pre-fix the scanner aborts before
// ever seeing it, post-fix it is consumed whole (and maps to no events).
func bug487FatJSONLine(t *testing.T, size int) []byte {
	t.Helper()
	return bug487Line(t, map[string]any{"type": "padding", "data": strings.Repeat("x", size)})
}

func TestBUG487_ClaudeLoaderReadsPastOversizedLine(t *testing.T) {
	prompt := bug487Line(t, map[string]any{"type": "user", "message": map[string]any{"content": "first"}})
	after := bug487Line(t, map[string]any{"type": "user", "message": map[string]any{"content": "after-fat"}})
	path := bug487WriteJSONL(t, prompt, bug487FatJSONLine(t, 5*1024*1024), after)
	events, err := loadClaudeTranscriptEvents(path)
	if err != nil {
		t.Fatalf("oversized-but-readable line must not error: %v", err)
	}
	var sawAfter bool
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt == "after-fat" {
			sawAfter = true
		}
	}
	if !sawAfter {
		t.Fatal("event after the oversized line is missing — file silently truncated")
	}
}

func TestBUG487_CodexLoaderReadsPastOversizedLine(t *testing.T) {
	before := bug487Line(t, map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "before"}}}})
	after := bug487Line(t, map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "after-fat"}}}})
	path := bug487WriteJSONL(t, before, bug487FatJSONLine(t, 5*1024*1024), after)
	events, err := loadCodexTranscriptEvents(path)
	if err != nil {
		t.Fatalf("oversized-but-readable line must not error: %v", err)
	}
	var sawAfter bool
	for _, e := range events {
		if strings.Contains(e.Text, "after-fat") {
			sawAfter = true
		}
	}
	if !sawAfter {
		t.Fatal("event after the oversized line is missing — file silently truncated")
	}
}

func TestBUG487_GrokLoaderReadsPastOversizedLine(t *testing.T) {
	before := bug487Line(t, map[string]any{"type": "assistant", "content": "before"})
	after := bug487Line(t, map[string]any{"type": "assistant", "content": "after-fat"})
	path := bug487WriteJSONL(t, before, bug487FatJSONLine(t, 5*1024*1024), after)
	events, err := loadGrokTranscriptEvents(path)
	if err != nil {
		t.Fatalf("oversized-but-readable line must not error: %v", err)
	}
	var sawAfter bool
	for _, e := range events {
		if e.Text == "after-fat" {
			sawAfter = true
		}
	}
	if !sawAfter {
		t.Fatal("event after the oversized line is missing — file silently truncated")
	}
}

func TestBUG487_LoaderReadErrorSurfaces(t *testing.T) {
	// Opening a directory succeeds on unix; the first read fails — the error
	// must reach the caller instead of looking like an empty transcript.
	dir := t.TempDir()
	for name, load := range map[string]func(string) ([]ProviderEvent, error){
		"claude": loadClaudeTranscriptEvents,
		"codex":  loadCodexTranscriptEvents,
		"grok":   loadGrokTranscriptEvents,
	} {
		if _, err := load(dir); err == nil {
			t.Fatalf("%s loader must surface a read error on an unreadable file", name)
		}
	}
}

func TestBUG487_RolloutMetaOversizedFirstLine(t *testing.T) {
	big := strings.Repeat("y", 200*1024)
	line := bug487Line(t, map[string]any{
		"type": "session_meta",
		"payload": map[string]any{
			"id":        "sess-big-meta",
			"cwd":       "/tmp/ws",
			"timestamp": "2026-09-25T00:00:00Z",
			"blob":      big,
		},
	})
	path := bug487WriteJSONL(t, line)
	meta, ok := readCodexRolloutMeta(path)
	if !ok {
		t.Fatal("rollout meta must parse when the first line exceeds 64KiB")
	}
	if meta.ID != "sess-big-meta" || meta.Cwd != "/tmp/ws" {
		t.Fatalf("meta parsed wrong: %+v", meta)
	}
}

func TestBUG487_ScaffoldTailReadsPastOversizedLine(t *testing.T) {
	good := bug487Line(t, ScaffoldProgressEvent{Seq: 1, Kind: "phase", Phase: "p"})
	fat := bug487Line(t, map[string]any{"seq": 2, "kind": "pad", "text": strings.Repeat("z", 2*1024*1024)})
	tail := bug487Line(t, ScaffoldProgressEvent{Seq: 3, Kind: "phase", Phase: "tail"})
	path := bug487WriteJSONL(t, good, fat, tail)
	events, err := readScaffoldProgressTail(path, 8)
	if err != nil {
		t.Fatalf("oversized-but-readable line must not error: %v", err)
	}
	var sawTail bool
	for _, e := range events {
		if e.Seq == 3 {
			sawTail = true
		}
	}
	if !sawTail {
		t.Fatal("tail event after the oversized line is missing — log silently truncated")
	}
}

func TestBUG487_LoadersHealthyFileNoError(t *testing.T) {
	claude := bug487WriteJSONL(t, bug487Line(t, map[string]any{"type": "user", "message": map[string]any{"content": "hi"}}))
	if _, err := loadClaudeTranscriptEvents(claude); err != nil {
		t.Fatalf("healthy claude file must not error: %v", err)
	}
	grok := bug487WriteJSONL(t, bug487Line(t, map[string]any{"type": "assistant", "content": "hi"}))
	if _, err := loadGrokTranscriptEvents(grok); err != nil {
		t.Fatalf("healthy grok file must not error: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "nope.jsonl")
	if ev, err := loadClaudeTranscriptEvents(missing); err != nil || ev != nil {
		t.Fatalf("missing file: want (nil,nil) open-failure passthrough, got %v %v", ev, err)
	}
}
