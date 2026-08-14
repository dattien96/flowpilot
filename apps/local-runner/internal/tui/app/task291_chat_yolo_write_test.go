package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestChatYoloOn_PermissionRequiredDoesNotShowApprovalCard(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", Yolo: true}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.yolo = true

	m2, cmd := m.handleEvent(client.ProviderEvent{
		Type:          "permission_required",
		ApprovalID:    "ap-291",
		WorkflowRunID: "run-291",
	})
	am := m2.(*AppModel)
	if am.approval != nil {
		t.Fatal("YOLO=on must not mount an approval card for ordinary write")
	}
	if strings.Contains(am.View(), "[APPROVAL]") {
		t.Fatalf("YOLO=on leaked approval prompt:\n%s", am.View())
	}
	if cmd == nil {
		t.Fatal("expected silent auto-approve cmd")
	}
}

func TestChatYoloOff_PermissionRequiredShowsApprovalCard(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", Yolo: false}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.yolo = false

	m2, cmd := m.handleEvent(client.ProviderEvent{
		Type:          "permission_required",
		ApprovalID:    "ap-291-off",
		WorkflowRunID: "run-291",
	})
	am := m2.(*AppModel)
	if am.approval == nil || am.approval.ID != "ap-291-off" {
		t.Fatalf("YOLO=off approval=%+v", am.approval)
	}
	if !strings.Contains(am.View(), "[APPROVAL]") {
		t.Fatalf("YOLO=off missing approval prompt:\n%s", am.View())
	}
	if cmd != nil {
		t.Fatal("YOLO=off must wait for the operator, not auto-approve")
	}
}

func TestChatYoloOn_BuildTurnInputSendsYoloTrue(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.yolo = true
	in := m.buildTurnInput("write test.txt with content test")
	if in.YoloMode == nil || !*in.YoloMode {
		t.Fatalf("YoloMode=%v, want pointer to true", in.YoloMode)
	}
}

func TestChatYoloOn_GrokStartupAppliesExistingPostureEndpoint(t *testing.T) {
	var got *bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider-accounts/grok-yolo-posture" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var body struct {
			Yolo bool `json:"yolo"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		got = &body.Yolo
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: true}, srv.URL)
	cmd := m.startupGrokYoloPostureCmd()
	if cmd == nil {
		t.Fatal("--yolo + grok must call ApplyGrokYoloPosture (same endpoint as /yolo)")
	}
	if msg := cmd(); msg != nil {
		if err, ok := msg.(ErrMsg); ok {
			t.Fatalf("ApplyGrokYoloPosture: %v", err.Err)
		}
		t.Fatalf("unexpected msg %#v", msg)
	}
	if got == nil || !*got {
		t.Fatalf("posture yolo=%v, want true", got)
	}
}

func TestChatYoloOn_NonGrokStartupDoesNotCallGrokPosture(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", Yolo: true}, "http://127.0.0.1:4317")
	if cmd := m.startupGrokYoloPostureCmd(); cmd != nil {
		t.Fatal("non-Grok --yolo must not hit grok-yolo-posture")
	}
}

func TestChatYoloOff_GrokStartupDoesNotCallGrokPosture(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	if cmd := m.startupGrokYoloPostureCmd(); cmd != nil {
		t.Fatal("YOLO=off must not apply Grok always-approve posture")
	}
}
