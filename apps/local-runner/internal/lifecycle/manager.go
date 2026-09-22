package lifecycle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Default timings (SS-24 / SD-28). Config may override; zero falls back here.
const (
	DefaultHeartbeatTTL    = 15 * time.Second
	DefaultHeartbeatPeriod = 5 * time.Second
	DefaultBootGrace       = 30 * time.Second
	DefaultIdleGrace       = 30 * time.Second
	DefaultReconnectGrace  = 60 * time.Second
	DefaultConfirmTokenTTL = 15 * time.Second
	minSweepInterval       = 50 * time.Millisecond
)

// Config wires the manager (Task-414 §6). Now, WorkSnapshot, and OnPhase are
// injectable so tests are fully deterministic; the manager itself never
// sleeps on real time except the sweeper ticker.
type Config struct {
	RunnerInstanceID string
	Generation       int // defaults to 1
	Mode             LifecycleMode
	ProtocolVersion  int
	BuildID          string

	HeartbeatTTL    time.Duration
	BootGrace       time.Duration
	IdleGrace       time.Duration
	ReconnectGrace  time.Duration
	ConfirmTokenTTL time.Duration

	// WorkSnapshot returns the current protected-work inventory. Called
	// without the manager lock held; errors keep the last known inventory
	// (fail-safe: never idle-exit on a snapshot error).
	WorkSnapshot func(context.Context) (WorkloadSnapshot, error)
	// OnPhase fires after every phase transition, outside the manager lock,
	// with the post-transition snapshot. The runner uses it to drive
	// stop-all, cleanup, and exit — the manager never exits on its own.
	OnPhase func(RunnerPhase, LifecycleSnapshot)
	// Now is the injectable clock; nil = time.Now.
	Now func() time.Time
	// SweepInterval overrides the expiry/transition ticker (tests inject a
	// large value and drive evaluation through method calls).
	SweepInterval time.Duration
}

// pendingConfirmation is a minted-but-unconsumed destructive-action token
// (D-4). It binds action + runner instance + the inventory revision the
// requester saw; any inventory change or expiry makes it stale.
type pendingConfirmation struct {
	action           string // "shutdown" | "restart"
	tokenHash        string
	inventoryRev     int64
	expiresAt        time.Time
	requesterLeaseID string
}

// Manager is the serialized lifecycle authority. All mutation flows through
// m.mu; WorkSnapshot is fetched outside the lock so a slow inventory scan
// cannot block lease operations.
type Manager struct {
	cfg Config

	mu               sync.Mutex
	generation       int
	startedAt        time.Time
	phase            RunnerPhase
	leases           map[string]*ClientLease // leaseID -> lease
	byInstance       map[string]string       // clientInstanceID -> leaseID
	releasedLeaseIDs map[string]bool         // this generation's released ids
	workload         WorkloadSnapshot
	workloadErr      error
	inventoryRev     int64
	inventoryHash    string
	idleDeadline     *time.Time
	restart          *RestartInfo
	updatePending    bool
	drainRequester   string                          // audit identity recorded when a drain was accepted
	confirmations    map[string]*pendingConfirmation // action -> pending

	events    []RunnerPhase // transitions queued for OnPhase, fired unlocked
	stopCh    chan struct{}
	stoppedCh chan struct{}
	now       func() time.Time
}

// NewManager constructs the lifecycle authority. Phase starts at `starting`;
// the sweeper goroutine begins immediately (stop with Close).
func NewManager(cfg Config) *Manager {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.HeartbeatTTL <= 0 {
		cfg.HeartbeatTTL = DefaultHeartbeatTTL
	}
	if cfg.BootGrace <= 0 {
		cfg.BootGrace = DefaultBootGrace
	}
	if cfg.IdleGrace <= 0 {
		cfg.IdleGrace = DefaultIdleGrace
	}
	if cfg.ReconnectGrace <= 0 {
		cfg.ReconnectGrace = DefaultReconnectGrace
	}
	if cfg.ConfirmTokenTTL <= 0 {
		cfg.ConfirmTokenTTL = DefaultConfirmTokenTTL
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeClientManaged
	}
	if cfg.RunnerInstanceID == "" {
		cfg.RunnerInstanceID = "instance_" + randHex(16)
	}
	gen := cfg.Generation
	if gen <= 0 {
		gen = 1
	}
	m := &Manager{
		cfg:              cfg,
		generation:       gen,
		startedAt:        cfg.Now(),
		phase:            PhaseStarting,
		leases:           map[string]*ClientLease{},
		byInstance:       map[string]string{},
		releasedLeaseIDs: map[string]bool{},
		confirmations:    map[string]*pendingConfirmation{},
		inventoryRev:     1,
		stopCh:           make(chan struct{}),
		stoppedCh:        make(chan struct{}),
		now:              cfg.Now,
	}
	go m.sweepLoop()
	return m
}

