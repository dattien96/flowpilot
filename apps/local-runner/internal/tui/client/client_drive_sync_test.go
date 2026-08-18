package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// G1 (CA-548): the TUI client had no surface for the runner's Drive chat-session
// sync/restore endpoints (Desktop Navigator parity). These tests lock the three
// new methods — SyncChatRun, ListRemoteChatSessions, RestoreChatRun — and the
// sync metadata fields on RunHistoryItem.

func TestRunHistoryItem_DecodesDriveSyncMetadata(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/projects/p-1/workflow-runs" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"runId":"r1","projectId":"p-1","providerKey":"codex","status":"completed","sourceMachineId":"mch_a","sourceRunId":"r1","syncStatus":"synced"},{"runId":"r2","projectId":"p-1","providerKey":"grok","status":"completed","syncStatus":"failed","unavailableReason":"drive not connected"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	items, err := c.ListRunHistory(t.Context(), "p-1")
	if err != nil {
		t.Fatalf("ListRunHistory: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%+v", items)
	}
	if items[0].SyncStatus != "synced" || items[0].SourceMachineID != "mch_a" || items[0].SourceRunID != "r1" {
		t.Fatalf("row0=%+v", items[0])
	}
	if items[1].SyncStatus != "failed" || items[1].UnavailableReason != "drive not connected" {
		t.Fatalf("row1=%+v", items[1])
	}
}

func TestSyncChatRun_PostsProjectID(t *testing.T) {
	var mu sync.Mutex
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"runId":"r1","sourceMachineId":"mch_a","sourceRunId":"r1","syncStatus":"synced","syncedAt":"2026-08-18T00:00:00Z","remotePath":"chat-sessions/runs/mch_a/r1/manifest.json"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.SyncChatRun(t.Context(), "r1", ChatSessionSyncRequest{GoogleDriveProjectID: "p-1"})
	if err != nil {
		t.Fatalf("SyncChatRun: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/client/workflow-runs/r1/sync-chat" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotBody["googleDriveProjectId"] != "p-1" {
		t.Fatalf("body=%+v", gotBody)
	}
	if res.SyncStatus != "synced" || res.RemotePath == "" {
		t.Fatalf("res=%+v", res)
	}
}

func TestListRemoteChatSessions_PathAndDecode(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/projects/p-1/chat-sessions/remote" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"runId":"rx","projectId":"p-1","providerKey":"claude","sourceMachineId":"mch_b","sourceRunId":"r9","lastPrompt":"ship","syncedAt":"2026-08-18T00:00:00Z"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	items, err := c.ListRemoteChatSessions(t.Context(), "p-1")
	if err != nil {
		t.Fatalf("ListRemoteChatSessions: %v", err)
	}
	if len(items) != 1 || items[0].SourceMachineID != "mch_b" || items[0].SourceRunID != "r9" || items[0].ProviderKey != "claude" {
		t.Fatalf("items=%+v", items)
	}
}

func TestRestoreChatRun_PostsRequest(t *testing.T) {
	var mu sync.Mutex
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"runId":"r-new","sourceMachineId":"mch_b","sourceRunId":"r9","providerKey":"claude","restoreStatus":"restored"}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.RestoreChatRun(t.Context(), ChatSessionRestoreRequest{
		ProjectID: "p-1", SourceMachineID: "mch_b", SourceRunID: "r9", Cwd: "C:\\work",
	})
	if err != nil {
		t.Fatalf("RestoreChatRun: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/client/chat-sessions/restore" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotBody["projectId"] != "p-1" || gotBody["sourceMachineId"] != "mch_b" || gotBody["sourceRunId"] != "r9" || gotBody["cwd"] != "C:\\work" {
		t.Fatalf("body=%+v", gotBody)
	}
	if res.RestoreStatus != "restored" || res.RunID != "r-new" {
		t.Fatalf("res=%+v", res)
	}
}

func TestSyncChatRun_ErrorSurfacesCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"google_drive_not_connected","message":"google drive is not connected for this project"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.SyncChatRun(t.Context(), "r1", ChatSessionSyncRequest{})
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if ae.Code != "google_drive_not_connected" {
		t.Fatalf("code=%q", ae.Code)
	}
	if !strings.Contains(ae.Error(), "google drive is not connected") {
		t.Fatalf("err=%v", ae)
	}
}
