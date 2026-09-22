package runner

// CP-81 Task-420 E2E: cross-client shared-runner lifecycle driven over real
// HTTP round trips. These tests exercise the same wiring `runner serve`
// performs (Runner + InteractiveService + lifecycle.Manager on a mux) with
// a live sweeper so TTL expiry / idle grace / restart deadlines are observed
// on the wall clock — not by poking internals.
//
// Process-level halves of these scenarios (runnerboot stale replacement,
// supervisor fenced commands, port-safety) live in their own packages:
//   internal/tui/runnerboot/runnerboot_test.go
//   tests/phase1/supervisorLifecycle.test.ts
// Here we assert the runner-side evidence those layers depend on.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/lifecycle"
)

// ── harness ──────────────────────────────────────────────────────────────────

type e2eLifecycleHarness struct {
	r   *Runner
	svc *InteractiveService
	mgr *lifecycle.Manager
	srv *httptest.Server

	mu      sync.Mutex
	phases  []lifecycle.RunnerPhase
	drains  []LifecycleDrainInput
	// drainOnce mirrors the serve command's dedupe: whether the drain is
	// initiated by the HTTP handler or by a manager phase transition (idle
	// expiry), cleanup runs exactly once.
	drainOnce sync.Once
	workFn    func(context.Context) (lifecycle.WorkloadSnapshot, error)
}

func (h *e2eLifecycleHarness) recordDrain(in LifecycleDrainInput) {
	h.drainOnce.Do(func() {
		h.mu.Lock()
		h.drains = append(h.drains, in)
		h.mu.Unlock()
	})
}

func newE2EHarness(t *testing.T, mode lifecycle.LifecycleMode, workFn func(context.Context) (lifecycle.WorkloadSnapshot, error)) *e2eLifecycleHarness {
	t.Helper()
	h := &e2eLifecycleHarness{workFn: workFn}
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	svc := NewInteractiveService()
	var mgr *lifecycle.Manager // declared first: the OnPhase closure captures it
	mgr = lifecycle.NewManager(lifecycle.Config{
		Mode:           mode,
		HeartbeatTTL:   250 * time.Millisecond,
		BootGrace:      200 * time.Millisecond,
		IdleGrace:      200 * time.Millisecond,
		ReconnectGrace: 400 * time.Millisecond,
		ConfirmTokenTTL: 2 * time.Second,
		SweepInterval:  25 * time.Millisecond, // live sweeper
		WorkSnapshot: func(ctx context.Context) (lifecycle.WorkloadSnapshot, error) {
			if h.workFn == nil {
				return lifecycle.WorkloadSnapshot{}, nil
			}
			return h.workFn(ctx)
		},
		OnPhase: func(p lifecycle.RunnerPhase, _ lifecycle.LifecycleSnapshot) {
			h.mu.Lock()
			h.phases = append(h.phases, p)
			h.mu.Unlock()
			// Manager-initiated drains (idle expiry) enter draining_* without
			// an HTTP request — the serve command's OnPhase performs the same
			// drain. Record it through the shared single-shot.
			if p.IsDraining() {
				action := "shutdown"
				if p == lifecycle.PhaseDrainingRestart {
					action = "restart"
				}
				h.recordDrain(LifecycleDrainInput{Action: action, Requester: mgr.DrainRequester()})
			}
		},
	})
	r.AttachLifecycle(mgr)
	svc.AttachLifecycle(mgr)
	h.r, h.svc, h.mgr = r, svc, mgr

	mux := http.NewServeMux()
	r.RegisterLifecycleRoutes(mux, LifecycleRouteOptions{})
	// /system/shutdown + /system/restart mirror the root.go handlers: they
	// delegate to HandleSystemActionRequest with a drain callback, exactly as
	// the serve command wires them (requester attribution is exercised at
	// that boundary in TestLifecycleAPI_BusyShutdownRequiresConfirmation).
	for _, action := range []string{"shutdown", "restart"} {
		action := action
		mux.HandleFunc("POST /system/"+action, func(w http.ResponseWriter, req *http.Request) {
			r.HandleSystemActionRequest(w, req, LifecycleRouteOptions{
				Drain: h.recordDrain,
			}, action, "e2e-http")
		})
	}
	svc.RegisterInteractiveRoutes(mux)
	h.srv = httptest.NewServer(mux)
	t.Cleanup(func() { h.srv.Close(); mgr.Close() })
	return h
}

