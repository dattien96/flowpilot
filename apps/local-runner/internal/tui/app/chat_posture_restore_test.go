package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TUI restart restore: SessionDefaultsMsg firstLoad must GET the runner's
// persisted Active (SSOT) and apply its profile without PUT (dirty=false).
func TestSessionDefaultsRestoresRunnerActive(t *testing.T) {
	// Runner says active=plan with a pinned provider/model.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"plan","profiles":{"scan":{},"plan":{"provider":"claude","model":"opus"},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	// Before first SessionDefaultsMsg, chatPosture is code (default).
	m.chatPosture = "code"
	m.provider = "codex"
	m.model = "o3"
	m.providers = []client.Provider{
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
	}
	m.sessionDefaultsLoaded = false

	// First SessionDefaultsMsg should schedule a restore load.
	m2, batch := m.Update(SessionDefaultsMsg{Provider: "codex", Model: "o3", Providers: m.providers})
	am := m2.(*AppModel)
	if am.chatPosturePending != "restore" {
		t.Fatalf("first SessionDefaultsMsg must set pending=restore, got %q", am.chatPosturePending)
	}
	if batch == nil {
		t.Fatal("SessionDefaultsMsg firstLoad must return a batch containing restore load")
	}
	// Execute the batch: find the restore load cmd inside. The batch is a tea.Batch
	// that returns a tea.Cmd; we can instead directly call cmdLoadChatPosture
	// which is what the pending restore will use — verify it fetches plan.
	cmd := am.cmdLoadChatPosture()
	msg := cmd()
	cp, ok := msg.(chatPostureMsg)
	if !ok {
		t.Fatalf("expected chatPostureMsg, got %T", msg)
	}
	if cp.Err != nil {
		t.Fatalf("load chat posture: %v", cp.Err)
	}
	if cp.Cfg.Active != "plan" {
		t.Fatalf("loaded active = %q, want plan", cp.Cfg.Active)
	}
	// Apply restore: should set chatPosture=plan, provider=claude, model=opus, no dirty.
	am.chatPostureCmdFromPending(cp.Cfg)
	if am.chatPosture != "plan" {
		t.Fatalf("posture after restore = %q, want plan", am.chatPosture)
	}
	if am.provider != "claude" {
		t.Fatalf("provider after restore = %q, want claude (plan pin)", am.provider)
	}
	if am.model != "opus" {
		t.Fatalf("model after restore = %q, want opus (plan pin)", am.model)
	}
	if am.chatPostureDirty {
		t.Fatal("restore must NOT mark dirty (no PUT)")
	}
	if am.chatPostureCfg.Active != "plan" {
		t.Fatalf("cfg active after restore = %q, want plan", am.chatPostureCfg.Active)
	}
}

func TestSessionDefaultsRestoreHandlesScanWithYolo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"scan","profiles":{"scan":{"provider":"grok","model":"grok-4","yolo":true},"plan":{},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "codex"
	m.providers = []client.Provider{{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4"}}}}
	m.sessionDefaultsLoaded = false

	m2, _ := m.Update(SessionDefaultsMsg{Provider: "codex", Model: "o3", Providers: m.providers})
	am := m2.(*AppModel)
	cmd := am.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	am.chatPostureCmdFromPending(cp.Cfg)
	if am.chatPosture != "scan" {
		t.Fatalf("posture after restore = %q, want scan", am.chatPosture)
	}
	if !am.yolo {
		t.Fatal("yolo after restore should be true (scan pin)")
	}
	if strings.ToLower(am.provider) != "grok" {
		t.Fatalf("provider after restore = %q, want grok", am.provider)
	}
	if am.chatPostureDirty {
		t.Fatal("restore must NOT mark dirty")
	}
}

func TestSlashModeApplyStillPutsActive(t *testing.T) {
	// Verify the user-switch path still PUTs active (the restore path must not).
	putSeen := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"code","profiles":{"scan":{},"plan":{},"code":{}}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/client/chat-posture":
			body := make([]byte, 4096)
			n, _ := r.Body.Read(body)
			putSeen = string(body[:n])
			// Echo the PUT body back as the response so cmdSaveChatPosture can unmarshal.
			w.Header().Set("Content-Type", "application/json")
			if putSeen == "" {
				_, _ = w.Write([]byte(`{"active":"plan","profiles":{"scan":{},"plan":{},"code":{}}}`))
			} else {
				_, _ = w.Write([]byte(putSeen))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"

	m2, cmd := m.handleSlashCommand("/mode plan")
	am := m2.(*AppModel)
	if am.chatPosturePending != "apply:plan" {
		t.Fatalf("pending = %q, want apply:plan", am.chatPosturePending)
	}
	msg := cmd()
	cp := msg.(chatPostureMsg)
	// This will mark dirty and set Active=plan.
	am.chatPostureCmdFromPending(cp.Cfg)
	if !am.chatPostureDirty {
		t.Fatal("apply must mark dirty")
	}
	// Directly exercise the save cmd — it must PUT active=plan.
	saveCmd := am.cmdSaveChatPosture(am.chatPostureCfg)
	saveMsg := saveCmd()
	saveCp, ok := saveMsg.(chatPostureMsg)
	if !ok {
		t.Fatalf("expected chatPostureMsg from save, got %T", saveMsg)
	}
	if saveCp.Err != nil {
		t.Fatalf("save chat posture: %v", saveCp.Err)
	}
	if !strings.Contains(putSeen, `"active":"plan"`) && !strings.Contains(putSeen, `"active": "plan"`) {
		t.Fatalf("PUT body must contain active=plan, got %q", putSeen)
	}
}