// Close stops the sweeper goroutine. It does not change phase — tests and the
// runner exit path own shutdown semantics.
func (m *Manager) Close() {
	select {
	case <-m.stoppedCh:
		return
	default:
	}
	close(m.stopCh)
	<-m.stoppedCh
}

func (m *Manager) sweepLoop() {
	defer close(m.stoppedCh)
	interval := m.cfg.SweepInterval
	if interval <= 0 {
		interval = m.cfg.HeartbeatTTL / 3
		if interval < minSweepInterval {
			interval = minSweepInterval
		}
		if interval > time.Second {
			interval = time.Second
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-t.C:
			m.evaluate(context.Background())
		}
	}
}

// ── lease registry ──────────────────────────────────────────────────────────

// Register issues a generation-scoped lease (D-2). Re-registering an existing
// clientInstanceID is idempotent: same leaseID, rotated token, 200 semantics.
func (m *Manager) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	if strings.TrimSpace(in.ClientInstanceID) == "" {
		return RegisterResult{}, newError(ErrInvalidLifecycleRequest, "clientInstanceId is required")
	}
	m.evaluate(ctx)

	m.mu.Lock()
	if m.phase.IsDraining() {
		snap := m.snapshotLocked()
		m.mu.Unlock()
		m.drainEvents()
		return RegisterResult{}, &Error{Code: ErrRunnerDraining, Message: fmt.Sprintf("runner is %s; new leases rejected", snap.Phase)}
	}

	now := m.now()
	// Idempotent re-register: rotate the token, keep leaseID + createdAt.
	if existingID, ok := m.byInstance[in.ClientInstanceID]; ok {
		if lease, ok := m.leases[existingID]; ok {
			raw := newToken()
			lease.TokenHash = hashToken(raw)
			lease.Kind = in.Kind
			lease.PID = in.PID
			lease.Label = in.Label
			lease.ProjectPath = in.ProjectPath
			lease.ProtocolVersion = in.ProtocolVersion
			lease.BuildID = in.BuildID
			lease.LastHeartbeatAt = now
			lease.ExpiresAt = now.Add(m.cfg.HeartbeatTTL)
			m.bumpInventoryLocked()
			m.transitionForStateLocked(now)
			res := RegisterResult{
				LeaseID:             lease.LeaseID,
				LeaseToken:          raw,
				RunnerInstanceID:    m.cfg.RunnerInstanceID,
				Generation:          m.generation,
				HeartbeatIntervalMs: heartbeatIntervalMs(m.cfg.HeartbeatTTL),
				TTLMs:               m.cfg.HeartbeatTTL.Milliseconds(),
				Idempotent:          true,
				Snapshot:            m.snapshotLocked(),
			}
			m.mu.Unlock()
			m.drainEvents()
			return res, nil
		}
		delete(m.byInstance, in.ClientInstanceID)
	}

	lease := &ClientLease{
		LeaseID:          "lease_" + randHex(12),
		TokenHash:        "",
		ClientInstanceID: in.ClientInstanceID,
		Kind:             in.Kind,
		PID:              in.PID,
		Label:            in.Label,
		ProjectPath:      in.ProjectPath,
		ProtocolVersion:  in.ProtocolVersion,
		BuildID:          in.BuildID,
		Generation:       m.generation,
		CreatedAt:        now,
		LastHeartbeatAt:  now,
		ExpiresAt:        now.Add(m.cfg.HeartbeatTTL),
	}
	raw := newToken()
	lease.TokenHash = hashToken(raw)
	m.leases[lease.LeaseID] = lease
	m.byInstance[lease.ClientInstanceID] = lease.LeaseID
	m.bumpInventoryLocked()
	m.transitionForStateLocked(now)
	res := RegisterResult{
		LeaseID:             lease.LeaseID,
		LeaseToken:          raw,
		RunnerInstanceID:    m.cfg.RunnerInstanceID,
		Generation:          m.generation,
		HeartbeatIntervalMs: heartbeatIntervalMs(m.cfg.HeartbeatTTL),
		TTLMs:               m.cfg.HeartbeatTTL.Milliseconds(),
		Snapshot:            m.snapshotLocked(),
	}
	m.mu.Unlock()
	m.drainEvents()
	return res, nil
}

