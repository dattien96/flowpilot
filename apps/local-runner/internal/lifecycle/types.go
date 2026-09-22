// Package lifecycle is the runner-owned shared-lifecycle authority (CP-81,
// SS-24, SD-28). It tracks generation-scoped client leases, heartbeat TTL
// expiry, idle grace, workload-aware phase transitions, two-phase destructive
// confirmations, and restart handoff metadata.
//
// The package is deliberately storage-free and process-free: it never calls
// os.Exit, never spawns processes, and never serves HTTP. It returns intents
// and fires phase callbacks; internal/cli and internal/runner act on them.
package lifecycle

import (
	"time"
)

// ClientKind identifies which front-end owns a lease (SD-28 §5).
type ClientKind string

const (
	ClientKindTUI        ClientKind = "tui"
	ClientKindDesktop    ClientKind = "desktop"
	ClientKindSupervisor ClientKind = "supervisor"
)

// LifecycleMode is the runner boot mode (SD-28 §5). "persistent" runners never
// idle-exit; "client-managed" runners idle-exit when the last lease leaves and
// no protected work remains; "supervised" is client-managed but launched by
// scripts/supervisor.js (Task-419) — same lease semantics, distinct telemetry.
type LifecycleMode string

const (
	ModePersistent    LifecycleMode = "persistent"
	ModeClientManaged LifecycleMode = "client-managed"
	ModeSupervised    LifecycleMode = "supervised"
)

// RunnerPhase is the closed lifecycle state machine (SD-28 §5 mermaid).
type RunnerPhase string

const (
	PhaseStarting         RunnerPhase = "starting"
	PhaseReady            RunnerPhase = "ready"
	PhaseIdleGrace        RunnerPhase = "idle_grace"
	PhaseOrphanedWork     RunnerPhase = "orphaned_work"
	PhaseDrainingRestart  RunnerPhase = "draining_restart"
	PhaseDrainingShutdown RunnerPhase = "draining_shutdown"
	PhaseStopped          RunnerPhase = "stopped"
)

// IsDraining reports whether the phase rejects new leases/work.
func (p RunnerPhase) IsDraining() bool {
	return p == PhaseDrainingRestart || p == PhaseDrainingShutdown || p == PhaseStopped
}

// Typed lifecycle error codes (Task-414 T-1). Handlers map them to HTTP status.
const (
	ErrLeaseUnknown            = "lease_unknown"
	ErrLeaseAlreadyReleased    = "lease_already_released"
	ErrInvalidLeaseToken       = "invalid_lease_token"
	ErrStaleGeneration         = "stale_generation"
	ErrRunnerDraining          = "runner_draining"
	ErrConfirmationRequired    = "lifecycle_confirmation_required"
	ErrStaleLifecycleSnapshot  = "stale_lifecycle_snapshot"
	ErrInvalidLifecycleRequest = "invalid_lifecycle_request"
)

// Error is the typed lifecycle error surface. Raw tokens are never embedded.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func newError(code, msg string) *Error { return &Error{Code: code, Message: msg} }

// ClientLease is the stored lease record (SD-28 §5). TokenHash holds the
// SHA-256 hex of the raw leaseToken — the raw token exists only in the
// register response and in client memory (T-2).
type ClientLease struct {
	LeaseID          string
	TokenHash        string
	ClientInstanceID string
	Kind             ClientKind
	PID              int
	Label            string
	ProjectPath      string
	ProtocolVersion  int
	BuildID          string
	Generation       int
	CreatedAt        time.Time
	LastHeartbeatAt  time.Time
	ExpiresAt        time.Time
}

// ClientLeaseView is the wire-facing lease projection. It deliberately has no
// token material — raw tokens never leave the manager after issuance.
type ClientLeaseView struct {
	LeaseID          string     `json:"leaseId"`
	ClientInstanceID string     `json:"clientInstanceId"`
	Kind             ClientKind `json:"kind"`
	PID              int        `json:"pid"`
	Label            string     `json:"label"`
	ProjectPath      string     `json:"projectPath,omitempty"`
	ProtocolVersion  int        `json:"protocolVersion,omitempty"`
	BuildID          string     `json:"buildId,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	LastHeartbeatAt  time.Time  `json:"lastHeartbeatAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
}

func viewOf(l *ClientLease) ClientLeaseView {
	return ClientLeaseView{
		LeaseID:          l.LeaseID,
		ClientInstanceID: l.ClientInstanceID,
		Kind:             l.Kind,
		PID:              l.PID,
		Label:            l.Label,
		ProjectPath:      l.ProjectPath,
		ProtocolVersion:  l.ProtocolVersion,
		BuildID:          l.BuildID,
		CreatedAt:        l.CreatedAt,
		LastHeartbeatAt:  l.LastHeartbeatAt,
		ExpiresAt:        l.ExpiresAt,
	}
}

