package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CP-51 Task-256 parity with the Desktop DispatchAttentionCard + the runner
// dispatch_operator_test.go suite. These mirror the 7 runner §4.2 tests at the
// TUI surface: ListDispatchAttention, InspectDispatch, resolve (uncertain),
// retry-as-new superseded, repair resolution, settle-pending passive details,
// and hydrate-on-open (rebuild after restart). Provider-agnostic but each
// decision test loops claude/codex/grok to prove 3-provider parity.

func attentionFlowModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-64000", Status: "running"}
	m.connStatus = ConnWaiting
	m.statusMsg = "flow running…"
	m.flowLoopStatus = "running"
	return m
}

// dispatchAttentionServer serves the Task-256 per-run routes with a mutable
// attention set, mirroring runner dispatch_operator.go.
func dispatchAttentionServer(items []client.DispatchAttentionItem, inspect *client.DispatchInspectResult) (*httptest.Server, *[]client.DispatchAttentionItem) {
	mu := items
	hit := &mu
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/dispatch-attention"):
			json.NewEncoder(w).Encode(map[string]any{"items": *hit})
		case r.Method == http.MethodGet && strings.Contains(path, "/dispatches/") && !strings.HasSuffix(path, "/audit"):
			if inspect != nil {
				json.NewEncoder(w).Encode(inspect)
				return
			}
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/resolve"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad", 400)
				return
			}
			// On resolve, drop the item (committed settlement) — Desktop refresh parity.
			*hit = nil
			json.NewEncoder(w).Encode(map[string]any{
				"state": "dispatch_terminal_completed", "settlePhase": "settled",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/retry-as-new"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad", 400)
				return
			}
			// T-5 cancel-bias supersede: a stale envelope hash is rejected 409.
			if body["expectedEnvelopeHash"] == "stale-hash-does-not-match" {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"code": "dispatch_retry_superseded"},
				})
				return
			}
			*hit = nil
			json.NewEncoder(w).Encode(map[string]any{
				"state": "dispatch_send_prepared", "settlePhase": "dispatch_begin", "newTurnId": "t2",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/repair-resolution"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad", 400)
				return
			}
			action, _ := body["action"].(string)
			outcome := "resolved_abandon"
			if action == "retry_load" {
				outcome = "resolved_retry_load"
			}
			*hit = nil
			json.NewEncoder(w).Encode(map[string]any{
				"revision": 2, "outcome": outcome, "detail": "ok",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, hit
}

// TestClient_DispatchAttentionWire mirrors runner TestDispatchAttentionHandlers_StatusAndReplay
// at the client layer: list → resolve → attention clears (replay-safe refresh).
func TestClient_DispatchAttentionWire(t *testing.T) {
	srv, _ := dispatchAttentionServer([]client.DispatchAttentionItem{
		{Kind: "uncertain", RunID: "run-64000", TurnID: "t1", Reason: "unknown outcome"},
	}, nil)
	defer srv.Close()
	cl := client.New(srv.URL)

	items, err := cl.ListDispatchAttention(context.Background(), "run-64000")
	if err != nil {
		t.Fatalf("ListDispatchAttention: %v", err)
	}
	if len(items) != 1 || items[0].Kind != "uncertain" || items[0].TurnID != "t1" {
		t.Fatalf("unexpected attention: %+v", items)
	}

	disp, err := cl.ResolveDispatchUncertain(context.Background(), "run-64000", "t1", 3, "res-1", "mark_completed", "")
	if err != nil {
		t.Fatalf("ResolveDispatchUncertain: %v", err)
	}
	// OR ledger row: the response carries the ALREADY-COMMITTED settlement.
	if disp.State == "" || disp.SettlePhase == "" {
		t.Fatalf("resolve response missing settlement disposition: %+v", disp)
	}
	items, err = cl.ListDispatchAttention(context.Background(), "run-64000")
	if err != nil {
		t.Fatalf("re-list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("attention must clear after resolve, got %+v", items)
	}
}

// TestDispatchInspect_RedactsEvidence mirrors runner
// TestDispatchInspect_RedactsCanonicalReceiptEvidence: inspect surfaces only the
// canonical hash, never the raw payload.
func TestDispatchInspect_RedactsEvidence(t *testing.T) {
	srv, _ := dispatchAttentionServer(nil, &client.DispatchInspectResult{
		DispatchSettlement: client.DispatchSettlement{State: "dispatch_send_started", SettlePhase: "recovery_unknown"},
		RunID:              "run-64000",
		TurnID:             "t1",
		Revision:           4,
		CancelRequested:    false,
		EnvelopeHash:       "canonical-hash-only",
		ReceiptEvidence: &client.ReceiptEvidenceSummary{
			ProviderKey: "fake", ReceiptID: "rid-1", EvidenceKind: "ack", PayloadSHA256: "canonical-hash-only",
		},
	})
	defer srv.Close()
	cl := client.New(srv.URL)
	res, err := cl.InspectDispatch(context.Background(), "run-64000", "t1")
	if err != nil {
		t.Fatalf("InspectDispatch: %v", err)
	}
	if res.ReceiptEvidence == nil || res.ReceiptEvidence.PayloadSHA256 != "canonical-hash-only" {
		t.Fatalf("expected canonical hash surfaced, got %+v", res.ReceiptEvidence)
	}
	if res.State == "" || res.SettlePhase == "" {
		t.Fatalf("inspect missing settlement fields: %+v", res)
	}
}

// TestAttentionBanner_ChipsAndNoStop mirrors Desktop DispatchAttentionCard: an
// uncertain item surfaces clickable chips and parks the flow (no [stop], send
// blocked) — never reads as live-running.
func TestAttentionBanner_ChipsAndNoStop(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := attentionFlowModel(pk)
			m.applyAttention([]client.DispatchAttentionItem{
				{Kind: "uncertain", RunID: "run-64000", TurnID: "t1", Reason: "unknown outcome"},
			})
			if !m.hasUnresolvedAttention() {
				t.Fatalf("%s: uncertain item must be unresolved", pk)
			}
			if m.turnIsActive() {
				t.Fatalf("%s: attention-parked flow must not arm [stop]", pk)
			}
			if !m.sendBlocked() {
				t.Fatalf("%s: attention-parked flow must block send", pk)
			}
			if strings.Contains(stripANSI(m.renderStatusLine()), "[stop]") {
				t.Fatalf("%s: attention statusline must not show [stop]:\n%s", pk, m.renderStatusLine())
			}
			bar := stripANSI(m.renderInputLine())
			for _, chip := range []string{"[inspect]", "[confirm-cancelled]", "[mark-completed]", "[mark-failed]", "[retry-as-new]", "[abandon]"} {
				if !strings.Contains(bar, chip) {
					t.Fatalf("%s: attention bar missing %s:\n%s", pk, chip, bar)
				}
			}
			if _, _, ok := findClickTarget(m, "attention-resolve:run-64000/t1:mark_completed"); !ok {
				t.Fatalf("%s: mark-completed chip must be clickable", pk)
			}
			if _, _, ok := findClickTarget(m, "attention-inspect:run-64000/t1"); !ok {
				t.Fatalf("%s: inspect chip must be clickable", pk)
			}
		})
	}
}