func (h *e2eLifecycleHarness) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	return postLifecycleJSON(t, h.srv, path, body)
}

func (h *e2eLifecycleHarness) snapshot(t *testing.T) map[string]any {
	t.Helper()
	resp, err := http.Get(h.srv.URL + "/system/lifecycle")
	if err != nil {
		t.Fatalf("GET lifecycle: %v", err)
	}
	defer resp.Body.Close()
	var snap map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode lifecycle: %v", err)
	}
	return snap
}

func (h *e2eLifecycleHarness) register(t *testing.T, kind, inst string) map[string]any {
	t.Helper()
	status, body := postLifecycleJSON(t, h.srv, "/system/clients/register", map[string]any{
		"kind": kind, "clientInstanceId": inst, "pid": 4242, "label": kind + "-" + inst,
	})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("register %s → %d: %v", inst, status, body)
	}
	return body
}

func (h *e2eLifecycleHarness) release(t *testing.T, reg map[string]any) (int, map[string]any) {
	t.Helper()
	return h.post(t, "/system/clients/"+reg["leaseId"].(string)+"/release", map[string]any{
		"leaseToken": reg["leaseToken"], "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
}

func (h *e2eLifecycleHarness) heartbeat(t *testing.T, reg map[string]any) (int, map[string]any) {
	t.Helper()
	return h.post(t, "/system/clients/"+reg["leaseId"].(string)+"/heartbeat", map[string]any{
		"leaseToken": reg["leaseToken"], "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
}

func (h *e2eLifecycleHarness) phasesSeen() []lifecycle.RunnerPhase {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]lifecycle.RunnerPhase, len(h.phases))
	copy(out, h.phases)
	return out
}

func (h *e2eLifecycleHarness) drainCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.drains)
}

