package runner

// CP-81 Task-415 tests: lifecycle HTTP surface + durable stop-all, driven
// through real httptest servers (same convention as cp71_worktree_e2e_test).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/lifecycle"
)

// lifecycleHTTPServer boots a Runner + InteractiveService + lifecycle manager
// on a real mux — the same wiring `runner serve` performs.
func lifecycleHTTPServer(t *testing.T, mode lifecycle.LifecycleMode, workFn func(context.Context) (lifecycle.WorkloadSnapshot, error)) (*Runner, *InteractiveService, *lifecycle.Manager, *httptest.Server) {
	t.Helper()
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	svc := NewInteractiveService()
	mgr := lifecycle.NewManager(lifecycle.Config{
		Mode:           mode,
		HeartbeatTTL:   300 * time.Millisecond,
		BootGrace:      300 * time.Millisecond,
		IdleGrace:      300 * time.Millisecond,
		ReconnectGrace: 5 * time.Second,
		SweepInterval:  time.Hour, // deterministic: evaluate via calls
		WorkSnapshot:   workFn,
	})
	r.AttachLifecycle(mgr)
	svc.AttachLifecycle(mgr)
	mux := http.NewServeMux()
	r.RegisterLifecycleRoutes(mux, LifecycleRouteOptions{})
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { srv.Close(); mgr.Close() })
	return r, svc, mgr, srv
}

func postLifecycleJSON(t *testing.T, srv *httptest.Server, path string, body any) (int, map[string]any) {
	t.Helper()
	payload, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func registerClient(t *testing.T, srv *httptest.Server, kind, instanceID string) map[string]any {
	t.Helper()
	status, body := postLifecycleJSON(t, srv, "/system/clients/register", map[string]any{
		"kind": kind, "clientInstanceId": instanceID, "pid": 4321, "label": kind + "-" + instanceID,
	})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("register %s → %d: %v", instanceID, status, body)
	}
	return body
}

// ── Task-415 acceptance: HTTP round trips ────────────────────────────────────