// TestAttentionSettlePending_OnlyDetails: settle_pending offers no decision
// chips — just [details] — and does NOT park the flow (passive, task already
// recorded; Desktop onlyPendingSettles parity).
func TestAttentionSettlePending_OnlyDetails(t *testing.T) {
	m := attentionFlowModel("codex")
	m.connStatus = ConnIdle // isolate attention logic from flow-running chrome
	m.applyAttention([]client.DispatchAttentionItem{
		{Kind: "settle_pending", RunID: "run-64000", TurnID: "t1", Reason: "finishing bookkeeping"},
	})
	if m.hasUnresolvedAttention() {
		t.Fatal("settle_pending must not be treated as requiring a decision")
	}
	if m.sendBlocked() {
		t.Fatal("settle_pending must not block send")
	}
	bar := stripANSI(m.renderInputLine())
	if !strings.Contains(bar, "[details]") {
		t.Fatalf("settle_pending must offer [details]:\n%s", bar)
	}
	for _, chip := range []string{"[mark-completed]", "[mark-failed]", "[retry-as-new]", "[abandon]"} {
		if strings.Contains(bar, chip) {
			t.Fatalf("settle_pending must not offer decision chip %s:\n%s", chip, bar)
		}
	}
}

// TestAttentionResolve_DropsItem mirrors Desktop resolve: clicking mark-completed
// POSTs resolve and, after the refresh, the card clears.
func TestAttentionResolve_DropsItem(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			srv, _ := dispatchAttentionServer([]client.DispatchAttentionItem{
				{Kind: "uncertain", RunID: "run-64000", TurnID: "t1"},
			}, nil)
			defer srv.Close()
			m := attentionFlowModel(pk)
			m.runnerURL = srv.URL
			m.applyAttention([]client.DispatchAttentionItem{
				{Kind: "uncertain", RunID: "run-64000", TurnID: "t1"},
			})

			// Click the mark-completed chip → dispatch resolve cmd.
			x, y, ok := findClickTarget(m, "attention-resolve:run-64000/t1:mark_completed")
			if !ok {
				t.Fatalf("%s: mark-completed chip not clickable", pk)
			}
			m2, cmd := m.dispatchMouseClick(x, y)
			_ = m2
			if cmd == nil {
				t.Fatalf("%s: click produced no cmd", pk)
			}
			msg := cmd()
			rm, ok := msg.(AttentionResolvedMsg)
			if !ok {
				t.Fatalf("%s: got %T, want AttentionResolvedMsg", pk, msg)
			}
			if rm.Err != nil {
				t.Fatalf("%s: resolve err: %v", pk, rm.Err)
			}
			if rm.Outcome != "mark_completed" {
				t.Fatalf("%s: outcome=%q want mark_completed", pk, rm.Outcome)
			}
			// Feeding the resolved msg triggers an attention refresh that clears.
			m3, cmd := m.Update(msg)
			am := m3.(*AppModel)
			if cmd == nil {
				t.Fatalf("%s: resolve handler should schedule attention refresh", pk)
			}
			refresh := cmd()
			if _, ok := refresh.(AttentionLoadedMsg); !ok {
				t.Fatalf("%s: refresh cmd returned %T", pk, refresh)
			}
			m4, _ := am.Update(refresh)
			am = m4.(*AppModel)
			if am.hasUnresolvedAttention() {
				t.Fatalf("%s: attention must clear after resolve, got %+v", pk, am.attention)
			}
		})
	}
}