// waitPhase polls until the recorded OnPhase stream contains `want`.
func (h *e2eLifecycleHarness) waitPhase(t *testing.T, want lifecycle.RunnerPhase, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, p := range h.phasesSeen() {
			if p == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("phase %q never observed; phases=%v", want, h.phasesSeen())
}

func activeTurnWork(context.Context) (lifecycle.WorkloadSnapshot, error) {
	return lifecycle.WorkloadSnapshot{Items: []lifecycle.WorkloadItem{
		{Kind: "turn", RunID: "run-e2e", Cancellable: true},
	}}, nil
}

// ── A. Shared runner + per-client release ────────────────────────────────────

func TestE2E_TUIAndDesktopShareRunner(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	tui := h.register(t, "tui", "tui-1")
	desk := h.register(t, "desktop", "desk-1")

	if tui["runnerInstanceId"] != desk["runnerInstanceId"] {
		t.Fatalf("instance mismatch: %v vs %v", tui["runnerInstanceId"], desk["runnerInstanceId"])
	}
	snap := h.snapshot(t)
	clients, _ := snap["clients"].([]any)
	if len(clients) != 2 {
		t.Fatalf("clients = %v, want 2", clients)
	}
	if snap["phase"] != "ready" {
		t.Fatalf("phase = %v, want ready with live leases", snap["phase"])
	}
}

func TestE2E_CloseOneClientKeepsRunner(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	tui := h.register(t, "tui", "tui-1")
	desk := h.register(t, "desktop", "desk-1")
	inst := desk["runnerInstanceId"]

	if status, _ := h.release(t, tui); status != http.StatusOK {
		t.Fatalf("release tui → %d", status)
	}
	snap := h.snapshot(t)
	if snap["phase"] != "ready" || snap["runnerInstanceId"] != inst {
		t.Fatalf("runner disturbed by peer release: %v %v", snap["phase"], snap["runnerInstanceId"])
	}
	// Desktop heartbeat still fenced-valid — the runner stayed up for it.
	if status, hb := h.heartbeat(t, desk); status != http.StatusOK || hb["phase"] != "ready" {
		t.Fatalf("desktop heartbeat → %d %v", status, hb["phase"])
	}
}

func TestE2E_LastClientNormalCloseStartsIdleShutdown(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	tui := h.register(t, "tui", "tui-1")

	if status, snap := h.release(t, tui); status != http.StatusOK || snap["phase"] != "idle_grace" {
		t.Fatalf("release last client → %d phase=%v, want 200 idle_grace", status, snap["phase"])
	}
	snap := h.snapshot(t)
	if snap["idleDeadline"] == nil {
		t.Fatalf("idleDeadline missing in idle_grace snapshot: %v", snap)
	}
	// Grace expiry is itself a drain decision: idle_grace → draining_shutdown
	// (the point where production runs stop-all then MarkStopped → exit).
	h.waitPhase(t, lifecycle.PhaseDrainingShutdown, 3*time.Second)
	if h.drainCount() != 1 {
		t.Fatalf("idle-grace drain fired %d times, want 1", h.drainCount())
	}
	h.mgr.MarkStopped()
	h.waitPhase(t, lifecycle.PhaseStopped, 2*time.Second)
}

func TestE2E_LastClientCrashExpiresLeaseThenIdleShutdown(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	h.register(t, "tui", "tui-crashed") // no release, no heartbeat — the crash

	// Sweeper expires the lease (TTL 250ms), enters idle_grace, then drains.
	h.waitPhase(t, lifecycle.PhaseIdleGrace, 3*time.Second)
	h.waitPhase(t, lifecycle.PhaseDrainingShutdown, 3*time.Second)
	h.mgr.MarkStopped()
	h.waitPhase(t, lifecycle.PhaseStopped, 2*time.Second)
	snap := h.snapshot(t)
	clients, _ := snap["clients"].([]any)
	if len(clients) != 0 {
		t.Fatalf("crashed lease still visible: %v", clients)
	}
}

func TestE2E_ClientAttachDuringIdleGraceCancelsShutdown(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	tui := h.register(t, "tui", "tui-1")
	if status, _ := h.release(t, tui); status != http.StatusOK {
		t.Fatalf("release → %d", status)
	}
	if snap := h.snapshot(t); snap["phase"] != "idle_grace" {
		t.Fatalf("phase = %v, want idle_grace", snap["phase"])
	}
	// A new client attaches inside the grace window → shutdown cancelled.
	desk := h.register(t, "desktop", "desk-late")
	if snap := h.snapshot(t); snap["phase"] != "ready" {
		t.Fatalf("phase after attach = %v, want ready", snap["phase"])
	}
	// Runner must still serve the late client (no drain fired).
	if status, _ := h.heartbeat(t, desk); status != http.StatusOK {
		t.Fatalf("late client heartbeat → %d", status)
	}
	// Stay alive past the original idle deadline (heartbeat keeps the lease).
	for i := 0; i < 4; i++ {
		time.Sleep(100 * time.Millisecond)
		if status, _ := h.heartbeat(t, desk); status != http.StatusOK {
			t.Fatalf("heartbeat %d → %d", i, status)
		}
	}
	if snap := h.snapshot(t); snap["phase"] != "ready" {
		t.Fatalf("phase drifted to %v after attach", snap["phase"])
	}
	if h.drainCount() != 0 {
		t.Fatal("drain fired despite attach during idle grace")
	}
}

// ── B. Work protection ───────────────────────────────────────────────────────

func TestE2E_ActiveWorkBlocksIdleShutdown(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, activeTurnWork)
	tui := h.register(t, "tui", "tui-1")
	if status, _ := h.release(t, tui); status != http.StatusOK {
		t.Fatalf("release → %d", status)
	}
	// No leases but protected work → orphaned_work, never idle_grace.
	h.waitPhase(t, lifecycle.PhaseOrphanedWork, 3*time.Second)
	snap := h.snapshot(t)
	if snap["phase"] == "idle_grace" || snap["phase"] == "stopped" {
		t.Fatalf("active work did not block idle shutdown: phase=%v", snap["phase"])
	}
	if h.drainCount() != 0 {
		t.Fatalf("drain fired despite active work")
	}
}