func TestLifecycleAPI_RegisterHeartbeatReleaseRoundTrip(t *testing.T) {
	_, _, _, srv := lifecycleHTTPServer(t, lifecycle.ModeClientManaged, nil)

	reg := registerClient(t, srv, "tui", "tui-1")
	leaseID, _ := reg["leaseId"].(string) //nolint:errcheck — presence asserted below
	token, _ := reg["leaseToken"].(string)
	if leaseID == "" || token == "" {
		t.Fatalf("register body = %v", reg)
	}
	if reg["generation"].(float64) != 1 {
		t.Fatalf("generation = %v", reg["generation"])
	}

	// Heartbeat extends + returns full snapshot.
	status, snap := postLifecycleJSON(t, srv, "/system/clients/"+leaseID+"/heartbeat", map[string]any{
		"leaseToken": token, "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("heartbeat → %d: %v", status, snap)
	}
	if snap["phase"] != "ready" {
		t.Fatalf("phase = %v", snap["phase"])
	}

	// Release → 200; repeat release idempotent.
	status, _ = postLifecycleJSON(t, srv, "/system/clients/"+leaseID+"/release", map[string]any{
		"leaseToken": token, "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("release → %d", status)
	}
	status, _ = postLifecycleJSON(t, srv, "/system/clients/"+leaseID+"/release", map[string]any{
		"leaseToken": token, "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("repeat release → %d, want 200 (idempotent)", status)
	}
}

func TestLifecycleAPI_LifecycleSnapshotEndpoint(t *testing.T) {
	_, _, _, srv := lifecycleHTTPServer(t, lifecycle.ModeClientManaged, nil)
	reg := registerClient(t, srv, "desktop", "desk-1")
	resp, err := http.Get(srv.URL + "/system/lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var snap map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap["runnerInstanceId"] != reg["runnerInstanceId"] {
		t.Fatalf("instance mismatch: %v vs %v", snap["runnerInstanceId"], reg["runnerInstanceId"])
	}
	clients, _ := snap["clients"].([]any)
	if len(clients) != 1 {
		t.Fatalf("clients = %v", clients)
	}
	if snap["lifecycleMode"] != "client-managed" {
		t.Fatalf("mode = %v", snap["lifecycleMode"])
	}
}

func TestLifecycleAPI_StaleGenerationAndBadTokenAreTyped(t *testing.T) {
	_, _, _, srv := lifecycleHTTPServer(t, lifecycle.ModeClientManaged, nil)
	reg := registerClient(t, srv, "tui", "tui-1")
	leaseID := reg["leaseId"].(string)

	status, body := postLifecycleJSON(t, srv, "/system/clients/"+leaseID+"/heartbeat", map[string]any{
		"leaseToken": reg["leaseToken"], "runnerInstanceId": reg["runnerInstanceId"], "generation": 99,
	})
	if status != http.StatusConflict {
		t.Fatalf("stale gen → %d, want 409: %v", status, body)
	}
	if body["error"].(map[string]any)["code"] != "stale_generation" {
		t.Fatalf("code = %v", body["error"])
	}

	status, body = postLifecycleJSON(t, srv, "/system/clients/"+leaseID+"/heartbeat", map[string]any{
		"leaseToken": "forged", "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
	if status != http.StatusForbidden || body["error"].(map[string]any)["code"] != "invalid_lease_token" {
		t.Fatalf("bad token → %d %v, want 403 invalid_lease_token", status, body)
	}

	status, body = postLifecycleJSON(t, srv, "/system/clients/lease_ghost/heartbeat", map[string]any{
		"leaseToken": "x", "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
	if status != http.StatusNotFound || body["error"].(map[string]any)["code"] != "lease_unknown" {
		t.Fatalf("unknown lease → %d %v, want 404 lease_unknown", status, body)
	}
}

func TestLifecycleAPI_RegisterDuringDrainRejected(t *testing.T) {
	_, _, mgr, srv := lifecycleHTTPServer(t, lifecycle.ModeClientManaged, nil)
	reg := registerClient(t, srv, "tui", "tui-1")

	// Sole-client + idle → direct accept; but use the manager directly to
	// avoid the HTTP drain callback exiting anything (Drain is unset anyway).
	_, err := mgr.RequestShutdown(context.Background(), lifecycle.SystemActionInput{
		RequesterLeaseID:   reg["leaseId"].(string),
		LeaseToken:         reg["leaseToken"].(string),
		ExpectedInstanceID: reg["runnerInstanceId"].(string),
		Reason:             "test",
	})
	if err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	status, body := postLifecycleJSON(t, srv, "/system/clients/register", map[string]any{
		"kind": "desktop", "clientInstanceId": "desk-late",
	})
	if status != http.StatusConflict || body["error"].(map[string]any)["code"] != "runner_draining" {
		t.Fatalf("register during drain → %d %v, want 409 runner_draining", status, body)
	}
}

// ── Busy confirmation flow over HTTP ─────────────────────────────────────────

func TestLifecycleAPI_BusyShutdownRequiresConfirmation(t *testing.T) {
	work := func(context.Context) (lifecycle.WorkloadSnapshot, error) {
		return lifecycle.WorkloadSnapshot{Items: []lifecycle.WorkloadItem{
			{Kind: "turn", RunID: "run-1", Cancellable: true},
		}}, nil
	}
	r, _, _, srv := lifecycleHTTPServer(t, lifecycle.ModeClientManaged, work)
	reg := registerClient(t, srv, "tui", "tui-1")

	// /system/shutdown is served by cli handlers in production (CA-911/913
	// attribution lives there); the negotiation itself is exercised here
	// through the same HandleSystemActionRequest entry point they call.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/system/shutdown", strings.NewReader(mustJSON(map[string]any{
		"requesterLeaseId":   reg["leaseId"],
		"leaseToken":         reg["leaseToken"],
		"expectedInstanceId": reg["runnerInstanceId"],
		"reason":             "user_exit",
	})))
	var drainCalls []LifecycleDrainInput
	r.HandleSystemActionRequest(rec, req, LifecycleRouteOptions{
		Drain: func(in LifecycleDrainInput) { drainCalls = append(drainCalls, in) },
	}, "shutdown", "test-requester")
	if rec.Code != http.StatusConflict {
		t.Fatalf("busy shutdown → %d, want 409", rec.Code)
	}
	var conflict map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&conflict)
	errObj := conflict["error"].(map[string]any)
	if errObj["code"] != "lifecycle_confirmation_required" {
		t.Fatalf("code = %v", errObj["code"])
	}
	token, _ := errObj["confirmToken"].(string)
	if token == "" {
		t.Fatal("missing confirmToken in 409")
	}

	// Confirmed → 202 + drain fires.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/system/shutdown", strings.NewReader(mustJSON(map[string]any{
		"requesterLeaseId":   reg["leaseId"],
		"leaseToken":         reg["leaseToken"],
		"expectedInstanceId": reg["runnerInstanceId"],
		"confirm":            true,
		"confirmToken":       token,
	})))
	r.HandleSystemActionRequest(rec2, req2, LifecycleRouteOptions{
		Drain: func(in LifecycleDrainInput) { drainCalls = append(drainCalls, in) },
	}, "shutdown", "test-requester")
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("confirmed shutdown → %d, want 202: %s", rec2.Code, rec2.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(drainCalls) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(drainCalls) != 1 || drainCalls[0].Action != "shutdown" || drainCalls[0].Requester != "test-requester" {
		t.Fatalf("drain calls = %+v", drainCalls)
	}
}

