package lifecycle

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── deterministic harness ────────────────────────────────────────────────────

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

// testRig bundles a manager with a fake clock, a settable workload, and a
// phase-transition recorder. SweepInterval is huge so the real ticker never
// fires — every evaluation is driven by an explicit method call.
type testRig struct {
	m       *Manager
	clock   *fakeClock
	workMu  sync.Mutex
	work    WorkloadSnapshot
	workErr error
	mu      sync.Mutex
	phases  []RunnerPhase
	snaps   []LifecycleSnapshot
}

func newTestRig(t *testing.T, mutate func(*Config)) *testRig {
	t.Helper()
	rig := &testRig{clock: newFakeClock()}
	cfg := Config{
		RunnerInstanceID: "inst-test",
		Mode:             ModeClientManaged,
		ProtocolVersion:  1,
		BuildID:          "build-test",
		HeartbeatTTL:     15 * time.Second,
		BootGrace:        30 * time.Second,
		IdleGrace:        30 * time.Second,
		ReconnectGrace:   60 * time.Second,
		ConfirmTokenTTL:  15 * time.Second,
		SweepInterval:    time.Hour,
		Now:              rig.clock.Now,
		WorkSnapshot: func(context.Context) (WorkloadSnapshot, error) {
			rig.workMu.Lock()
			defer rig.workMu.Unlock()
			return rig.work, rig.workErr
		},
		OnPhase: func(p RunnerPhase, s LifecycleSnapshot) {
			rig.mu.Lock()
			rig.phases = append(rig.phases, p)
			rig.snaps = append(rig.snaps, s)
			rig.mu.Unlock()
		},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	rig.m = NewManager(cfg)
	t.Cleanup(rig.m.Close)
	return rig
}

func (r *testRig) setWork(items ...WorkloadItem) {
	r.workMu.Lock()
	r.work = WorkloadSnapshot{Items: items}
	r.workMu.Unlock()
}

func (r *testRig) phaseTransitions() []RunnerPhase {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RunnerPhase(nil), r.phases...)
}

func (r *testRig) lastTransition() RunnerPhase {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.phases) == 0 {
		return ""
	}
	return r.phases[len(r.phases)-1]
}

func (r *testRig) register(t *testing.T, kind ClientKind, instanceID string) RegisterResult {
	t.Helper()
	res, err := r.m.Register(context.Background(), RegisterInput{
		Kind:             kind,
		ClientInstanceID: instanceID,
		PID:              4242,
		Label:            string(kind) + "-" + instanceID,
	})
	if err != nil {
		t.Fatalf("register %s: %v", instanceID, err)
	}
	return res
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	le, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %T %v, want *Error code %s", err, err, code)
	}
	if le.Code != code {
		t.Fatalf("err code = %s, want %s", le.Code, code)
	}
}

// ── Task-414 §7 test matrix ──────────────────────────────────────────────────

func TestLifecycle_RegisterIssuesGenerationScopedLease(t *testing.T) {
	rig := newTestRig(t, nil)
	res := rig.register(t, ClientKindTUI, "tui-1")
	if res.LeaseID == "" || res.LeaseToken == "" {
		t.Fatal("register must return leaseId + raw leaseToken")
	}
	if res.RunnerInstanceID != "inst-test" || res.Generation != 1 {
		t.Fatalf("identity = %s gen %d, want inst-test/1", res.RunnerInstanceID, res.Generation)
	}
	if res.HeartbeatIntervalMs <= 0 || res.TTLMs != 15000 {
		t.Fatalf("heartbeat=%d ttl=%d", res.HeartbeatIntervalMs, res.TTLMs)
	}
	if res.Snapshot.Phase != PhaseReady {
		t.Fatalf("phase = %s, want ready after first lease", res.Snapshot.Phase)
	}
	if len(res.Snapshot.Clients) != 1 || res.Snapshot.Clients[0].LeaseID != res.LeaseID {
		t.Fatalf("clients = %+v", res.Snapshot.Clients)
	}
	// The raw token must never appear inside the stored lease/snapshot.
	for _, c := range res.Snapshot.Clients {
		if strings.Contains(c.LeaseID, res.LeaseToken) {
			t.Fatal("snapshot leaked token material")
		}
	}
}