// Heartbeat refreshes a lease to now+TTL and returns the post-eval snapshot.
func (m *Manager) Heartbeat(ctx context.Context, in HeartbeatInput) (LifecycleSnapshot, error) {
	m.evaluate(ctx)
	m.mu.Lock()
	lease, err := m.authorizeLeaseLocked(in.LeaseID, in.LeaseToken, in.RunnerInstanceID, in.Generation)
	if err != nil {
		m.mu.Unlock()
		m.drainEvents()
		return LifecycleSnapshot{}, err
	}
	now := m.now()
	lease.LastHeartbeatAt = now
	lease.ExpiresAt = now.Add(m.cfg.HeartbeatTTL)
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.drainEvents()
	return snap, nil
}

// Release removes a lease. Repeated release of the same leaseID is idempotent
// (returns the snapshot, no side effects); a never-seen leaseID is
// lease_unknown.
func (m *Manager) Release(ctx context.Context, in ReleaseInput) (LifecycleSnapshot, error) {
	m.evaluate(ctx)
	m.mu.Lock()
	if m.releasedLeaseIDs[in.LeaseID] {
		snap := m.snapshotLocked()
		m.mu.Unlock()
		m.drainEvents()
		return snap, nil
	}
	lease, err := m.authorizeLeaseLocked(in.LeaseID, in.LeaseToken, in.RunnerInstanceID, in.Generation)
	if err != nil {
		m.mu.Unlock()
		m.drainEvents()
		return LifecycleSnapshot{}, err
	}
	m.removeLeaseLocked(lease)
	m.transitionForStateLocked(m.now())
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.drainEvents()
	return snap, nil
}

// Snapshot evaluates timers/expiry and returns the full lifecycle snapshot.
func (m *Manager) Snapshot(ctx context.Context) (LifecycleSnapshot, error) {
	m.evaluate(ctx)
	m.mu.Lock()
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.drainEvents()
	return snap, nil
}

// ── destructive actions (D-4 two-phase) ──────────────────────────────────────

// RequestShutdown asks for a global runner stop. Idle + sole requester →
// accepted immediately; busy or shared → minted confirmToken bound to the
// current inventory revision; confirmed requests revalidate instance + token
// + revision before entering draining_shutdown.
func (m *Manager) RequestShutdown(ctx context.Context, in SystemActionInput) (SystemActionResult, error) {
	return m.requestSystemAction(ctx, "shutdown", in)
}

// RequestRestart is the planned-restart twin of RequestShutdown; on accept it
// enters draining_restart carrying restartId + reconnect deadline.
func (m *Manager) RequestRestart(ctx context.Context, in SystemActionInput) (SystemActionResult, error) {
	return m.requestSystemAction(ctx, "restart", in)
}