// TestAttentionRetry_Superseded409 mirrors runner
// TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance: a 409
// dispatch_retry_superseded must surface an error (never clear the item).
func TestAttentionRetry_Superseded409(t *testing.T) {
	m := attentionFlowModel("codex")
	srv, _ := dispatchAttentionServer([]client.DispatchAttentionItem{
		{Kind: "uncertain", RunID: "run-64000", TurnID: "t1"},
	}, &client.DispatchInspectResult{
		DispatchSettlement: client.DispatchSettlement{State: "dispatch_send_started", SettlePhase: "recovery_unknown"},
		RunID:              "run-64000", TurnID: "t1", Revision: 4, CancelRequested: false,
		EnvelopeHash: "stale-hash-does-not-match",
	})
	defer srv.Close()
	m.runnerURL = srv.URL
	m.applyAttention([]client.DispatchAttentionItem{
		{Kind: "uncertain", RunID: "run-64000", TurnID: "t1"},
	})
	m.attentionInspect = map[string]*client.DispatchInspectResult{
		attentionKey("run-64000", "t1"): {DispatchSettlement: client.DispatchSettlement{State: "s", SettlePhase: "p"}, RunID: "run-64000", TurnID: "t1", Revision: 4, EnvelopeHash: "stale-hash-does-not-match"},
	}

	// First click arms the confirm (cancel-bias), second click retries.
	x, y, ok := findClickTarget(m, "attention-retry:run-64000/t1")
	if !ok {
		t.Fatalf("retry chip not clickable")
	}
	m2, cmd := m.dispatchMouseClick(x, y)
	if cmd != nil {
		t.Fatalf("first retry click must only arm confirm, no cmd")
	}
	if !m2.(*AppModel).attentionRetryConfirm[attentionKey("run-64000", "t1")] {
		t.Fatalf("first click must arm retry confirm")
	}
	x, y, ok = findClickTarget(m2.(*AppModel), "attention-retry:run-64000/t1")
	if !ok {
		t.Fatalf("confirm-retry chip not clickable")
	}
	m3, cmd := m2.(*AppModel).dispatchMouseClick(x, y)
	_ = m3
	if cmd == nil {
		t.Fatalf("confirm click must dispatch retry cmd")
	}
	msg := cmd()
	rm, ok := msg.(AttentionResolvedMsg)
	if !ok {
		t.Fatalf("got %T, want AttentionResolvedMsg", msg)
	}
	if rm.Err == nil {
		t.Fatal("superseded retry must surface an error")
	}
	var apiErr *client.APIError
	if !errors.As(rm.Err, &apiErr) || apiErr.Code != "dispatch_retry_superseded" {
		t.Fatalf("expected dispatch_retry_superseded, got %v", rm.Err)
	}
}