func TestLifecycle_RegisterSameClientInstanceIsIdempotent(t *testing.T) {
	rig := newTestRig(t, nil)
	first := rig.register(t, ClientKindTUI, "tui-1")
	second, err := rig.m.Register(context.Background(), RegisterInput{
		Kind:             ClientKindTUI,
		ClientInstanceID: "tui-1",
	})
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if !second.Idempotent {
		t.Fatal("re-register should report Idempotent")
	}
	if second.LeaseID != first.LeaseID {
		t.Fatalf("leaseId changed: %s → %s", first.LeaseID, second.LeaseID)
	}
	if second.LeaseToken == first.LeaseToken {
		t.Fatal("re-register must rotate the token")
	}
	snap, _ := rig.m.Snapshot(context.Background())
	if len(snap.Clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(snap.Clients))
	}
	// Old token must be dead after rotation.
	_, err = rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: first.LeaseID, LeaseToken: first.LeaseToken, RunnerInstanceID: "inst-test", Generation: 1,
	})
	requireCode(t, err, ErrInvalidLeaseToken)
}

func TestLifecycle_RegisterDuringDrainRejected(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	// Sole client + idle → direct accept, phase enters draining_shutdown.
	res, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID:   a.LeaseID,
		LeaseToken:         a.LeaseToken,
		ExpectedInstanceID: "inst-test",
		Reason:             "user_exit",
	})
	if err != nil || !res.Accepted {
		t.Fatalf("shutdown: %+v err=%v", res, err)
	}
	_, err = rig.m.Register(context.Background(), RegisterInput{
		Kind: ClientKindDesktop, ClientInstanceID: "desk-1",
	})
	requireCode(t, err, ErrRunnerDraining)
}

func TestLifecycle_HeartbeatExtendsLease(t *testing.T) {
	rig := newTestRig(t, nil)
	reg := rig.register(t, ClientKindTUI, "tui-1")
	rig.clock.Advance(10 * time.Second) // inside TTL
	_, err := rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: reg.LeaseID, LeaseToken: reg.LeaseToken,
		RunnerInstanceID: "inst-test", Generation: 1,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	rig.clock.Advance(10 * time.Second) // 20s total — beyond original TTL
	snap, err := rig.m.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Clients) != 1 {
		t.Fatalf("lease expired despite heartbeat: %+v", snap.Clients)
	}
	if snap.Phase != PhaseReady {
		t.Fatalf("phase = %s, want ready", snap.Phase)
	}
}

func TestLifecycle_HeartbeatStaleGenerationRejected(t *testing.T) {
	rig := newTestRig(t, nil)
	reg := rig.register(t, ClientKindTUI, "tui-1")
	// Simulate an epoch bump (a new lease generation inside the process).
	rig.m.mu.Lock()
	rig.m.generation = 2
	rig.m.mu.Unlock()
	_, err := rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: reg.LeaseID, LeaseToken: reg.LeaseToken,
		RunnerInstanceID: "inst-test", Generation: 1,
	})
	requireCode(t, err, ErrStaleGeneration)
	// Wrong runnerInstanceId is stale too (cross-process fence).
	rig.m.mu.Lock()
	rig.m.generation = 1
	rig.m.mu.Unlock()
	_, err = rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: reg.LeaseID, LeaseToken: reg.LeaseToken,
		RunnerInstanceID: "inst-other", Generation: 1,
	})
	requireCode(t, err, ErrStaleGeneration)
}

func TestLifecycle_HeartbeatInvalidTokenRejected(t *testing.T) {
	rig := newTestRig(t, nil)
	reg := rig.register(t, ClientKindTUI, "tui-1")
	_, err := rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: reg.LeaseID, LeaseToken: "wrong-token",
		RunnerInstanceID: "inst-test", Generation: 1,
	})
	requireCode(t, err, ErrInvalidLeaseToken)
}

func TestLifecycle_HeartbeatUnknownLeaseRejected(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	_, err := rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: "lease_nope", LeaseToken: "x",
		RunnerInstanceID: "inst-test", Generation: 1,
	})
	requireCode(t, err, ErrLeaseUnknown)
}