func (m *Manager) requestSystemAction(ctx context.Context, action string, in SystemActionInput) (SystemActionResult, error) {
	m.evaluate(ctx)
	m.mu.Lock()
	defer func() {
		m.mu.Unlock()
		m.drainEvents()
	}()

	// Idempotent re-request of the in-flight drain.
	if (action == "shutdown" && m.phase == PhaseDrainingShutdown) ||
		(action == "restart" && m.phase == PhaseDrainingRestart) {
		return SystemActionResult{Accepted: true, Snapshot: m.snapshotLocked()}, nil
	}
	if m.phase.IsDraining() || m.phase == PhaseStopped {
		return SystemActionResult{}, newError(ErrRunnerDraining,
			fmt.Sprintf("runner is %s; cannot start %s", m.phase, action))
	}
	if in.ExpectedInstanceID != "" && in.ExpectedInstanceID != m.cfg.RunnerInstanceID {
		return SystemActionResult{}, newError(ErrStaleLifecycleSnapshot,
			"expectedInstanceId does not match this runner instance")
	}
	// Requester lease fencing: when a leaseID is supplied it must be a live
	// lease of this generation with a valid token.
	if in.RequesterLeaseID != "" {
		if _, err := m.authorizeLeaseLocked(in.RequesterLeaseID, in.LeaseToken, in.ExpectedInstanceID, m.generation); err != nil {
			return SystemActionResult{}, err
		}
	}

	otherClients := len(m.leases)
	if in.RequesterLeaseID != "" {
		if _, ok := m.leases[in.RequesterLeaseID]; ok {
			otherClients--
		}
	}
	busy := m.workload.ActiveCount() > 0 || otherClients > 0
	now := m.now()

	if busy && !in.Confirm && !in.Force {
		raw := newToken()
		pc := &pendingConfirmation{
			action:           action,
			tokenHash:        hashToken(raw),
			inventoryRev:     m.inventoryRev,
			expiresAt:        now.Add(m.cfg.ConfirmTokenTTL),
			requesterLeaseID: in.RequesterLeaseID,
		}
		m.confirmations[action] = pc
		return SystemActionResult{
			ConfirmationRequired:  true,
			ConfirmationToken:     raw,
			ConfirmationExpiresAt: pc.expiresAt,
			Snapshot:              m.snapshotLocked(),
		}, nil
	}

	if busy && !in.Force {
		pc, ok := m.confirmations[action]
		if !ok || now.After(pc.expiresAt) ||
			pc.inventoryRev != m.inventoryRev ||
			pc.tokenHash != hashToken(in.ConfirmToken) {
			return SystemActionResult{}, newError(ErrStaleLifecycleSnapshot,
				"confirmation token is missing, expired, or bound to an older inventory")
		}
		delete(m.confirmations, action)
	}

	res := SystemActionResult{Accepted: true}
	m.drainRequester = in.Requester
	if action == "restart" {
		deadline := now.Add(m.cfg.ReconnectGrace)
		m.restart = &RestartInfo{RestartID: "restart_" + randHex(12), RequestedAt: now, Deadline: deadline}
		res.RestartID = m.restart.RestartID
		res.ReconnectDeadline = &deadline
		m.setPhaseLocked(PhaseDrainingRestart)
	} else {
		m.setPhaseLocked(PhaseDrainingShutdown)
	}
	res.Snapshot = m.snapshotLocked()
	return res, nil
}

// DrainRequester returns the audit identity recorded when the current drain
// was accepted ("" for timer-driven drains like idle-grace expiry). Read it
// from the OnPhase drain handoff — the HTTP drain call site may lose the
// drainOnce race to the phase callback.
func (m *Manager) DrainRequester() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.drainRequester
}

// MarkStopped moves a draining phase to stopped after the runner finishes its
// bounded cleanup (stop-all, process sweep). No-op for other phases.
func (m *Manager) MarkStopped() {
	m.mu.Lock()
	if m.phase == PhaseDrainingShutdown || m.phase == PhaseDrainingRestart {
		m.setPhaseLocked(PhaseStopped)
	}
	m.mu.Unlock()
	m.drainEvents()
}

// SetUpdatePending flags the snapshot when runnerboot classified this runner
// as busy-stale — an update is waiting for an idle window (Task-416).
func (m *Manager) SetUpdatePending(pending bool) {
	m.mu.Lock()
	m.updatePending = pending
	m.mu.Unlock()
}

// ── evaluation / transitions ─────────────────────────────────────────────────

