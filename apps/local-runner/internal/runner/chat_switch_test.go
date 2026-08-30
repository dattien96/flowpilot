package runner

// Chat provider switch tests (Task-314): guards, three-phase happy path over
// fake adapters, envelope build/budget, E-9 exactly-once, crash heal. New
// file — additive only.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

// newSwitchTestService builds a service whose registry has codex (fake) plus a
// second AVAILABLE fake provider (claude) so cross-provider switches can run on
// fakes without credentials (matrix row codex→claude).
func newSwitchTestService(t *testing.T) (*InteractiveService, *chatTranscriptWriter) {
	t.Helper()
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	reg := DefaultProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true, Interrupt: true},
		newAdapter:   func() ProviderRuntimeAdapter { return newFakeProviderAdapter(ProviderKeyClaude) },
	})
	svc, _ := newTestServerWith(t, reg, nil, nil)
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("chat transcript writer nil with flag on")
	}
	return svc, writer
}

func postSwitch(t *testing.T, svc *InteractiveService, chatID string, body chatSwitchRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/client/chats/"+chatID+"/switch-provider", strings.NewReader(string(raw)))
	req.SetPathValue("chatId", chatID)
	rec := httptest.NewRecorder()
	svc.handleChatSwitchProvider(rec, req)
	return rec
}

func TestSwitchRouteFlagGated(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "0")
	svc, _ := newCaptureTestService(t)
	t.Setenv("FLOWPILOT_CHAT_SSOT", "0") // capture helper forces it on; re-off
	rec := postSwitch(t, svc, "cht_x", chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "chat_ssot_disabled") {
		t.Fatalf("flag-off status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSwitchUnknownChat404(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	rec := postSwitch(t, svc, "cht_unknown", chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "chat_not_found") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSwitchSameProvider409(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyCodex})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "handoff_same_provider") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	// No new leg minted.
	svc.mu.Lock()
	count := 0
	for _, rs := range svc.runs {
		if rs.chatID == handle.ChatID {
			count++
		}
	}
	svc.mu.Unlock()
	if count != 1 {
		t.Fatalf("legs after same-provider = %d, want 1", count)
	}
}

func TestSwitchBusyDuringTurn(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[handle.RunID].turnInFlight = true
	svc.mu.Unlock()
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "handoff_run_busy") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSwitchTargetProviderUnavailableTyped(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// grok is not registered in the default test registry → typed 422 before
	// any mutation (CS-14 runner half).
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyGrok})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "provider_unavailable") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	intent := rs.switchFromRunID
	state := rs.legState
	svc.mu.Unlock()
	if intent != "" || state != LegStateActive {
		t.Fatalf("mutation on refusal: intent=%q state=%q", intent, state)
	}
}