func TestLifecycle_ReleaseRemovesClient(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	b := rig.register(t, ClientKindDesktop, "desk-1")
	snap, err := rig.m.Release(context.Background(), ReleaseInput{
		LeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
		RunnerInstanceID: "inst-test", Generation: 1,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if len(snap.Clients) != 1 || snap.Clients[0].LeaseID != b.LeaseID {
		t.Fatalf("clients = %+v", snap.Clients)
	}
	if snap.Phase != PhaseReady {
		t.Fatalf("phase = %s, want ready (one lease remains)", snap.Phase)
	}
}

func TestLifecycle_ReleaseIsIdempotent(t *testing.T) {
	rig := newTestRig(t, nil)
	reg := rig.register(t, ClientKindTUI, "tui-1")
	in := ReleaseInput{LeaseID: reg.LeaseID, LeaseToken: reg.LeaseToken, RunnerInstanceID: "inst-test", Generation: 1}
	if _, err := rig.m.Release(context.Background(), in); err != nil {
		t.Fatalf("first release: %v", err)
	}
	snap, err := rig.m.Release(context.Background(), in)
	if err != nil {
		t.Fatalf("repeat release must be idempotent, got %v", err)
	}
	if len(snap.Clients) != 0 {
		t.Fatalf("clients = %+v", snap.Clients)
	}
	// Releasing a never-registered lease is unknown, not silently ok.
	_, err = rig.m.Release(context.Background(), ReleaseInput{
		LeaseID: "lease_ghost", LeaseToken: "x", RunnerInstanceID: "inst-test", Generation: 1,
	})
	requireCode(t, err, ErrLeaseUnknown)
}

func TestLifecycle_ExpiredLeaseTransitionsIdleGrace(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.clock.Advance(16 * time.Second) // past 15s TTL, no heartbeat
	snap, err := rig.m.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Clients) != 0 {
		t.Fatalf("expired lease still live: %+v", snap.Clients)
	}
	if snap.Phase != PhaseIdleGrace {
		t.Fatalf("phase = %s, want idle_grace", snap.Phase)
	}
	if snap.IdleDeadline == nil {
		t.Fatal("idle_grace must carry a deadline")
	}
}

func TestLifecycle_RegisterDuringIdleGraceCancelsShutdown(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.clock.Advance(16 * time.Second)
	if s, _ := rig.m.Snapshot(context.Background()); s.Phase != PhaseIdleGrace {
		t.Fatalf("phase = %s, want idle_grace", s.Phase)
	}
	rig.register(t, ClientKindDesktop, "desk-1")
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseReady {
		t.Fatalf("phase = %s, want ready (attach cancelled shutdown)", snap.Phase)
	}
	if snap.IdleDeadline != nil {
		t.Fatal("idle deadline must be cleared on reattach")
	}
}

func TestLifecycle_IdleGraceDeadlineDrainsShutdown(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.clock.Advance(16 * time.Second) // → idle_grace
	if s, _ := rig.m.Snapshot(context.Background()); s.Phase != PhaseIdleGrace {
		t.Fatalf("phase = %s", s.Phase)
	}
	rig.clock.Advance(31 * time.Second) // past 30s idle grace
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseDrainingShutdown {
		t.Fatalf("phase = %s, want draining_shutdown after idle deadline", snap.Phase)
	}
}

func TestLifecycle_WorkloadBlocksIdleGrace(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1", Cancellable: true})
	rig.clock.Advance(16 * time.Second) // lease expires but work is active
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseOrphanedWork {
		t.Fatalf("phase = %s, want orphaned_work (leases gone, work active)", snap.Phase)
	}
	// Even far past the idle grace window, active work blocks shutdown.
	rig.clock.Advance(60 * time.Second)
	snap, _ = rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseOrphanedWork {
		t.Fatalf("phase = %s, orphaned work must never idle-exit", snap.Phase)
	}
}

func TestLifecycle_OrphanedWorkTransitionsToIdleGraceWhenDrained(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"})
	rig.clock.Advance(16 * time.Second)
	if s, _ := rig.m.Snapshot(context.Background()); s.Phase != PhaseOrphanedWork {
		t.Fatalf("phase = %s, want orphaned_work", s.Phase)
	}
	rig.setWork() // work drains
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseIdleGrace {
		t.Fatalf("phase = %s, want idle_grace after drain", snap.Phase)
	}
	if snap.IdleDeadline == nil {
		t.Fatal("idle_grace must carry deadline")
	}
}

func TestLifecycle_ReattachDuringOrphanedWorkReturnsReady(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "agent", RunID: "run-9"})
	rig.clock.Advance(16 * time.Second)
	if s, _ := rig.m.Snapshot(context.Background()); s.Phase != PhaseOrphanedWork {
		t.Fatalf("phase = %s", s.Phase)
	}
	rig.register(t, ClientKindDesktop, "desk-1")
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseReady {
		t.Fatalf("phase = %s, want ready on reattach", snap.Phase)
	}
}