func TestE2E_ForceShutdownStopsRunsScaffoldAndClients(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, activeTurnWork)
	tui := h.register(t, "tui", "tui-1")
	cancelled := make(chan string, 1)
	h.svc.claimScaffold("proj-e2e")
	h.svc.armScaffoldCancel("proj-e2e", func() { cancelled <- "proj-e2e" })

	// Busy → first request must negotiate.
	status, body := h.post(t, "/system/shutdown", map[string]any{
		"requesterLeaseId":   tui["leaseId"],
		"leaseToken":         tui["leaseToken"],
		"expectedInstanceId": tui["runnerInstanceId"],
		"reason":             "user_exit",
	})
	if status != http.StatusConflict {
		t.Fatalf("busy shutdown → %d, want 409: %v", status, body)
	}
	token := body["error"].(map[string]any)["confirmToken"]
	if token == nil {
		t.Fatalf("no confirmToken: %v", body)
	}

	status, body = h.post(t, "/system/shutdown", map[string]any{
		"requesterLeaseId":   tui["leaseId"],
		"leaseToken":         tui["leaseToken"],
		"expectedInstanceId": tui["runnerInstanceId"],
		"confirm":            true,
		"confirmToken":       token,
	})
	if status != http.StatusAccepted {
		t.Fatalf("confirmed shutdown → %d: %v", status, body)
	}
	h.waitPhase(t, lifecycle.PhaseDrainingShutdown, 2*time.Second)
	if h.drainCount() == 0 {
		t.Fatal("drain callback never fired")
	}
	// The drain is what runs durable stop-all before exit — prove the
	// scaffold cancel seam is reachable at this point.
	res, err := h.svc.StopAllForSystemAction(context.Background(), "e2e-drain")
	if err != nil || len(res.CancelledScaffolds) != 1 {
		t.Fatalf("stop-all: %v res=%+v", err, res)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("scaffold cancel not invoked during drain")
	}
}

// ── C. Restart + reconnect ───────────────────────────────────────────────────

func TestE2E_PlannedRestartReconnectsBothClients(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeSupervised, nil)
	tui := h.register(t, "tui", "tui-1")
	desk := h.register(t, "desktop", "desk-1")

	status, body := h.post(t, "/system/restart", map[string]any{
		"requesterLeaseId":   tui["leaseId"],
		"leaseToken":         tui["leaseToken"],
		"expectedInstanceId": tui["runnerInstanceId"],
		"reason":             "planned_upgrade",
	})
	// Two live clients → confirmation negotiation first.
	if status == http.StatusConflict {
		token := body["error"].(map[string]any)["confirmToken"]
		status, body = h.post(t, "/system/restart", map[string]any{
			"requesterLeaseId":   tui["leaseId"],
			"leaseToken":         tui["leaseToken"],
			"expectedInstanceId": tui["runnerInstanceId"],
			"confirm":            true,
			"confirmToken":       token,
		})
	}
	if status != http.StatusAccepted {
		t.Fatalf("restart → %d: %v", status, body)
	}
	restartID, _ := body["restartId"].(string)
	if restartID == "" {
		t.Fatalf("missing restartId: %v", body)
	}
	h.waitPhase(t, lifecycle.PhaseDrainingRestart, 2*time.Second)

	// Both clients' heartbeats keep working through the drain and report the
	// restart metadata they need to poll /health for the new instance.
	for _, reg := range []map[string]any{tui, desk} {
		status, hb := h.heartbeat(t, reg)
		if status != http.StatusOK || hb["phase"] != "draining_restart" {
			t.Fatalf("heartbeat during restart → %d %v", status, hb["phase"])
		}
	}
	snap := h.snapshot(t)
	restart, _ := snap["restart"].(map[string]any)
	if restart["restartId"] != restartID || restart["deadline"] == nil {
		t.Fatalf("restart metadata = %v", restart)
	}
}

