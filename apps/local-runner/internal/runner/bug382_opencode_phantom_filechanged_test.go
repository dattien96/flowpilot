package runner

import "testing"

// BUG-382: a denied/failed write must not emit file_changed — nothing was
// written, yet the phantom event poisoned WrittenPaths and tripped
// r-ca/r-contract flow gates. Likewise an in_progress update is progress, not
// a completion: it must not surface as tool_completed.
func TestBug382_OpencodeDeniedWriteEmitsNoFileChanged(t *testing.T) {
	update := map[string]any{
		"status":   "failed",
		"title":    "Edit",
		"kind":     "edit",
		"rawInput": map[string]any{"file_path": "main.go"},
		"rawOutput": map[string]any{
			"error": "The user rejected permission to use this specific tool call.",
		},
	}
	events, mapped := mapOpencodeToolCallUpdate(update)
	if !mapped {
		t.Fatal("terminal failed update should still map")
	}
	var completed, fileChanged int
	for _, ev := range events {
		if ev.Type == EventToolCompleted {
			completed++
			if ev.Status != "failed" {
				t.Fatalf("tool_completed status=%q, want failed", ev.Status)
			}
		}
		if ev.Type == EventFileChanged {
			fileChanged++
		}
	}
	if completed != 1 {
		t.Fatalf("want exactly 1 tool_completed, got %d events=%v", completed, events)
	}
	if fileChanged != 0 {
		t.Fatalf("denied write emitted %d phantom file_changed events", fileChanged)
	}
}

// BUG-382: in_progress updates carry declared paths but nothing was written.
func TestBug382_OpencodeInProgressUpdateEmitsNoCompletion(t *testing.T) {
	update := map[string]any{
		"status":   "in_progress",
		"title":    "Write",
		"kind":     "write",
		"rawInput": map[string]any{"file_path": "main.go"},
	}
	events, _ := mapOpencodeToolCallUpdate(update)
	for _, ev := range events {
		if ev.Type == EventToolCompleted {
			t.Fatalf("in_progress update emitted premature tool_completed: %+v", ev)
		}
		if ev.Type == EventFileChanged {
			t.Fatalf("in_progress update emitted phantom file_changed: %+v", ev)
		}
	}
}

// BUG-382 regression guard: a genuinely completed mutation still emits
// tool_completed + file_changed.
func TestBug382_OpencodeCompletedWriteStillEmitsFileChanged(t *testing.T) {
	update := map[string]any{
		"status":   "completed",
		"title":    "Write",
		"kind":     "write",
		"rawInput": map[string]any{"file_path": "main.go"},
	}
	events, _ := mapOpencodeToolCallUpdate(update)
	var completed, fileChanged int
	for _, ev := range events {
		if ev.Type == EventToolCompleted {
			completed++
		}
		if ev.Type == EventFileChanged {
			fileChanged++
			if ev.Path != "main.go" {
				t.Fatalf("file_changed path=%q want main.go", ev.Path)
			}
		}
	}
	if completed != 1 || fileChanged != 1 {
		t.Fatalf("completed write lost events: completed=%d fileChanged=%d events=%v", completed, fileChanged, events)
	}
}
