package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/config"
)

func TestCmdShutdownAndQuit_KillsReusedRunner(t *testing.T) {
	gotShutdown := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/system/shutdown" {
			select {
			case gotShutdown <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"accepted"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{OwnsRunner: false}, srv.URL)
	cmd := m.cmdShutdownAndQuit()
	msg := cmd()
	if _, ok := msg.(QuitMsg); !ok {
		t.Fatalf("got %T, want QuitMsg", msg)
	}
	select {
	case <-gotShutdown:
	case <-time.After(2 * time.Second):
		t.Fatal("reused runner (OwnsRunner=false) must still POST /system/shutdown")
	}
}

func TestCmdShutdownAndQuit_KillsOwnedRunner(t *testing.T) {
	gotShutdown := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/system/shutdown" {
			select {
			case gotShutdown <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"accepted"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{OwnsRunner: true}, srv.URL)
	cmd := m.cmdShutdownAndQuit()
	msg := cmd()
	if _, ok := msg.(QuitMsg); !ok {
		t.Fatalf("got %T, want QuitMsg", msg)
	}
	select {
	case <-gotShutdown:
	case <-time.After(2 * time.Second):
		t.Fatal("TUI-spawned runner (OwnsRunner=true) must POST /system/shutdown")
	}
}