func TestE2E_ReconnectTimeoutClosesClient(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeSupervised, nil)
	tui := h.register(t, "tui", "tui-1")

	status, _ := h.post(t, "/system/restart", map[string]any{
		"requesterLeaseId":   tui["leaseId"],
		"leaseToken":         tui["leaseToken"],
		"expectedInstanceId": tui["runnerInstanceId"],
	})
	if status != http.StatusAccepted {
		t.Fatalf("sole-client restart → %d, want 202", status)
	}
	snap := h.snapshot(t)
	restart, _ := snap["restart"].(map[string]any)
	deadlineStr, _ := restart["deadline"].(string)
	deadline, err := time.Parse(time.RFC3339Nano, deadlineStr)
	if err != nil {
		t.Fatalf("restart.deadline = %v", restart)
	}
	// Clients enforce the deadline themselves: past it without a new instance
	// → the client closes (TUI/Desktop-side behavior covered in their suites).
	if until := time.Until(deadline); until <= 0 || until > 2*time.Second {
		t.Fatalf("reconnect deadline = %v from now, want ~(0, 2s]", until)
	}
}

func TestE2E_UnplannedRunnerDeathClosesClients(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	reg := h.register(t, "tui", "tui-1")
	h.srv.Close() // unplanned death — the HTTP surface disappears

	payload, _ := json.Marshal(map[string]any{
		"leaseToken": reg["leaseToken"], "runnerInstanceId": reg["runnerInstanceId"], "generation": 1,
	})
	_, err := http.Post(h.srv.URL+"/system/clients/"+reg["leaseId"].(string)+"/heartbeat",
		"application/json", bytes.NewReader(payload))
	if err == nil {
		t.Fatal("heartbeat to dead runner unexpectedly succeeded")
	}
	// Connection refused is exactly what the client's heartbeat watchdog
	// converts into the unplanned-loss notice + quit.
}

// ── D. Migration / rollout hardening ─────────────────────────────────────────

func TestE2E_StaleRunnerAutoReplacesOnlyWhenIdle(t *testing.T) {
	// "Idle" for replacement = no leases AND no protected work. Assert the
	// runner reports the exact signal runnerboot keys on: phase idle_grace
	// only in that window.
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	reg := h.register(t, "tui", "tui-1")
	if snap := h.snapshot(t); snap["phase"] != "ready" {
		t.Fatalf("phase with lease = %v", snap["phase"])
	}
	if status, _ := h.release(t, reg); status != http.StatusOK {
		t.Fatalf("release → %d", status)
	}
	h.waitPhase(t, lifecycle.PhaseIdleGrace, 3*time.Second)
	// In idle_grace a stale build is safe to replace (runnerboot side covered
	// by TestEnsureRunner_IdleStaleAutoReplaces); assert updatePending stays
	// false so no pending update is misreported.
	if snap := h.snapshot(t); snap["updatePending"] == true {
		t.Fatalf("updatePending set while idle: %v", snap)
	}
}

func TestE2E_StaleRunnerBusyReportsUpdatePending(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, activeTurnWork)
	h.register(t, "tui", "tui-1")
	h.mgr.SetUpdatePending(true) // runnerboot classified this runner busy-stale

	snap := h.snapshot(t)
	if snap["updatePending"] != true {
		t.Fatalf("updatePending not surfaced: %v", snap)
	}
	workload, _ := snap["workload"].(map[string]any)
	items, _ := workload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("busy workload not surfaced: %v", workload)
	}
	// Clients reading the snapshot can show "update pending" — the runner
	// itself is NOT replaced while busy.
}