func TestSwitchHappyPathMintsLegClosesSourceAppendsRecordOnce(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// Seed the transcript: two turns + a tool action (envelope material).
	payload := func(m map[string]any) json.RawMessage { b, _ := json.Marshal(m); return b }
	if err := writer.append(context.Background(),
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload(map[string]any{"prompt": "hello ban la model gi"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatMessageCompleted, Payload: payload(map[string]any{"text": "toi la Muse Spark"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload(map[string]any{"prompt": "fix rounding"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatToolStarted, Payload: payload(map[string]any{"tool": "bash"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatFileChanged, Payload: payload(map[string]any{"path": "src/calc.go"})},
	); err != nil {
		t.Fatal(err)
	}

	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude, Model: "claude-sonnet-x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatSwitchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ChatID != handle.ChatID || resp.LegSeq != 1 || resp.Model != "claude-sonnet-x" {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Handle.ProviderKey != ProviderKeyClaude {
		t.Fatalf("new leg provider = %q", resp.Handle.ProviderKey)
	}
	if resp.Handoff.Mode != "raw" || resp.Handoff.IncludedTurnCount != 2 || !resp.Handoff.ActionsDigest {
		t.Fatalf("handoff stats = %+v", resp.Handoff)
	}

	// Source leg closed with provider_switch; new leg active on claude.
	svc.mu.Lock()
	src := svc.runs[handle.RunID]
	newLeg := svc.runs[resp.Handle.RunID]
	srcState, srcReason := src.legState, src.legClosedReason
	newState := newLeg.legState
	svc.mu.Unlock()
	if srcState != LegStateClosed || srcReason != LegClosedReasonProviderSwitch {
		t.Fatalf("source leg = %s/%s", srcState, srcReason)
	}
	if newState != LegStateActive {
		t.Fatalf("new leg state = %q", newState)
	}

	// E-9 appended exactly once with the from/to pair.
	records, rerr := writer.store.ReadChatRecords(context.Background(), handle.ChatID, 0, 0)
	if rerr != nil {
		t.Fatal(rerr)
	}
	switches := 0
	var switchRec ChatTranscriptRecord
	for _, r := range records {
		if r.Type == EventTypeChatProviderSwitch {
			switches++
			switchRec = r
		}
	}
	if switches != 1 {
		t.Fatalf("chat_provider_switch count = %d, want 1", switches)
	}
	var p map[string]any
	if err := json.Unmarshal(switchRec.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p["fromRunId"] != handle.RunID || p["toRunId"] != resp.Handle.RunID || p["fromProvider"] != "codex" || p["toProvider"] != "claude" {
		t.Fatalf("switch payload = %v", p)
	}
	// The seed turn was dispatched on the new leg (fire-and-return: at least
	// the launch event is present). Envelope CONTENT is asserted
	// deterministically at the builder level below — the async turn_started
	// event carries the prompt only after the turn pipeline attaches it.
	svc.mu.Lock()
	newEvents := svc.runs[resp.Handle.RunID].events
	svc.mu.Unlock()
	if len(newEvents) == 0 {
		t.Fatalf("seed turn not dispatched: %d events on new leg", len(newEvents))
	}
	// Deterministic envelope verification: full-chat turns + prefix + digest.
	svc.mu.Lock()
	srcRun := svc.runs[handle.RunID]
	svc.mu.Unlock()
	env := svc.buildChatHandoffContext(context.Background(), handle.ChatID, srcRun, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude, Model: "claude-sonnet-x"})
	if !strings.Contains(env.Prompt, handoffPromptPrefix) {
		t.Fatalf("envelope missing prefix")
	}
	if !strings.Contains(env.Prompt, "hello ban la model gi") || !strings.Contains(env.Prompt, "fix rounding") {
		t.Fatalf("envelope missing prior turns: %.200s", env.Prompt)
	}
	if !strings.Contains(env.Prompt, "<actions_summary>") || !strings.Contains(env.Prompt, "tool bash") || !strings.Contains(env.Prompt, "file src/calc.go") {
		t.Fatalf("envelope missing actions digest: %.300s", env.Prompt)
	}
}

func TestSwitchFreshStartNoEnvelope(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatSwitchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Handoff.Mode != "fresh_start" || resp.Handoff.IncludedTurnCount != 0 {
		t.Fatalf("fresh_start stats = %+v", resp.Handoff)
	}
}

func TestChatCrashTwoActiveHeals(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	// Simulate the crash window: phase B committed (new active leg stamped with
	// switchFromRunID=old.id), phase C never closed the old leg.
	old := &interactiveRun{id: "run-old", runKind: "chat", chatID: "cht_h", legSeq: 0, legState: LegStateActive, switchFromRunID: "run-old", providerKey: ProviderKeyCodex}
	newLeg := &interactiveRun{id: "run-new", runKind: "chat", chatID: "cht_h", legSeq: 1, legState: LegStateActive, switchFromRunID: "run-old", providerKey: ProviderKeyClaude}
	svc.runs[old.id] = old
	svc.runs[newLeg.id] = newLeg
	svc.chatRuns.register(old.id, "cht_h")
	svc.chatRuns.register(newLeg.id, "cht_h")

	svc.mu.Lock()
	svc.healChatLegsLocked("cht_h")
	svc.mu.Unlock()

	if old.legState != LegStateClosed || old.legClosedReason != LegClosedReasonProviderSwitch {
		t.Fatalf("old leg not closed: %s/%s", old.legState, old.legClosedReason)
	}
	if newLeg.legState != LegStateActive {
		t.Fatalf("new leg must stay active: %s", newLeg.legState)
	}
	records, rerr := writer.store.ReadChatRecords(context.Background(), "cht_h", 0, 0)
	if rerr != nil {
		t.Fatal(rerr)
	}
	switches := 0
	for _, r := range records {
		if r.Type == EventTypeChatProviderSwitch {
			switches++
		}
	}
	if switches != 1 {
		t.Fatalf("heal switch records = %d, want 1", switches)
	}
}

func TestChatCrashOrphanIntentCleared(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	lone := &interactiveRun{id: "run-lone", runKind: "chat", chatID: "cht_i", legSeq: 0, legState: LegStateActive, switchFromRunID: "run-lone"}
	svc.runs[lone.id] = lone
	svc.chatRuns.register(lone.id, "cht_i")
	svc.mu.Lock()
	svc.healChatLegsLocked("cht_i")
	svc.mu.Unlock()
	if lone.switchFromRunID != "" || lone.legState != LegStateActive {
		t.Fatalf("orphan intent not cleared: intent=%q state=%s", lone.switchFromRunID, lone.legState)
	}
}

func TestChatHandoffBudgetFloorAndCap(t *testing.T) {
	if got := chatHandoffBudget(0, "m"); got != handoffMaxBytes {
		t.Fatalf("unknown context budget = %d, want %d", got, handoffMaxBytes)
	}
	if got := chatHandoffBudget(200_000, "m"); got != chatSwitchHardCapBytes {
		t.Fatalf("200k budget = %d, want hard cap %d", got, chatSwitchHardCapBytes)
	}
	if got := chatHandoffBudget(8_000, "m"); got != 24_000 {
		t.Fatalf("8k budget = %d, want 24000", got)
	}
}

func TestChatEnvelopeFreshStartNoError(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	svc.mu.Lock()
	src := svc.runs[handle.RunID]
	svc.mu.Unlock()
	env := svc.buildChatHandoffContext(context.Background(), handle.ChatID, src, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if env.Stats.Mode != "fresh_start" || env.Prompt != "" {
		t.Fatalf("fresh_start envelope = %+v", env)
	}
}

// TestSwitchNeverHoldsLockAcrossCreateRun is the SD26-S-2 lock-rule canary
// (Task-314 DOD-11): the switch must complete without deadlocking AND s.mu
// must be acquirable from outside during phase B (envelope/createRun/seed run
// unlocked). A held-across-createRun bug would hang the switch past the
// watchdog or starve the probe for the whole window.
func TestSwitchNeverHoldsLockAcrossCreateRun(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"prompt": "hello"})
	if err := writer.append(context.Background(), ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload}); err != nil {
		t.Fatal(err)
	}

	prevDelay := fakeAdapterDelay
	fakeAdapterDelay = 120 * time.Millisecond // widen the phase-B window
	defer func() { fakeAdapterDelay = prevDelay }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
		if rec.Code != http.StatusOK {
			t.Errorf("switch status = %d body=%s", rec.Code, rec.Body.String())
		}
	}()

	probeAcquired := false
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-done:
			if !probeAcquired {
				t.Fatal("s.mu was never acquirable during the switch — lock likely held across createRun")
			}
			return // completed without deadlock, probe saw the unlock window
		case <-deadline:
			t.Fatal("switch did not complete within 15s — deadlock suspected (s.mu held across createRun?)")
		default:
		}
		if svc.mu.TryLock() {
			svc.mu.Unlock()
			probeAcquired = true
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSwitchCrashHealsOnLoad covers the closed-no-record crash window
// (review C-3): phase C's close landed but the E-9 record write was lost —
// heal appends it exactly once without touching the active new leg.
func TestSwitchCrashHealsOnLoad(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	closedSrc := &interactiveRun{id: "run-src", runKind: "chat", chatID: "cht_k", legSeq: 0, legState: LegStateClosed, legClosedReason: LegClosedReasonProviderSwitch, switchFromRunID: "run-src", providerKey: ProviderKeyCodex}
	newLeg := &interactiveRun{id: "run-new", runKind: "chat", chatID: "cht_k", legSeq: 1, legState: LegStateActive, switchFromRunID: "run-src", providerKey: ProviderKeyClaude}
	svc.runs[closedSrc.id] = closedSrc
	svc.runs[newLeg.id] = newLeg
	svc.chatRuns.register(closedSrc.id, "cht_k")
	svc.chatRuns.register(newLeg.id, "cht_k")

	svc.mu.Lock()
	svc.healChatLegsLocked("cht_k")
	svc.mu.Unlock()

	if newLeg.legState != LegStateActive {
		t.Fatalf("new leg disturbed: %s", newLeg.legState)
	}
	count := func() int {
		records, rerr := writer.store.ReadChatRecords(context.Background(), "cht_k", 0, 0)
		if rerr != nil {
			t.Fatal(rerr)
		}
		n := 0
		for _, r := range records {
			if r.Type == EventTypeChatProviderSwitch {
				n++
			}
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("heal record count = %d, want 1", n)
	}
	// Heal re-entry is idempotent (appendChatSwitchRecordOnce).
	svc.mu.Lock()
	svc.healChatLegsLocked("cht_k")
	svc.mu.Unlock()
	if n := count(); n != 1 {
		t.Fatalf("heal re-entry duplicated the record: %d", n)
	}
}

// TestSwitchSeedFailedRecordEmitted pins SD26-X-7: a seed failure leaves a
// queryable switch_seed_failed record and the leg stays continuable.
func TestSwitchSeedFailedRecordEmitted(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	svc.markSwitchSeedFailed("cht_s", "run-seed", context.DeadlineExceeded)
	records, rerr := writer.store.ReadChatRecords(context.Background(), "cht_s", 0, 0)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(records) != 1 || records[0].Type != EventTypeChatSeedFailed {
		t.Fatalf("seed-fail records = %+v", records)
	}
	var p map[string]any
	if err := json.Unmarshal(records[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p["runId"] != "run-seed" {
		t.Fatalf("payload = %v", p)
	}
}

// ---- guard variants: approval / question / in-flight / detached ----------

func TestSwitchBusyDuringApproval(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[handle.RunID].pendingApprovalID = "appr-1"
	svc.mu.Unlock()
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "handoff_run_busy") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSwitchBusyDuringQuestion(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[handle.RunID].pendingQuestionID = "q-1"
	svc.mu.Unlock()
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "handoff_run_busy") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSwitchInFlightDoubleCall409(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.chatSwitchInFlight = map[string]bool{handle.ChatID: true}
	svc.mu.Unlock()
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "handoff_run_busy") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSwitchDetachedChatNoActiveLeg409(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	// Restored chat: legs exist but all closed (detach policy, SD26 §10).
	closed := &interactiveRun{id: "run-old", runKind: "chat", chatID: "cht_det", legSeq: 0, legState: LegStateClosed, legClosedReason: LegClosedReasonRestored, providerKey: ProviderKeyCodex}
	svc.runs[closed.id] = closed
	svc.chatRuns.register(closed.id, "cht_det")
	rec := postSwitch(t, svc, "cht_det", chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "chat_no_active_leg") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSwitchModelStampsNewLeg pins the model propagation: the response echoes
// the requested model AND the new leg's run carries it (I-R6 server half).
func TestSwitchModelStampsNewLeg(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude, Model: "claude-sonnet-x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp chatSwitchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	newLeg := svc.runs[resp.Handle.RunID]
	svc.mu.Unlock()
	if newLeg == nil || newLeg.modelName != "claude-sonnet-x" {
		t.Fatalf("new leg model = %+v", newLeg)
	}
	if resp.Model != "claude-sonnet-x" {
		t.Fatalf("response model = %q", resp.Model)
	}
}

// TestSwitchChainThreeProvidersMultiLeg exercises the multi-leg future: three
// switches build three ordered legs with two E-9 records, and the second
// envelope carries turns from BOTH prior legs (chat-scoped, not run-scoped).
func TestSwitchChainThreeProvidersMultiLeg(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	svc.registry.register(ProviderRegistration{
		Key: ProviderKeyGrok, DisplayName: "Grok", Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return newFakeProviderAdapter(ProviderKeyGrok) },
	})
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	payload := func(m map[string]any) json.RawMessage { b, _ := json.Marshal(m); return b }
	if err := writer.append(context.Background(),
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload(map[string]any{"prompt": "turn on codex"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatMessageCompleted, Payload: payload(map[string]any{"text": "codex reply"})},
	); err != nil {
		t.Fatal(err)
	}
	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude, Model: "claude-sonnet-x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("switch 1 = %d body=%s", rec.Code, rec.Body.String())
	}
	var leg2 chatSwitchResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &leg2)
	// The seed turn streams fire-and-return; the busy guard correctly blocks a
	// switch while it runs — wait for leg 2 to go idle first.
	waitLegIdle(t, svc, leg2.Handle.RunID)
	// Give leg 2 its own turn in the transcript.
	if err := writer.append(context.Background(),
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: leg2.Handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload(map[string]any{"prompt": "turn on claude"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: leg2.Handle.RunID, Type: EventTypeChatMessageCompleted, Payload: payload(map[string]any{"text": "claude reply"})},
	); err != nil {
		t.Fatal(err)
	}
	rec = postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyGrok, Model: "grok-4.5"})
	if rec.Code != http.StatusOK {
		t.Fatalf("switch 2 = %d body=%s", rec.Code, rec.Body.String())
	}
	var leg3 chatSwitchResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &leg3)

	// Three legs, one chatId, ordered.
	svc.mu.Lock()
	var seqs []int
	var dump string
	for _, rs := range svc.runs {
		if rs.chatID == handle.ChatID && rs.legState == LegStateActive {
			dump += fmt.Sprintf("{%s seq=%d switchFrom=%q} ", rs.id, rs.legSeq, rs.switchFromRunID)
		}
		if rs.chatID == handle.ChatID {
			seqs = append(seqs, rs.legSeq)
		}
	}
	svc.mu.Unlock()
	sort.Ints(seqs)
	if len(seqs) != 3 || seqs[0] != 0 || seqs[1] != 1 || seqs[2] != 2 {
		t.Fatalf("legs = %v activeDump=%s", seqs, dump)
	}
	// Exactly one active leg — the newest (grok). Its residual switchFromRunID
	// is the durable provenance pointer; heal ignores it because the E-9
	// record for the pair already exists.
	if n := strings.Count(dump, "{"); n != 1 {
		t.Fatalf("expected exactly one active leg after chain, got %d: %s", n, dump)
	}
	if !strings.Contains(dump, leg3.Handle.RunID) || !strings.Contains(dump, "seq=2") {
		t.Fatalf("active leg is not the newest: %s", dump)
	}
	// Exactly two E-9 records; the second envelope spans both prior legs.
	records, rerr := writer.store.ReadChatRecords(context.Background(), handle.ChatID, 0, 0)
	if rerr != nil {
		t.Fatal(rerr)
	}
	switches := 0
	for _, r := range records {
		if r.Type == EventTypeChatProviderSwitch {
			switches++
		}
	}
	if switches != 2 {
		t.Fatalf("E-9 count = %d, want 2", switches)
	}
	svc.mu.Lock()
	src := svc.runs[handle.RunID]
	svc.mu.Unlock()
	env := svc.buildChatHandoffContext(context.Background(), handle.ChatID, src, chatSwitchRequest{TargetProviderKey: ProviderKeyOpencode})
	for _, want := range []string{"turn on codex", "codex reply", "turn on claude", "claude reply"} {
		if !strings.Contains(env.Prompt, want) {
			t.Fatalf("chain envelope missing %q", want)
		}
	}
	for _, notWant := range []string{EventTypeChatProviderSwitch} {
		if strings.Contains(env.Prompt, notWant) {
			t.Fatalf("switch records leaked into conversation body")
		}
	}
}

// waitLegIdle blocks until the run's turnInFlight clears (seed turns are
// fire-and-return; the busy guard correctly serializes switches behind them).
func waitLegIdle(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		busy := svc.runs[runID] != nil && svc.runs[runID].turnInFlight
		svc.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("leg %s still busy after 15s", runID)
}
