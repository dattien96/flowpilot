package runner

import (
	"context"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// Chat handle echo fix: createRun must echo RunKind and ChatID, resumeRun must
// echo ChatID/LegSeq/RunKind. Previously createRun omitted RunKind and resumeRun
// omitted ChatID/LegSeq, so TUI routeProviderSwitch (RunKind==chat && ChatID!= "")
// never fired and /model injected a foreign model into the old adapter (BUG-330).

func TestCreateRunChatHandleEchoesRunKindAndChatID(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun chat: %v", err)
	}
	if handle.RunKind != "chat" {
		t.Fatalf("chat RunKind = %q, want chat", handle.RunKind)
	}
	if matched, _ := regexp.MatchString(`^cht_[0-9a-f]{12}$`, handle.ChatID); !matched {
		t.Fatalf("ChatID = %q, want cht_<12-hex>", handle.ChatID)
	}
	if handle.LegSeq != 0 {
		t.Fatalf("LegSeq = %d, want 0", handle.LegSeq)
	}
}

func TestCreateRunWorkflowHandleHasWorkflowKindNoChat(t *testing.T) {
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4", StepID: "fix"})
	if err != nil {
		t.Fatalf("createRun workflow: %v", err)
	}
	if handle.RunKind != "workflow" {
		t.Fatalf("workflow RunKind = %q, want workflow", handle.RunKind)
	}
	if handle.ChatID != "" {
		t.Fatalf("workflow ChatID = %q, want empty", handle.ChatID)
	}
}

func TestResumeRunEchoesChatIdentity(t *testing.T) {
	// Isolate the provider home: on Windows preferredUserHomeDir reads
	// USERPROFILE (not HOME), and codex resume readiness probes the account
	// home for a rollout + auth — the real machine must never leak in
	// (same class as BUG-312).
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	fws := newFakeWorkflowStore()
	svc1 := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, fws)
	handle, err := svc1.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// createRun persists only a thread-* placeholder; post-hardening codex
	// resume readiness falls through to DiscoverCodexRolloutSessionID plus a
	// local auth check, so seed a rollout + auth.json under the isolated
	// account home before "restarting" (real ids promote via the durable
	// turn log, which a fresh placeholder run does not have).
	st, found, sessErr := fws.GetProviderSession(context.Background(), handle.RunID)
	if sessErr != nil || !found {
		t.Fatalf("GetProviderSession: found=%v err=%v", found, sessErr)
	}
	acctHome := filepath.Join(home, ".codex")
	writeCodexRollout(t, acctHome, "rollout-echo", st.WorkingDirectory, time.Now().UTC())
	mustWriteTestFile(t, filepath.Join(acctHome, "auth.json"), `{"id_token":"tok"}`)
	// New service sharing the same persisted store simulates a restart.
	svc2 := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, fws)
	resumed, err := svc2.resumeRun(handle.RunID)
	if err != nil {
		t.Fatalf("resumeRun: %v", err)
	}
	if resumed.RunKind != "chat" {
		t.Fatalf("resume RunKind = %q, want chat", resumed.RunKind)
	}
	if resumed.ChatID != handle.ChatID {
		t.Fatalf("resume ChatID = %q, want %q", resumed.ChatID, handle.ChatID)
	}
	if resumed.LegSeq != handle.LegSeq {
		t.Fatalf("resume LegSeq = %d, want %d", resumed.LegSeq, handle.LegSeq)
	}
}

// Table over provider targets proves the handle echo is provider-agnostic (R2).
func TestCreateRunChatHandleEchoProviderAgnostic(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok, ProviderKeyOpencode} {
		t.Run(string(pk), func(t *testing.T) {
			t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
			svc, _ := newTestServerWith(t, registryWithFake(pk), nil, nil)
			h, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun %s: %v", pk, err)
			}
			if h.RunKind != "chat" || h.ChatID == "" {
				t.Fatalf("handle %s = RunKind %q ChatID %q", pk, h.RunKind, h.ChatID)
			}
		})
	}
}

func registryWithFake(pk ProviderKey) *ProviderRegistry {
	reg := DefaultProviderRegistry()
	reg.register(ProviderRegistration{
		Key:         pk,
		DisplayName: string(pk),
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true, Interrupt: true},
		newAdapter:  func() ProviderRuntimeAdapter { return newFakeProviderAdapter(pk) },
	})
	return reg
}