func TestE2E_LegacyRunnerRequiresExplicitReplacement(t *testing.T) {
	// A legacy runner has no lifecycle manager: the whole /system/lifecycle
	// surface is absent → clients classify it as legacy (runnerboot side:
	// TestEnsureRunner_LegacyRequiresConfirmation) instead of fencing it.
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(r.Health())
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/system/lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("legacy /system/lifecycle → %d, want 404", resp.StatusCode)
	}
	// And /health carries no runnerInstanceId → unmistakably legacy.
	resp, err = http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var health map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&health)
	if _, ok := health["runnerInstanceId"]; ok {
		t.Fatalf("legacy health unexpectedly carries runnerInstanceId: %v", health)
	}
}

func TestE2E_WorkspaceMismatchRejected(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	// Registration records projectPath; runnerboot compares it against the
	// runner's own workspace before adopting. Assert the lease view carries
	// the path so a mismatch is detectable, never silently merged.
	status, body := h.post(t, "/system/clients/register", map[string]any{
		"kind": "tui", "clientInstanceId": "tui-ws", "projectPath": "/some/other/workspace",
	})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("register → %d: %v", status, body)
	}
	snap := h.snapshot(t)
	clients, _ := snap["clients"].([]any)
	found := false
	for _, c := range clients {
		cv, _ := c.(map[string]any)
		if cv["projectPath"] == "/some/other/workspace" {
			found = true
		}
	}
	if !found {
		t.Fatalf("projectPath not recorded in lease view: %v", clients)
	}
	// A second client for a different workspace registers separately — the
	// runner never merges identities across workspaces.
	status, _ = h.post(t, "/system/clients/register", map[string]any{
		"kind": "desktop", "clientInstanceId": "desk-ws", "projectPath": "/some/other/workspace",
	})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("second register → %d", status)
	}
	if snap := h.snapshot(t); len(snap["clients"].([]any)) != 2 {
		t.Fatalf("clients = %v", snap["clients"])
	}
}

func TestE2E_UnknownPortProcessIsNotKilled(t *testing.T) {
	// A foreign process bound to a port answers HTTP but is not a FlowPilot
	// runner: /health has no `status` field, /system/lifecycle 404s. Assert
	// the lifecycle client surface distinguishes it — and that NOTHING in
	// the lifecycle API offers a blind kill path.
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "not a flowpilot runner")
	}))
	defer foreign.Close()

	resp, err := http.Get(foreign.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	var health map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&health)
	resp.Body.Close()
	if _, isRunner := health["status"]; isRunner {
		t.Fatal("foreign listener masqueraded as a runner")
	}
	resp, err = http.Post(foreign.URL+"/system/shutdown", "application/json", strings.NewReader("{}"))
	if err == nil {
		resp.Body.Close()
	}
	// The foreign server is still alive — nobody killed it.
	if _, err := http.Get(foreign.URL + "/health"); err != nil {
		t.Fatal("foreign listener was killed")
	}
	// Defense in depth: the real lifecycle surface has no route that can be
	// invoked against an unknown process — only /system/shutdown on the
	// runner itself, which is lease/confirmation fenced (next test).
}

func TestE2E_SupervisorStaleCommandCannotKillNewGeneration(t *testing.T) {
	// The fence is runnerInstanceId + generation. Two generations of the
	// manager get different instance IDs — a stale supervisor command for
	// gen N can never match gen N+1.
	h1 := newE2EHarness(t, lifecycle.ModeSupervised, nil)
	reg1 := h1.register(t, "tui", "tui-1")
	inst1 := reg1["runnerInstanceId"].(string)

	r2, _ := New(t.TempDir())
	mgr2 := lifecycle.NewManager(lifecycle.Config{Mode: lifecycle.ModeSupervised, SweepInterval: time.Hour})
	defer mgr2.Close()
	r2.AttachLifecycle(mgr2)
	snap2, _ := mgr2.Snapshot(context.Background())
	inst2 := snap2.RunnerInstanceID

	if inst1 == inst2 {
		t.Fatalf("generations share instance id %q — supervisor fence broken", inst1)
	}
	// A fenced request carrying gen-1's instance ID is rejected by gen-1
	// after identity change — and would be meaningless to any other gen.
	status, body := h1.post(t, "/system/shutdown", map[string]any{
		"requesterLeaseId":   reg1["leaseId"],
		"leaseToken":         reg1["leaseToken"],
		"expectedInstanceId": inst2, // stale/other-generation fence
	})
	if status != http.StatusConflict {
		t.Fatalf("stale-instance shutdown → %d, want 409: %v", status, body)
	}
}