func TestLifecycle_RequestShutdownBusyReturnsConfirmation(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	rig.register(t, ClientKindDesktop, "desk-1") // shared → busy
	res, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID:   a.LeaseID,
		LeaseToken:         a.LeaseToken,
		ExpectedInstanceID: "inst-test",
		Reason:             "user_exit",
	})
	if err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if !res.ConfirmationRequired || res.Accepted {
		t.Fatalf("res = %+v, want confirmation required", res)
	}
	if res.ConfirmationToken == "" {
		t.Fatal("missing confirmToken")
	}
	if res.ConfirmationExpiresAt.IsZero() {
		t.Fatal("missing token expiry")
	}
	if len(res.Snapshot.Clients) != 2 {
		t.Fatalf("snapshot clients = %+v", res.Snapshot.Clients)
	}
	if s, _ := rig.m.Snapshot(context.Background()); s.Phase != PhaseReady {
		t.Fatalf("phase = %s, must stay ready until confirmed", s.Phase)
	}
}

func TestLifecycle_ConfirmedShutdownUsesFreshInventoryToken(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"})
	res, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
		ExpectedInstanceID: "inst-test", Reason: "user_exit",
	})
	if err != nil || !res.ConfirmationRequired {
		t.Fatalf("phase 1: %+v err=%v", res, err)
	}
	done, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
		ExpectedInstanceID: "inst-test", Reason: "user_exit",
		Confirm: true, ConfirmToken: res.ConfirmationToken,
	})
	if err != nil || !done.Accepted {
		t.Fatalf("phase 2: %+v err=%v", done, err)
	}
	if done.Snapshot.Phase != PhaseDrainingShutdown {
		t.Fatalf("phase = %s, want draining_shutdown", done.Snapshot.Phase)
	}
	rig.m.MarkStopped()
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseStopped {
		t.Fatalf("phase = %s, want stopped after MarkStopped", snap.Phase)
	}
}

func TestLifecycle_StaleConfirmTokenRejected(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"})
	res, _ := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken, ExpectedInstanceID: "inst-test",
	})
	// Inventory changes after the token was minted → token is stale.
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"},
		WorkloadItem{Kind: "turn", RunID: "run-2"})
	_, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
		ExpectedInstanceID: "inst-test", Confirm: true, ConfirmToken: res.ConfirmationToken,
	})
	requireCode(t, err, ErrStaleLifecycleSnapshot)
}

func TestLifecycle_ExpiredConfirmTokenRejected(t *testing.T) {
	rig := newTestRig(t, func(c *Config) { c.ConfirmTokenTTL = 5 * time.Second })
	a := rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"})
	res, _ := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken, ExpectedInstanceID: "inst-test",
	})
	rig.clock.Advance(6 * time.Second) // past 5s confirm TTL, inside lease TTL
	_, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
		ExpectedInstanceID: "inst-test", Confirm: true, ConfirmToken: res.ConfirmationToken,
	})
	requireCode(t, err, ErrStaleLifecycleSnapshot)
}

func TestLifecycle_WrongInstanceShutdownRejected(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	_, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		ExpectedInstanceID: "inst-other", Reason: "supervisor",
	})
	requireCode(t, err, ErrStaleLifecycleSnapshot)
}