// WorkloadItem is one unit of protected work (SD-28 §5). Warm provider
// sessions/pools are NOT items — only actively executing or resumable work.
type WorkloadItem struct {
	Kind        string    `json:"kind"` // "turn"|"flow"|"agent"|"scaffold"|"provider_exec"
	RunID       string    `json:"runId,omitempty"`
	ProjectID   string    `json:"projectId,omitempty"`
	ProviderKey string    `json:"providerKey,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	Cancellable bool      `json:"cancellable"`
	Detail      string    `json:"detail,omitempty"`
}

// WorkloadSnapshot is the protected-work inventory the manager evaluates. The
// runner supplies it via Config.WorkSnapshot; the manager fingerprints it into
// InventoryRevision so confirmation tokens bind to what the user saw.
type WorkloadSnapshot struct {
	Items []WorkloadItem `json:"items"`
}

// ActiveCount is the number of protected items.
func (w WorkloadSnapshot) ActiveCount() int { return len(w.Items) }

// RestartInfo rides the snapshot while phase == draining_restart (D-5).
// Clients that observe it enter reconnect mode until Deadline.
type RestartInfo struct {
	RestartID   string    `json:"restartId"`
	RequestedAt time.Time `json:"requestedAt"`
	Deadline    time.Time `json:"deadline"`
}

// LifecycleSnapshot is the full lifecycle state returned by GET
// /system/lifecycle and embedded in register/heartbeat/release/system-action
// responses (SD-28 §5, §6).
type LifecycleSnapshot struct {
	RunnerInstanceID  string            `json:"runnerInstanceId"`
	Generation        int               `json:"generation"`
	ProtocolVersion   int               `json:"protocolVersion"`
	BuildID           string            `json:"buildId"`
	Mode              LifecycleMode     `json:"lifecycleMode"`
	Phase             RunnerPhase       `json:"phase"`
	StartedAt         time.Time         `json:"startedAt"`
	Clients           []ClientLeaseView `json:"clients"`
	OtherClientCount  int               `json:"otherClientCount"`
	Workload          WorkloadSnapshot  `json:"workload"`
	IdleDeadline      *time.Time        `json:"idleDeadline,omitempty"`
	Restart           *RestartInfo      `json:"restart,omitempty"`
	UpdatePending     bool              `json:"updatePending"`
	InventoryRevision int64             `json:"inventoryRevision"`
	ServerNow         time.Time         `json:"serverNow"`
}

// RegisterInput is POST /system/clients/register (SD-28 §6.1).
type RegisterInput struct {
	Kind             ClientKind
	ClientInstanceID string
	PID              int
	Label            string
	ProtocolVersion  int
	BuildID          string
	ProjectPath      string
}

// RegisterResult is returned once per registration. LeaseToken is the raw
// secret — minted here, returned here, stored hashed; it is never logged.
// Idempotent is true when the clientInstanceID already held a lease this
// generation (HTTP 200) versus a fresh lease (HTTP 201).
type RegisterResult struct {
	LeaseID             string
	LeaseToken          string
	RunnerInstanceID    string
	Generation          int
	HeartbeatIntervalMs int64
	TTLMs               int64
	Idempotent          bool
	Snapshot            LifecycleSnapshot
}

// HeartbeatInput is POST /system/clients/{leaseId}/heartbeat.
type HeartbeatInput struct {
	LeaseID          string
	LeaseToken       string
	RunnerInstanceID string
	Generation       int
}

// ReleaseInput is POST /system/clients/{leaseId}/release.
type ReleaseInput struct {
	LeaseID          string
	LeaseToken       string
	RunnerInstanceID string
	Generation       int
}

// SystemActionInput is POST /system/shutdown | /system/restart (SD-28 §6.1).
type SystemActionInput struct {
	RequesterLeaseID   string
	LeaseToken         string
	ExpectedInstanceID string
	Reason             string // "user_exit"|"system_control"|"stale_build"|"supervisor"|"signal"
	// Requester is the audit identity of the caller (e.g. CA-913
	// describeRequester output, "signal", "idle-timeout"). It is recorded at
	// drain decision and surfaced via DrainRequester so the fenced supervisor
	// command keeps real attribution even when OnPhase won the drain race.
	Requester    string
	Confirm      bool
	ConfirmToken string
	// Force bypasses the two-phase confirmation (second SIGINT inside the
	// 5s warn window, fenced supervisor commands). It never bypasses the
	// expectedInstanceId fence.
	Force bool
}

// SystemActionResult is the two-phase destructive-action outcome (D-4).
// ConfirmationRequired means the caller must re-issue with Confirm=true and
// the returned raw ConfirmationToken before ConfirmationExpiresAt.
type SystemActionResult struct {
	Accepted              bool
	ConfirmationRequired  bool
	ConfirmationToken     string // raw — returned once, stored hashed
	ConfirmationExpiresAt time.Time
	RestartID             string
	ReconnectDeadline     *time.Time
	Snapshot              LifecycleSnapshot
}