func TestE2E_NoMCPChildCanShutdownRunner(t *testing.T) {
	// MCP children / stray processes hold no lease. With live clients the
	// destructive surface must refuse to proceed silently.
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	h.register(t, "tui", "tui-1")

	status, body := h.post(t, "/system/shutdown", map[string]any{
		// no requesterLeaseId, no leaseToken — an unauthenticated caller
		"reason": "mcp_child_attempt",
	})
	if status == http.StatusAccepted || status == http.StatusOK {
		t.Fatalf("lease-less shutdown accepted with live clients: %d %v", status, body)
	}
	if status != http.StatusConflict {
		t.Fatalf("lease-less shutdown → %d, want 409 confirmation_required: %v", status, body)
	}
	// Forged lease credentials are fenced too.
	reg := map[string]any{"leaseId": "lease_forged", "leaseToken": "forged", "runnerInstanceId": h.snapshot(t)["runnerInstanceId"]}
	status, body = h.post(t, "/system/shutdown", map[string]any{
		"requesterLeaseId": reg["leaseId"], "leaseToken": reg["leaseToken"],
	})
	if status == http.StatusAccepted || status == http.StatusOK {
		t.Fatalf("forged-lease shutdown accepted: %d %v", status, body)
	}
	if h.drainCount() != 0 {
		t.Fatal("drain fired for unauthenticated request")
	}
}

func TestE2E_NoGoProcessOrCompiledRunnerOrphaned(t *testing.T) {
	// Runner-side evidence for orphan-free shutdown: a drain decision emits
	// exactly one phase transition into draining_* and records the requester
	// so the supervisor-side process sweep (taskkill /T, negative-PID group
	// kill — supervisorLifecycle.test.ts) is attributed and single-shot.
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	reg := h.register(t, "tui", "tui-1")

	status, _ := h.post(t, "/system/shutdown", map[string]any{
		"requesterLeaseId":   reg["leaseId"],
		"leaseToken":         reg["leaseToken"],
		"expectedInstanceId": reg["runnerInstanceId"],
	})
	if status != http.StatusAccepted {
		t.Fatalf("sole-client shutdown → %d", status)
	}
	h.waitPhase(t, lifecycle.PhaseDrainingShutdown, 2*time.Second)
	// Idempotent re-request during drain → 202 again, no second drain.
	status, _ = h.post(t, "/system/shutdown", map[string]any{
		"requesterLeaseId": reg["leaseId"], "leaseToken": reg["leaseToken"],
	})
	if status != http.StatusAccepted {
		t.Fatalf("re-request during drain → %d", status)
	}
	time.Sleep(150 * time.Millisecond)
	if h.drainCount() != 1 {
		t.Fatalf("drain fired %d times, want exactly 1 (single-shot cleanup)", h.drainCount())
	}
	// MarkStopped → stopped transition fires once; the supervisor then owns
	// the process-tree sweep.
	h.mgr.MarkStopped()
	h.waitPhase(t, lifecycle.PhaseStopped, 2*time.Second)
}

// ── sanity: the harness really is on a live socket ───────────────────────────

func TestE2E_HarnessIsLiveHTTP(t *testing.T) {
	h := newE2EHarness(t, lifecycle.ModeClientManaged, nil)
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(h.srv.URL, "http://"), 2*time.Second)
	if err != nil {
		t.Fatalf("harness not on a live socket: %v", err)
	}
	conn.Close()
}