// evaluate refreshes workload, expires leases, and runs the state machine.
// Called by the sweeper and at the top of every mutating method so callers
// observe a current snapshot without depending on tick timing.
func (m *Manager) evaluate(ctx context.Context) {
	ws, werr := m.fetchWorkload(ctx)
	m.mu.Lock()
	if werr == nil {
		m.workload = ws
		m.workloadErr = nil
		m.recomputeInventoryLocked()
	} else {
		m.workloadErr = werr
	}
	now := m.now()
	m.expireLeasesLocked(now)
	m.transitionForStateLocked(now)
	m.mu.Unlock()
	m.drainEvents()
}

func (m *Manager) fetchWorkload(ctx context.Context) (WorkloadSnapshot, error) {
	if m.cfg.WorkSnapshot == nil {
		return WorkloadSnapshot{}, nil
	}
	return m.cfg.WorkSnapshot(ctx)
}

// transitionForStateLocked runs the closed state machine (SD-28 §5 mermaid)
// to a fixed point — one evaluate reaches the resting phase (e.g. a boot with
// no leases lands in idle_grace, not a one-step intermediate). Caller holds
// m.mu; the cap is a defensive bound, every edge strictly changes phase.
func (m *Manager) transitionForStateLocked(now time.Time) {
	for i := 0; i < 8; i++ {
		if !m.transitionOnceLocked(now) {
			return
		}
	}
}

func (m *Manager) transitionOnceLocked(now time.Time) bool {
	leases := len(m.leases)
	work := m.workload.ActiveCount()
	persistent := m.cfg.Mode == ModePersistent

	switch m.phase {
	case PhaseStarting:
		if leases > 0 {
			m.setPhaseLocked(PhaseReady)
			return true
		}
		if now.Sub(m.startedAt) >= m.cfg.BootGrace {
			if persistent {
				m.setPhaseLocked(PhaseReady)
			} else {
				m.enterIdleGraceLocked(now)
			}
			return true
		}
	case PhaseReady:
		if leases > 0 {
			return false
		}
		if work > 0 {
			m.setPhaseLocked(PhaseOrphanedWork)
			return true
		}
		if !persistent {
			m.enterIdleGraceLocked(now)
			return true
		}
	case PhaseOrphanedWork:
		if leases > 0 {
			m.setPhaseLocked(PhaseReady)
			return true
		}
		if work == 0 {
			if persistent {
				m.setPhaseLocked(PhaseReady)
			} else {
				m.enterIdleGraceLocked(now)
			}
			return true
		}
	case PhaseIdleGrace:
		if leases > 0 {
			m.idleDeadline = nil
			m.setPhaseLocked(PhaseReady)
			return true
		}
		if work > 0 {
			m.idleDeadline = nil
			m.setPhaseLocked(PhaseOrphanedWork)
			return true
		}
		if m.idleDeadline != nil && !now.Before(*m.idleDeadline) {
			m.setPhaseLocked(PhaseDrainingShutdown)
			return true
		}
	case PhaseDrainingRestart, PhaseDrainingShutdown, PhaseStopped:
		// Terminal-ish: only MarkStopped moves drain→stopped.
	}
	return false
}

func (m *Manager) enterIdleGraceLocked(now time.Time) {
	deadline := now.Add(m.cfg.IdleGrace)
	m.idleDeadline = &deadline
	m.setPhaseLocked(PhaseIdleGrace)
}

func (m *Manager) setPhaseLocked(p RunnerPhase) {
	if m.phase == p {
		return
	}
	m.phase = p
	if p != PhaseIdleGrace {
		m.idleDeadline = nil
	}
	m.events = append(m.events, p)
}

// expireLeasesLocked drops leases past TTL; removed ids count as released so
// later releases of the same id stay idempotent.
func (m *Manager) expireLeasesLocked(now time.Time) {
	for _, l := range m.leases {
		if !now.Before(l.ExpiresAt) {
			m.removeLeaseLocked(l)
		}
	}
}

func (m *Manager) removeLeaseLocked(l *ClientLease) {
	delete(m.leases, l.LeaseID)
	delete(m.byInstance, l.ClientInstanceID)
	m.releasedLeaseIDs[l.LeaseID] = true
	m.bumpInventoryLocked()
}