func TestLifecycle_RequestRestartEntersDrainingRestart(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	res, err := rig.m.RequestRestart(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
		ExpectedInstanceID: "inst-test", Reason: "system_control",
	})
	if err != nil || !res.Accepted {
		t.Fatalf("restart: %+v err=%v", res, err)
	}
	if res.RestartID == "" || res.ReconnectDeadline == nil {
		t.Fatalf("restart result missing handoff: %+v", res)
	}
	if res.Snapshot.Phase != PhaseDrainingRestart {
		t.Fatalf("phase = %s", res.Snapshot.Phase)
	}
	if res.Snapshot.Restart == nil || res.Snapshot.Restart.RestartID != res.RestartID {
		t.Fatalf("snapshot restart = %+v", res.Snapshot.Restart)
	}
}

func TestLifecycle_RestartSnapshotCarriesReconnectDeadline(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	res, _ := rig.m.RequestRestart(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken, ExpectedInstanceID: "inst-test",
	})
	// A heartbeat during drain still returns 200 + the restart metadata so
	// the client can enter reconnect mode (D-5).
	snap, err := rig.m.Heartbeat(context.Background(), HeartbeatInput{
		LeaseID: a.LeaseID, LeaseToken: a.LeaseToken, RunnerInstanceID: "inst-test", Generation: 1,
	})
	if err != nil {
		t.Fatalf("heartbeat during drain: %v", err)
	}
	if snap.Restart == nil || snap.Restart.Deadline.IsZero() {
		t.Fatalf("snapshot restart = %+v", snap.Restart)
	}
	want := res.ReconnectDeadline
	if !snap.Restart.Deadline.Equal(*want) {
		t.Fatalf("deadline = %v, want %v", snap.Restart.Deadline, *want)
	}
}

func TestLifecycle_ConcurrentRegisterAndExpiryAreAtomic(t *testing.T) {
	rig := newTestRig(t, nil)
	const n = 32
	var wg sync.WaitGroup
	errs := make(chan error, n*2)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := rig.m.Register(context.Background(), RegisterInput{
				Kind: ClientKindTUI, ClientInstanceID: strings.Repeat("x", 1) + string(rune('a'+i)),
			})
			if err != nil {
				errs <- err
				return
			}
			if i%2 == 0 {
				rig.clock.Advance(20 * time.Second) // force expiry mid-flight
			}
			_, err = rig.m.Heartbeat(context.Background(), HeartbeatInput{
				LeaseID: res.LeaseID, LeaseToken: res.LeaseToken,
				RunnerInstanceID: "inst-test", Generation: 1,
			})
			if err != nil {
				if le, ok := err.(*Error); ok && le.Code == ErrLeaseUnknown {
					return // expired concurrently — legal, but must be typed
				}
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent error: %v", err)
	}
	snap, _ := rig.m.Snapshot(context.Background())
	for _, c := range snap.Clients {
		if c.LeaseID == "" {
			t.Fatal("corrupt lease view")
		}
	}
}

func TestLifecycle_ConcurrentShutdownAndAttachAreSerialized(t *testing.T) {
	for attempt := 0; attempt < 25; attempt++ {
		rig := newTestRig(t, nil)
		a := rig.register(t, ClientKindTUI, "tui-1")
		rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"})
		var wg sync.WaitGroup
		var shutRes SystemActionResult
		var shutErr, regErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			// confirm path — busy runner needs the token first.
			pre, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
				RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken, ExpectedInstanceID: "inst-test",
			})
			if err != nil || !pre.ConfirmationRequired {
				shutErr = err
				return
			}
			shutRes, shutErr = rig.m.RequestShutdown(context.Background(), SystemActionInput{
				RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken,
				ExpectedInstanceID: "inst-test", Confirm: true, ConfirmToken: pre.ConfirmationToken,
			})
			_ = shutRes
		}()
		go func() {
			defer wg.Done()
			_, regErr = rig.m.Register(context.Background(), RegisterInput{
				Kind: ClientKindDesktop, ClientInstanceID: "desk-late",
			})
		}()
		wg.Wait()
		// Serialized: either the register landed before the drain (shutdown
		// confirm may fail stale on inventory change — both typed), or the
		// drain won and register got runner_draining. Never a panic/nil-map.
		if regErr != nil {
			requireCode(t, regErr, ErrRunnerDraining)
		}
		if shutErr != nil {
			le, ok := shutErr.(*Error)
			if !ok || (le.Code != ErrStaleLifecycleSnapshot && le.Code != ErrRunnerDraining) {
				t.Fatalf("shutdown err = %v", shutErr)
			}
		}
		snap, _ := rig.m.Snapshot(context.Background())
		if snap.Phase == "" {
			t.Fatal("empty phase")
		}
		rig.m.Close()
	}
}