// ── Inventory + stop-all ─────────────────────────────────────────────────────

func TestLiveWorkSnapshot_CountsActiveTurnsAndScaffolds(t *testing.T) {
	svc := NewInteractiveService()
	// No work → empty inventory.
	snap, err := svc.LiveWorkSnapshot(context.Background())
	if err != nil || snap.ActiveCount() != 0 {
		t.Fatalf("empty inventory = %+v err=%v", snap, err)
	}
	// Arm a scaffold claim + cancel.
	svc.claimScaffold("proj-1")
	svc.armScaffoldCancel("proj-1", func() {})
	snap, _ = svc.LiveWorkSnapshot(context.Background())
	found := false
	for _, it := range snap.Items {
		if it.Kind == "scaffold" && it.ProjectID == "proj-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scaffold missing from inventory: %+v", snap.Items)
	}
	// Release clears it.
	svc.releaseScaffold("proj-1")
	snap, _ = svc.LiveWorkSnapshot(context.Background())
	if snap.ActiveCount() != 0 {
		t.Fatalf("released scaffold still counted: %+v", snap.Items)
	}
}

func TestStopAllForSystemAction_CancelsScaffoldsAndStopsRuns(t *testing.T) {
	svc := NewInteractiveService()
	cancelled := make(chan string, 1)
	svc.claimScaffold("proj-9")
	svc.armScaffoldCancel("proj-9", func() { cancelled <- "proj-9" })

	res, err := svc.StopAllForSystemAction(context.Background(), "test-drain")
	if err != nil {
		t.Fatalf("stop-all: %v", err)
	}
	select {
	case got := <-cancelled:
		if got != "proj-9" {
			t.Fatalf("cancelled %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scaffold cancel not invoked")
	}
	if len(res.CancelledScaffolds) != 1 || res.CancelledScaffolds[0] != "proj-9" {
		t.Fatalf("cancelledScaffolds = %v", res.CancelledScaffolds)
	}
}

func TestDrainingGateRejectsNewWork(t *testing.T) {
	svc := NewInteractiveService()
	mgr := lifecycle.NewManager(lifecycle.Config{
		Mode:          lifecycle.ModeClientManaged,
		SweepInterval: time.Hour,
	})
	t.Cleanup(mgr.Close)
	svc.AttachLifecycle(mgr)

	// Drive the manager into draining directly.
	if _, err := mgr.RequestShutdown(context.Background(), lifecycle.SystemActionInput{Reason: "test"}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	status, body := postLifecycleJSON(t, srv, "/client/workflow-runs", map[string]any{
		"projectId": "p", "chatMode": "normal_chat",
	})
	if status != http.StatusConflict || body["error"].(map[string]any)["code"] != "runner_draining" {
		t.Fatalf("startRun during drain → %d %v, want 409 runner_draining", status, body)
	}
}

func TestLifecycleAPI_ConcurrentLeaseRacesAreAtomic(t *testing.T) {
	_, _, _, srv := lifecycleHTTPServer(t, lifecycle.ModeClientManaged, nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload, _ := json.Marshal(map[string]any{
				"kind": "tui", "clientInstanceId": "inst-" + string(rune('a'+i%4)),
			})
			resp, err := http.Post(srv.URL+"/system/clients/register", "application/json", bytes.NewReader(payload))
			if err == nil {
				resp.Body.Close()
			}
		}(i)
	}
	wg.Wait()
	resp, err := http.Get(srv.URL + "/system/lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var snap map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&snap)
	clients, _ := snap["clients"].([]any)
	if len(clients) == 0 || len(clients) > 4 {
		t.Fatalf("clients = %d, want 1..4 distinct instances", len(clients))
	}
}