// authorizeLeaseLocked enforces instance → existence → generation → token, in
// that order (SD-28 §6.1 error contract: 404 lease_unknown, 409
// stale_generation, 403 invalid_lease_token).
func (m *Manager) authorizeLeaseLocked(leaseID, token, instanceID string, generation int) (*ClientLease, error) {
	if instanceID != "" && instanceID != m.cfg.RunnerInstanceID {
		return nil, newError(ErrStaleGeneration, "runnerInstanceId belongs to a different runner instance")
	}
	lease, ok := m.leases[leaseID]
	if !ok {
		return nil, newError(ErrLeaseUnknown, "lease is expired, released, or never existed")
	}
	if generation != 0 && generation != m.generation {
		return nil, newError(ErrStaleGeneration, fmt.Sprintf("generation %d is stale (current %d)", generation, m.generation))
	}
	if lease.Generation != m.generation {
		return nil, newError(ErrStaleGeneration, "lease belongs to an older generation")
	}
	if hashToken(token) != lease.TokenHash {
		return nil, newError(ErrInvalidLeaseToken, "leaseToken does not match this lease")
	}
	return lease, nil
}

// ── snapshot / inventory ─────────────────────────────────────────────────────

func (m *Manager) snapshotLocked() LifecycleSnapshot {
	clients := make([]ClientLeaseView, 0, len(m.leases))
	for _, l := range m.leases {
		clients = append(clients, viewOf(l))
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].LeaseID < clients[j].LeaseID })
	other := 0
	if len(clients) > 1 {
		other = len(clients) - 1
	}
	return LifecycleSnapshot{
		RunnerInstanceID:  m.cfg.RunnerInstanceID,
		Generation:        m.generation,
		ProtocolVersion:   m.cfg.ProtocolVersion,
		BuildID:           m.cfg.BuildID,
		Mode:              m.cfg.Mode,
		Phase:             m.phase,
		StartedAt:         m.startedAt,
		Clients:           clients,
		OtherClientCount:  other,
		Workload:          m.workload,
		IdleDeadline:      m.idleDeadline,
		Restart:           m.restart,
		UpdatePending:     m.updatePending,
		InventoryRevision: m.inventoryRev,
		ServerNow:         m.now(),
	}
}

// recomputeInventoryLocked bumps inventoryRevision whenever the client set or
// workload fingerprint changed since the last evaluation.
func (m *Manager) recomputeInventoryLocked() {
	h := m.computeInventoryHashLocked()
	if h != m.inventoryHash {
		m.inventoryHash = h
		m.bumpInventoryLocked()
	}
}

func (m *Manager) computeInventoryHashLocked() string {
	var sb strings.Builder
	ids := make([]string, 0, len(m.leases))
	for id := range m.leases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		sb.WriteString("L:")
		sb.WriteString(id)
		sb.WriteByte(';')
	}
	items := append([]WorkloadItem(nil), m.workload.Items...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].RunID < items[j].RunID
	})
	for _, it := range items {
		fmt.Fprintf(&sb, "W:%s|%s|%s|%s|;", it.Kind, it.RunID, it.ProviderKey, it.Detail)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

func (m *Manager) bumpInventoryLocked() { m.inventoryRev++ }

// drainEvents fires queued OnPhase callbacks outside the lock with a fresh
// snapshot per transition.
func (m *Manager) drainEvents() {
	if m.cfg.OnPhase == nil {
		m.mu.Lock()
		m.events = nil
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	events := m.events
	m.events = nil
	if len(events) == 0 {
		m.mu.Unlock()
		return
	}
	snap := m.snapshotLocked()
	m.mu.Unlock()
	for _, p := range events {
		m.cfg.OnPhase(p, snap)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("lifecycle: crypto/rand unavailable: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("lifecycle: crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(buf)
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func heartbeatIntervalMs(ttl time.Duration) int64 {
	// Heartbeat at ~1/3 TTL so a single lost beat never expires a live client.
	ms := ttl.Milliseconds() / 3
	if ms <= 0 {
		ms = DefaultHeartbeatPeriod.Milliseconds()
	}
	return ms
}