func TestLifecycle_TokensAreHashedAndNotLogged(t *testing.T) {
	rig := newTestRig(t, nil)
	res := rig.register(t, ClientKindTUI, "tui-1")
	rig.m.mu.Lock()
	lease := rig.m.leases[res.LeaseID]
	rig.m.mu.Unlock()
	if lease == nil {
		t.Fatal("lease missing")
	}
	if lease.TokenHash == res.LeaseToken || lease.TokenHash == "" {
		t.Fatal("raw token stored instead of hash")
	}
	if lease.TokenHash != hashToken(res.LeaseToken) {
		t.Fatal("TokenHash is not sha256(raw)")
	}
	// Snapshot String/JSON surface must not contain the raw token.
	snap, _ := rig.m.Snapshot(context.Background())
	raw := snap.Clients[0]
	for _, field := range []string{raw.LeaseID, raw.Label, raw.ClientInstanceID} {
		if strings.Contains(field, res.LeaseToken) {
			t.Fatal("token leaked into snapshot field")
		}
	}
}

func TestLifecycle_PersistentModeNeverIdleExits(t *testing.T) {
	rig := newTestRig(t, func(c *Config) { c.Mode = ModePersistent })
	rig.register(t, ClientKindTUI, "tui-1")
	rig.clock.Advance(16 * time.Second)
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase == PhaseIdleGrace {
		t.Fatal("persistent runner must not enter idle_grace")
	}
	rig.clock.Advance(600 * time.Second)
	snap, _ = rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseReady {
		t.Fatalf("persistent phase = %s, want ready", snap.Phase)
	}
}

func TestLifecycle_BootGraceZeroClientsGoesIdle(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.clock.Advance(31 * time.Second) // past 30s boot grace
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseIdleGrace {
		t.Fatalf("phase = %s, want idle_grace after boot grace", snap.Phase)
	}
	rig.clock.Advance(31 * time.Second) // past idle grace too
	snap, _ = rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseDrainingShutdown {
		t.Fatalf("phase = %s, want draining_shutdown", snap.Phase)
	}
}

func TestLifecycle_ShutdownDuringDrainIsIdempotent(t *testing.T) {
	rig := newTestRig(t, nil)
	a := rig.register(t, ClientKindTUI, "tui-1")
	first, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken, ExpectedInstanceID: "inst-test",
	})
	if err != nil || !first.Accepted {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := rig.m.RequestShutdown(context.Background(), SystemActionInput{
		RequesterLeaseID: a.LeaseID, LeaseToken: a.LeaseToken, ExpectedInstanceID: "inst-test",
	})
	if err != nil || !second.Accepted {
		t.Fatalf("repeat shutdown during drain must be idempotent: %+v %v", second, err)
	}
}

func TestLifecycle_WorkloadSnapshotErrorKeepsLastInventory(t *testing.T) {
	rig := newTestRig(t, nil)
	rig.register(t, ClientKindTUI, "tui-1")
	rig.setWork(WorkloadItem{Kind: "turn", RunID: "run-1"})
	rig.clock.Advance(16 * time.Second)
	if s, _ := rig.m.Snapshot(context.Background()); s.Phase != PhaseOrphanedWork {
		t.Fatalf("phase = %s", s.Phase)
	}
	// Snapshot provider starts failing — manager must NOT treat missing data
	// as "no work" (fail-safe against idle-exit during inventory errors).
	rig.workMu.Lock()
	rig.workErr = errInjected{}
	rig.workMu.Unlock()
	rig.clock.Advance(120 * time.Second)
	snap, _ := rig.m.Snapshot(context.Background())
	if snap.Phase != PhaseOrphanedWork {
		t.Fatalf("phase = %s, inventory error must keep last known work", snap.Phase)
	}
}

type errInjected struct{}

func (errInjected) Error() string { return "injected inventory failure" }