// TestAttentionRepair_AbandonAndRetryLoad mirrors runner
// TestRepairResolutionHandler_TwoPhaseAndAbandon + _RetryLoadRestoresSession:
// repair_required offers retry-load / abandon and both resolve + clear.
func TestAttentionRepair_AbandonAndRetryLoad(t *testing.T) {
	for _, action := range []string{"abandon", "retry_load"} {
		t.Run(action, func(t *testing.T) {
			srv, _ := dispatchAttentionServer([]client.DispatchAttentionItem{
				{Kind: "repair_required", RunID: "run-64000", Reason: "corrupt runtime blob"},
			}, nil)
			defer srv.Close()
			m := attentionFlowModel("codex")
			m.runnerURL = srv.URL
			m.applyAttention([]client.DispatchAttentionItem{
				{Kind: "repair_required", RunID: "run-64000", Reason: "corrupt runtime blob"},
			})
			bar := stripANSI(m.renderInputLine())
			if !strings.Contains(bar, "[retry-load]") || !strings.Contains(bar, "[abandon-repair]") {
				t.Fatalf("repair bar missing chips:\n%s", bar)
			}
			chip := "attention-repair:run-64000:abandon"
			if action == "retry_load" {
				chip = "attention-repair:run-64000:retry_load"
			}
			x, y, ok := findClickTarget(m, chip)
			if !ok {
				t.Fatalf("%s chip not clickable", action)
			}
			m2, cmd := m.dispatchMouseClick(x, y)
			_ = m2
			if cmd == nil {
				t.Fatalf("%s: click produced no cmd", action)
			}
			msg := cmd()
			rm, ok := msg.(AttentionResolvedMsg)
			if !ok {
				t.Fatalf("%s: got %T, want AttentionResolvedMsg", action, msg)
			}
			if rm.Err != nil {
				t.Fatalf("%s: err %v", action, rm.Err)
			}
			if rm.Kind != "repair_required" || rm.Outcome != action {
				t.Fatalf("%s: resolved %s/%s", action, rm.Kind, rm.Outcome)
			}
		})
	}
}

// TestAttentionHydrate_OnOpen mirrors runner TestDispatchAttention_RebuildsAfterRestart
// at the TUI surface: after /open (cmdHydrateDispatchAttention), the attention set
// is rebuilt from durable truth and surfaces the banner + chips.
func TestAttentionHydrate_OnOpen(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			srv, _ := dispatchAttentionServer([]client.DispatchAttentionItem{
				{Kind: "uncertain", RunID: "run-64000", TurnID: "t1"},
			}, nil)
			defer srv.Close()
			m := attentionFlowModel(pk)
			m.runnerURL = srv.URL
			cmd := m.cmdHydrateDispatchAttention("run-64000")
			if cmd == nil {
				t.Fatalf("%s: hydrate returned nil cmd", pk)
			}
			msg := cmd()
			lm, ok := msg.(AttentionLoadedMsg)
			if !ok {
				t.Fatalf("%s: got %T, want AttentionLoadedMsg", pk, msg)
			}
			if lm.Err != nil {
				t.Fatalf("%s: hydrate err %v", pk, lm.Err)
			}
			m2, _ := m.Update(lm)
			am := m2.(*AppModel)
			if len(am.attention) != 1 || am.attention[0].Kind != "uncertain" {
				t.Fatalf("%s: attention not rebuilt: %+v", pk, am.attention)
			}
			if strings.Contains(stripANSI(am.renderStatusLine()), "[stop]") {
				t.Fatalf("%s: hydrated attention must not arm [stop]", pk)
			}
			bar := stripANSI(am.renderInputLine())
			if !strings.Contains(bar, "[mark-completed]") {
				t.Fatalf("%s: hydrated bar missing decision chips:\n%s", pk, bar)
			}
		})
	}
}

// TestAttention_NonAttentionRunIgnored: attention for a different run is ignored
// (stale-run guard) so a late refresh cannot surface the wrong run's card.
func TestAttention_NonAttentionRunIgnored(t *testing.T) {
	m := attentionFlowModel("codex")
	msg := AttentionLoadedMsg{RunID: "run-OTHER", Items: []client.DispatchAttentionItem{
		{Kind: "uncertain", RunID: "run-OTHER", TurnID: "t9"},
	}}
	m2, _ := m.Update(msg)
	if m2.(*AppModel).hasUnresolvedAttention() {
		t.Fatalf("attention for another run must be ignored")
	}
	if len(m2.(*AppModel).attention) != 0 {
		t.Fatalf("attention for another run must not be stored: %+v", m2.(*AppModel).attention)
	}
}
