package app

// Task-437: /wt-merge — the TUI worktree merge-resolution surface. Covers
// arg validation, the resolve path wiring, the 409 requiresConfirm hint, the
// fail-closed conflict render, and badge refresh on success.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func lastMsg(m *AppModel) string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1].Content
}

func TestTUI_WtMergeRequiresRun(t *testing.T) {
	m := &AppModel{}
	if cmd := m.cmdWorktreeMerge([]string{"discard"}); cmd != nil {
		t.Fatal("no runHandle → cmd must be nil")
	}
	if !strings.Contains(lastMsg(m), "No active run") {
		t.Fatalf("msg=%q", lastMsg(m))
	}
}

func TestTUI_WtMergeRejectsBadMode(t *testing.T) {
	m := &AppModel{runHandle: &client.RunHandle{RunID: "run-1"}}
	if cmd := m.cmdWorktreeMerge([]string{"bogus"}); cmd != nil {
		t.Fatal("bad mode must not reach the wire")
	}
	if !strings.Contains(lastMsg(m), "Unknown mode bogus") {
		t.Fatalf("msg=%q", lastMsg(m))
	}
}

func TestTUI_WtMergeResolvesAndUpdatesBadge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/worktree/resolve") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"worktreeState": "merged", "applied": true,
		})
	}))
	defer srv.Close()
	m := &AppModel{runHandle: &client.RunHandle{RunID: "run-1"}, runnerURL: srv.URL}

	cmd := m.cmdWorktreeMerge([]string{"apply_patch"})
	msg, ok := cmd().(worktreeMergeMsg)
	if !ok || msg.Err != nil || msg.Res == nil {
		t.Fatalf("msg=%+v", msg)
	}
	m.handleWorktreeMergeMsg(msg)
	if m.liveWorktree != "merged" {
		t.Fatalf("liveWorktree=%q want merged", m.liveWorktree)
	}
	if !strings.Contains(lastMsg(m), "merged") || !strings.Contains(lastMsg(m), "patch applied") {
		t.Fatalf("msg=%q", lastMsg(m))
	}
}

func TestTUI_WtMergeNoArgProbesBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("no-arg probe must GET, got %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runId": "run-1", "status": "waiting",
			"worktree": map[string]any{"state": "merge_pending", "path": "/wt/x", "slug": "fp-chat-x"},
		})
	}))
	defer srv.Close()
	m := &AppModel{runHandle: &client.RunHandle{RunID: "run-1"}, runnerURL: srv.URL}

	msg, ok := m.cmdWorktreeMerge(nil)().(worktreeMergeMsg)
	if !ok || msg.Snap == nil {
		t.Fatalf("msg=%+v", msg)
	}
	m.handleWorktreeMergeMsg(msg)
	if m.liveWorktree != "merge_pending" {
		t.Fatalf("liveWorktree=%q", m.liveWorktree)
	}
	if !strings.Contains(lastMsg(m), "merge_pending") {
		t.Fatalf("msg=%q", lastMsg(m))
	}
}

func TestTUI_WtMergeConfirmHintListsFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requiresConfirm": true, "uncommitted": []string{"a.txt", "b.txt"},
			"worktreeState": "active",
		})
	}))
	defer srv.Close()
	m := &AppModel{runHandle: &client.RunHandle{RunID: "run-1"}, runnerURL: srv.URL}

	msg := m.cmdWorktreeMerge([]string{"keep_branch"})().(worktreeMergeMsg)
	m.handleWorktreeMergeMsg(msg)
	out := lastMsg(m)
	for _, want := range []string{"a.txt", "b.txt", "--confirm", "keep_branch"} {
		if !strings.Contains(out, want) {
			t.Fatalf("confirm hint missing %q: %q", want, out)
		}
	}
}

func TestTUI_WtMergeConflictFailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		// Wire-truth: conflict evidence body has no code field.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conflict": true, "conflictPaths": []string{"CONFLICT.txt"},
			"patchArtifactRef": "C:/tmp/x.patch", "worktreeState": "merge_pending",
		})
	}))
	defer srv.Close()
	m := &AppModel{runHandle: &client.RunHandle{RunID: "run-1"}, runnerURL: srv.URL}

	msg := m.cmdWorktreeMerge([]string{"apply_patch"})().(worktreeMergeMsg)
	m.handleWorktreeMergeMsg(msg)
	out := lastMsg(m)
	for _, want := range []string{"CONFLICT.txt", "x.patch", "Merge conflict"} {
		if !strings.Contains(out, want) {
			t.Fatalf("conflict render missing %q: %q", want, out)
		}
	}
}

func TestTUI_WtMergeConfirmFlagReachesWire(t *testing.T) {
	var gotConfirm bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm bool `json:"confirm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotConfirm = body.Confirm
		_ = json.NewEncoder(w).Encode(map[string]any{"worktreeState": "kept_branch"})
	}))
	defer srv.Close()
	m := &AppModel{runHandle: &client.RunHandle{RunID: "run-1"}, runnerURL: srv.URL}

	msg := m.cmdWorktreeMerge([]string{"keep_branch", "--confirm"})().(worktreeMergeMsg)
	if msg.Err != nil {
		t.Fatalf("err=%v", msg.Err)
	}
	if !gotConfirm {
		t.Fatal("--confirm must send confirm:true on the wire")
	}
}
