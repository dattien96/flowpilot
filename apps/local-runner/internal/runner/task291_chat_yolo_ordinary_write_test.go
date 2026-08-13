package runner

import (
	"context"
	"testing"
	"time"
)

// Task-291: chat YOLO=on must auto-run ordinary workspace writes; YOLO=off still
// gates; ask_user still prompts. Case 3 — Claude + Codex + Grok each verified.

func TestChatYoloOn_OrdinaryWriteDoesNotPrompt_Claude(t *testing.T) {
	posture := resolveYoloPostureForTurn(true, false)
	if posture.ClaudePermissionMode != "bypassPermissions" || !posture.RunnerAutoApprove {
		t.Fatalf("claude chat YOLO=on posture = %+v, want bypassPermissions + auto-approve", posture)
	}
	args := claudeArgs(posture, "", "", "", "", nil)
	for _, a := range args {
		if a == "--permission-prompt-tool" {
			t.Fatalf("claude YOLO=on must not attach permission-prompt-tool, args=%v", args)
		}
	}
	assertWriteAutoApproved(t, true)
}

func TestChatYoloOn_OrdinaryWriteDoesNotPrompt_Codex(t *testing.T) {
	sandbox, mode := codexYoloDeriveForTurn(true, false)
	if sandbox != "danger-full-access" || mode != "never" {
		t.Fatalf("codex chat YOLO=on derive = (%q,%q), want danger-full-access/never", sandbox, mode)
	}
	assertWriteAutoApproved(t, true)
}

func TestChatYoloOn_OrdinaryWriteDoesNotPrompt_Grok(t *testing.T) {
	posture := resolveYoloPostureForTurn(true, false)
	if posture.GrokPermissionMode != "bypassPermissions" || !posture.RunnerAutoApprove {
		t.Fatalf("grok chat YOLO=on posture = %+v, want bypassPermissions + auto-approve", posture)
	}
	assertWriteAutoApproved(t, true)
}

func TestChatYoloOff_OrdinaryWriteStillGates(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{id: "run-291-off", workspaceCwd: t.TempDir()}
	svc.runs[rs.id] = rs
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-off", yolo: false}

	started := make(chan struct{})
	done := make(chan string, 1)
	go func() {
		close(started)
		d, err := b.RequestApproval(ApprovalDetails{Kind: "write", Command: "write test.txt"})
		if err != nil {
			t.Errorf("RequestApproval: %v", err)
		}
		done <- d
	}()
	<-started
	deadline := time.Now().Add(2 * time.Second)
	var apprID string
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		apprID = rs.pendingApprovalID
		svc.mu.Unlock()
		if apprID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if apprID == "" {
		t.Fatal("YOLO=off ordinary write must emit a pending approval")
	}
	select {
	case d := <-done:
		t.Fatalf("RequestApproval returned %q before human decision", d)
	case <-time.After(40 * time.Millisecond):
	}
}

func TestChatYoloOn_AskUserStillPrompts(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{id: "run-291-q", workspaceCwd: t.TempDir()}
	svc.runs[rs.id] = rs
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-q", yolo: true}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := b.AskQuestion("Need a filename?", []QuestionOption{{Label: "ok", Value: "ok"}}, false)
		done <- err
	}()
	<-started
	deadline := time.Now().Add(2 * time.Second)
	var qid string
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		qid = rs.pendingQuestionID
		svc.mu.Unlock()
		if qid != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if qid == "" {
		t.Fatal("YOLO=on must still surface ask_user as a pending question")
	}
	select {
	case err := <-done:
		t.Fatalf("AskQuestion returned early under YOLO=on: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
}

func TestResolveTurnYolo_ChatToggleOverridesRunDefault(t *testing.T) {
	on := true
	rs := &interactiveRun{yolo: false, runKind: "chat"}
	if got := resolveTurnYolo(rs, TurnInput{YoloMode: &on}); !got {
		t.Fatal("chat turn yoloMode=true must override run yolo=false")
	}
	off := false
	rs.yolo = true
	if got := resolveTurnYolo(rs, TurnInput{YoloMode: &off}); got {
		t.Fatal("chat turn yoloMode=false must override run yolo=true")
	}
}

func assertWriteAutoApproved(t *testing.T, yolo bool) {
	t.Helper()
	svc := NewInteractiveService()
	rs := &interactiveRun{id: "run-291-write", workspaceCwd: t.TempDir()}
	svc.runs[rs.id] = rs
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-w", yolo: yolo}
	decision, err := b.RequestApproval(ApprovalDetails{Kind: "write", Command: "write test.txt"})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if yolo && decision != "approve" {
		t.Fatalf("YOLO=on write decision=%q, want approve", decision)
	}
	if yolo && rs.pendingApprovalID != "" {
		t.Fatalf("YOLO=on write left pending approval %q", rs.pendingApprovalID)
	}
}
