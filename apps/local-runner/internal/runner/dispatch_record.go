package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DispatchProtocolV2 is the durable turn-dispatch protocol version (SD-24 / CP-51).
const DispatchProtocolV2 = 2

// DispatchState is the closed forward-only 8-state machine (SD-24 §5.1).
// There is no intent_pending state — before CreatePrepared the durable artifact
// is the outer intent itself.
type DispatchState string

const (
	DispatchPrepared          DispatchState = "prepared"
	DispatchSendClaimed       DispatchState = "send_claimed"
	DispatchSendStarted       DispatchState = "send_started"
	DispatchProviderAccepted  DispatchState = "provider_accepted"
	DispatchUncertain         DispatchState = "uncertain"
	DispatchTerminalCompleted DispatchState = "terminal_completed"
	DispatchTerminalFailed    DispatchState = "terminal_failed"
	DispatchTerminalCancelled DispatchState = "terminal_cancelled"
)

// SettlePhase is the gate-settlement sub-lifecycle on the same record (SD-24 §5.3).
// Values record the last completed phase (not "being attempted").
type SettlePhase string

const (
	SettleNone                   SettlePhase = ""
	SettlePending                SettlePhase = "settle_pending"
	SettleGateEvaluated          SettlePhase = "gate_evaluated"
	SettleCompletionCommitted    SettlePhase = "completion_committed"
	SettleGraphSettled           SettlePhase = "graph_settled"
	SettleDependentsReleased     SettlePhase = "dependents_released"
	SettleFinalized              SettlePhase = "finalized"
	SettleSupersededReprompt     SettlePhase = "settle_superseded_reprompt"
)

// StopOutcome records which Stop branch won linearization (SD-24 §7.2).
const (
	StopOutcomeStoppedBeforeSend  = "stopped_before_send"
	StopOutcomeCancelledInFlight  = "cancelled_in_flight"
)

// PreSendStopSource distinguishes own-run Stop from parent fence at pre-send cancel.
type PreSendStopSource string

const (
	PreSendStopSelf        PreSendStopSource = "self"
	PreSendStopParentFence PreSendStopSource = "parent_fence"
)

// RecoveryUnknownDecision is the result of CommitRecoveryUnknownOrRequireCancel.
type RecoveryUnknownDecision string

const (
	RecoveryClassifiedUncertain RecoveryUnknownDecision = "classified_uncertain"
	RecoveryCancelRequired      RecoveryUnknownDecision = "cancel_required"
)

// ResolveAction is the closed SS-17 operator action table (SD-24 §6.7).
type ResolveAction string

const (
	ResolveMarkCompleted     ResolveAction = "mark_completed"
	ResolveMarkFailed        ResolveAction = "mark_failed"
	ResolveConfirmCancelled  ResolveAction = "confirm_cancelled"
	ResolveAbandon           ResolveAction = "abandon"
)

// RepairAction / RepairOutcome for the two-phase repair lifecycle.
type RepairAction string

const (
	RepairActionRetryLoad RepairAction = "retry_load"
	RepairActionAbandon   RepairAction = "abandon"
)

type RepairOutcome string

const (
	RepairResolvedRetryLoad RepairOutcome = "resolved_retry_load"
	RepairFailedStillOpen   RepairOutcome = "failed_still_open"
	RepairResolvedAbandon   RepairOutcome = "resolved_abandon"
)

// ReleaseManifestState is the revisioned dependents-release lifecycle.
type ReleaseManifestState string

const (
	ReleasePending    ReleaseManifestState = "pending"
	ReleaseCreated    ReleaseManifestState = "created"
	ReleaseSuppressed ReleaseManifestState = "suppressed"
)

// DispatchRecord is the single durable source of truth for one turn's dispatch
// (SD-24 §5.3). Forward-only; EVERY mutation is a Revision CAS via DispatchStore.
type DispatchRecord struct {
	ProtocolVersion         int               `json:"protocol_version"`
	TurnID                  string            `json:"turn_id"`
	RunID                   string            `json:"run_id"`
	// ProjectID scopes the per-project dispatch.ndjson shard
	// (.flowpilot/chats/<project_id>/dispatch.ndjson) and Drive sync.
	ProjectID               string            `json:"project_id,omitempty"`
	IntentOwnerRunID        string            `json:"intent_owner_run_id,omitempty"`
	State                   DispatchState     `json:"state"`
	StopOutcome             string            `json:"stop_outcome,omitempty"`
	Revision                int64             `json:"revision"`
	ClaimOwner              string            `json:"claim_owner,omitempty"`
	ClaimExpiresAt          string            `json:"claim_expires_at,omitempty"`
	RecoveryAttachEpoch     int64             `json:"recovery_attach_epoch"`
	RecoveryAttachOwner     string            `json:"recovery_attach_owner,omitempty"`
	RecoveryAttachExpiresAt string            `json:"recovery_attach_expires_at,omitempty"`
	CancelRequested         bool              `json:"cancel_requested,omitempty"`
	StopGeneration          int64             `json:"stop_generation,omitempty"`
	OuterIntentKey          string            `json:"outer_intent_key,omitempty"`
	OuterIntentGen          int64             `json:"outer_intent_gen,omitempty"`
	EnvelopeHash            string            `json:"envelope_hash"`
	ReceiptEvidence         *ReceiptEvidence  `json:"receipt_evidence,omitempty"`
	TerminalEvidence        *TerminalEvidence `json:"terminal_evidence,omitempty"`
	SettleOwed              bool              `json:"settle_owed"`
	SettlePhase             SettlePhase       `json:"settle_phase,omitempty"`
	PredecessorTurnID       string            `json:"predecessor_turn_id,omitempty"`
	Outcome                 string            `json:"outcome,omitempty"`
	ParentStopFence         *ParentStopFence  `json:"parent_stop_fence,omitempty"`
	CreatedAt               string            `json:"created_at,omitempty"`
	UpdatedAt               string            `json:"updated_at,omitempty"`
}

// ReceiptEvidence is the canonical acceptance receipt (SD-24 §6.3a).
// Equality/conflict is over ProviderKey, ReceiptID, EvidenceKind and PayloadSHA256.
type ReceiptEvidence struct {
	ProviderKey          string `json:"provider_key"`
	ReceiptID            string `json:"receipt_id"`
	EvidenceKind         string `json:"evidence_kind"`
	PayloadCanonicalJSON []byte `json:"payload_canonical_json,omitempty"`
	PayloadSHA256        string `json:"payload_sha256"`
	ObservedAt           string `json:"observed_at,omitempty"`
}

// TerminalEvidence is required for every automatic post-send terminal commit.
// Outcome is the closed enum completed|failed|cancelled.
type TerminalEvidence struct {
	ProviderKey          string `json:"provider_key"`
	EvidenceKind         string `json:"evidence_kind"`
	Outcome              string `json:"outcome"` // completed|failed|cancelled
	ReceiptIdentity      string `json:"receipt_identity,omitempty"`
	PayloadCanonicalJSON []byte `json:"payload_canonical_json,omitempty"`
	PayloadSHA256        string `json:"payload_sha256"`
	ObservedAt           string `json:"observed_at,omitempty"`
}

// AttachedEffectPayload is a durable attached-stream event (SD-25).
type AttachedEffectPayload struct {
	Kind          string `json:"kind"`
	CanonicalJSON []byte `json:"canonical_json,omitempty"`
	SHA256        string `json:"sha256"`
	ObservedAt    string `json:"observed_at,omitempty"`
}

// ParentStopFence binds a released child to its parent RunStopState authority.
type ParentStopFence struct {
	ParentRunID            string `json:"parent_run_id"`
	ExpectedStopGeneration int64  `json:"expected_stop_generation"`
}

// RunStopState is the only durable run-Stop authority (SD-24 §5.4.1).
type RunStopState struct {
	RunID      string `json:"run_id"`
	Generation int64  `json:"generation"`
	Revision   int64  `json:"revision"`
	Stopped    bool   `json:"stopped"`
}

// StopReason is recorded on RequestRunStop audit.
type StopReason string

const (
	StopReasonUser      StopReason = "user"
	StopReasonParent    StopReason = "parent"
	StopReasonSystem    StopReason = "system"
	StopReasonRepair    StopReason = "repair"
)

// RecoveryAttachToken is a bounded attach ownership grant (SD-25).
type RecoveryAttachToken struct {
	Epoch     int64     `json:"epoch"`
	Owner     string    `json:"owner"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DispatchEnvelope is written ONCE at prepared and never mutated (SD-24 §5.4).
// Recovery re-sends from this, not from live intent fields.
type DispatchEnvelope struct {
	TurnID                     string           `json:"turn_id"`
	RunID                      string           `json:"run_id"`
	StepID                     string           `json:"step_id,omitempty"`
	ProviderKey                ProviderKey      `json:"provider_key"`
	ProviderAccountID          string           `json:"provider_account_id,omitempty"`
	ProviderSessionIDAtPrepare string           `json:"provider_session_id_at_prepare,omitempty"`
	PromptRef                  string           `json:"prompt_ref,omitempty"`
	PromptSHA256               string           `json:"prompt_sha256"`
	Model                      string           `json:"model,omitempty"`
	ReasoningEffort            string           `json:"reasoning_effort,omitempty"`
	Yolo                       bool             `json:"yolo,omitempty"`
	SelectedSkills             []SkillSelection `json:"selected_skills,omitempty"`
	Scenario                   string           `json:"scenario,omitempty"`
	Attachments                []AttachmentRef  `json:"attachments,omitempty"`
	FlowContextInjected        bool             `json:"flow_context_injected,omitempty"`
	CreatedAt                  string           `json:"created_at,omitempty"`
	EnvelopeHash               string           `json:"envelope_hash"`
}

// AttachmentRef is a path/ref + content hash for envelope binding.
type AttachmentRef struct {
	Ref    string `json:"ref"`
	SHA256 string `json:"sha256"`
}

// DurableIntent is the immutable child identity used by release manifests.
type DurableIntent struct {
	ChildTurnID     string            `json:"child_turn_id"`
	ChildRunID      string            `json:"child_run_id"`
	IntentKey       string            `json:"intent_key"`
	IntentGen       int64             `json:"intent_gen,omitempty"`
	Envelope        *DispatchEnvelope `json:"envelope,omitempty"`
	EnvelopeHash    string            `json:"envelope_hash"`
	ParentStopFence ParentStopFence   `json:"parent_stop_fence"`
	IntentHash      string            `json:"intent_hash"`
}

// ReleaseManifestItem is a revisioned dependents-release effect record.
type ReleaseManifestItem struct {
	RunID          string              `json:"run_id"`
	TurnID         string              `json:"turn_id"`
	DependentRunID string              `json:"dependent_run_id"`
	Intent         DurableIntent       `json:"intent"`
	State          ReleaseManifestState `json:"state"`
	Revision       int64               `json:"revision"`
	StopGeneration int64               `json:"stop_generation,omitempty"`
	CreatedAt      string              `json:"created_at,omitempty"`
	UpdatedAt      string              `json:"updated_at,omitempty"`
}

// OperatorEvidence is the operator assertion payload for ResolveUncertain.
type OperatorEvidence struct {
	Actor      string `json:"actor"`
	Detail     string `json:"detail,omitempty"`
	CapturedAt string `json:"captured_at,omitempty"`
}

// RepairRecord is the fail-closed quarantine lifecycle (SD-24 §6.7).
type RepairRecord struct {
	RunID            string `json:"run_id"`
	RepairRevision   int64  `json:"repair_revision"`
	Reason           string `json:"reason"`
	QuarantineBlob   []byte `json:"quarantine_blob,omitempty"`
	QuarantineHash   string `json:"quarantine_hash"`
	State            string `json:"state"` // open|resolved
	ResolvedAction   string `json:"resolved_action,omitempty"`
	ResolutionID     string `json:"resolution_id,omitempty"`
	AttemptClaim     string `json:"attempt_claim,omitempty"`
	AttemptExpiresAt string `json:"attempt_expires_at,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	ResolvedAt       string `json:"resolved_at,omitempty"`
}

// EffectDone is a settle effect ledger entry (audit/skip optimization).
type EffectDone struct {
	RunID       string `json:"run_id"`
	TurnID      string `json:"turn_id"`
	EffectKind  string `json:"effect_kind"`
	Payload     []byte `json:"payload,omitempty"`
	PayloadHash string `json:"payload_hash"`
	Revision    int64  `json:"revision"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// AttentionItem surfaces uncertain dispatches and open repairs (SS-17).
type AttentionItem struct {
	Kind       string `json:"kind"` // uncertain|repair_required|cancel_required|settle_pending
	RunID      string `json:"run_id"`
	TurnID     string `json:"turn_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// AuditEntry is an immutable audit row co-committed with store mutations.
type AuditEntry struct {
	Seq       int64  `json:"seq"`
	RunID     string `json:"run_id"`
	TurnID    string `json:"turn_id,omitempty"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail,omitempty"`
	At        string `json:"at"`
	Actor     string `json:"actor,omitempty"`
}

// ResolutionResult supports idempotent operator-resolution replay.
type ResolutionResult struct {
	ResolutionID string        `json:"resolution_id"`
	RunID        string        `json:"run_id"`
	TurnID       string        `json:"turn_id"`
	Action       string        `json:"action"`
	Revision     int64         `json:"revision"`
	NewTurnID    string        `json:"new_turn_id,omitempty"`
	Record       DispatchRecord `json:"record,omitempty"`
}

// Typed fence / attach / conflict errors (SD-24 / SD-25).
type ErrRunStopFence struct{ CurrentGeneration int64 }

func (e ErrRunStopFence) Error() string {
	return fmt.Sprintf("run stop fence: generation=%d", e.CurrentGeneration)
}

type ErrParentStopFence struct{ CurrentGeneration int64 }

func (e ErrParentStopFence) Error() string {
	return fmt.Sprintf("parent stop fence: generation=%d", e.CurrentGeneration)
}

var (
	ErrStaleDispatch         = errors.New("dispatch record revision is stale")
	ErrRecoveryLeaseLost     = errors.New("recovery lease lost or expired")
	ErrRecoveryAttachRevoked = errors.New("recovery attach token is revoked")
	ErrRecoveryAttachActive  = errors.New("recovery attach token remains active")
	ErrReceiptConflict       = errors.New("receipt identity payload conflict")
	ErrEffectConflict        = errors.New("effect payload hash conflict")
	ErrSuperseded            = errors.New("intent superseded by newer generation or envelope")
	ErrNotFound              = errors.New("dispatch record not found")
	ErrAlreadyExists         = errors.New("dispatch record already exists")
	ErrIllegalTransition     = errors.New("illegal dispatch transition")
	ErrRepairNotOpen         = errors.New("repair is not open")
	ErrDispatchLocked        = errors.New("dispatch store lock held by another process")
)

// dispatchLegal is the CLOSED forward-only edge set (SD-24 §5.2).
var dispatchLegal = map[DispatchState][]DispatchState{
	DispatchPrepared:         {DispatchSendClaimed, DispatchTerminalCancelled},
	DispatchSendClaimed:      {DispatchSendStarted, DispatchTerminalCancelled},
	DispatchSendStarted:      {DispatchProviderAccepted, DispatchUncertain, DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled},
	DispatchProviderAccepted: {DispatchUncertain, DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled},
	DispatchUncertain:        {DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled},
}

// settleLegal is the closed settle-phase edge set (SD-24 §5.3).
var settleLegal = map[SettlePhase][]SettlePhase{
	SettleNone:                {SettlePending},
	SettlePending:             {SettleGateEvaluated, SettleSupersededReprompt},
	SettleGateEvaluated:       {SettleCompletionCommitted, SettleSupersededReprompt},
	SettleCompletionCommitted: {SettleGraphSettled},
	SettleGraphSettled:        {SettleDependentsReleased},
	SettleDependentsReleased:  {SettleFinalized},
	// Superseded and finalized are terminal settle dispositions.
	SettleSupersededReprompt: {},
	SettleFinalized:          {},
}

// IsTerminal reports whether s is a terminal dispatch state.
func (s DispatchState) IsTerminal() bool {
	switch s {
	case DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled:
		return true
	}
	return false
}

// IsSent reports whether the provider may have received bytes (post-linearization).
func (s DispatchState) IsSent() bool {
	switch s {
	case DispatchSendStarted, DispatchProviderAccepted:
		return true
	}
	return false
}

// IsSettleFinal reports whether settle work is done (or never owed).
func (p SettlePhase) IsSettleFinal() bool {
	switch p {
	case SettleFinalized, SettleSupersededReprompt, SettleNone:
		return true
	}
	return false
}

// canTransition enforces the edge table PLUS CancelRequested invariants:
//   - CancelRequested ⇒ send_claimed→send_started is REJECTED
//   - CancelRequested in prepared/send_claimed ⇒ only terminal_cancelled is legal
func (r *DispatchRecord) canTransition(to DispatchState) error {
	if r == nil {
		return fmt.Errorf("%w: nil record", ErrIllegalTransition)
	}
	if r.CancelRequested && to == DispatchSendStarted {
		return fmt.Errorf("%w: cancel_requested blocks send_started (stop won linearization) turn=%s",
			ErrIllegalTransition, r.TurnID)
	}
	if r.CancelRequested && !r.State.IsTerminal() &&
		(r.State == DispatchPrepared || r.State == DispatchSendClaimed) &&
		to != DispatchTerminalCancelled {
		return fmt.Errorf("%w: cancel_requested in %s only permits terminal_cancelled turn=%s",
			ErrIllegalTransition, r.State, r.TurnID)
	}
	for _, ok := range dispatchLegal[r.State] {
		if ok == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s (turn %s)", ErrIllegalTransition, r.State, to, r.TurnID)
}

// canAdvanceSettle enforces the settle-phase edge table.
func canAdvanceSettle(from, to SettlePhase) error {
	for _, ok := range settleLegal[from] {
		if ok == to {
			return nil
		}
	}
	return fmt.Errorf("%w: settle %s -> %s", ErrIllegalTransition, from, to)
}

// RecoveryInferenceRule documents what each dispatch state / event proves for
// recovery. This is the single authority for DOD-G8 (EventTurnStarted is NOT
// recovery evidence — only DispatchRecord.State is).
//
//	State prepared / send_claimed  → safely-retryable (provably not sent)
//	State send_started / provider_accepted → reconcile or uncertain
//	State uncertain                → hold for operator (SS-17)
//	State terminal_*               → done; settle phase may still run
//	EventTurnStarted               → NOT recovery authority (emitted before go runTurn)
//	Bare idempotency value         → terminal_completed only with terminal/lastTurnID corroboration
func RecoveryInferenceRule() string {
	return "recovery_infers_only_from_DispatchRecord.State; EventTurnStarted is not recovery evidence"
}

// HashCanonicalJSON returns SHA-256 hex of canonical JSON for v.
func HashCanonicalJSON(v any) (string, []byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

// ComputeEnvelopeHash sets EnvelopeHash from all fields except EnvelopeHash itself.
func ComputeEnvelopeHash(env *DispatchEnvelope) (string, error) {
	if env == nil {
		return "", errors.New("nil envelope")
	}
	// Copy without hash for canonicalization.
	tmp := *env
	tmp.EnvelopeHash = ""
	// Stable skill / attachment order.
	if len(tmp.SelectedSkills) > 1 {
		sort.Slice(tmp.SelectedSkills, func(i, j int) bool {
			return tmp.SelectedSkills[i].Name < tmp.SelectedSkills[j].Name
		})
	}
	if len(tmp.Attachments) > 1 {
		sort.Slice(tmp.Attachments, func(i, j int) bool {
			return tmp.Attachments[i].Ref < tmp.Attachments[j].Ref
		})
	}
	h, _, err := HashCanonicalJSON(tmp)
	return h, err
}

// ComputeIntentHash binds a DurableIntent payload for release-manifest equality.
func ComputeIntentHash(intent *DurableIntent) (string, error) {
	if intent == nil {
		return "", errors.New("nil durable intent")
	}
	tmp := *intent
	tmp.IntentHash = ""
	h, _, err := HashCanonicalJSON(tmp)
	return h, err
}

// HashBytes returns SHA-256 hex of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// receiptEqual reports equality of identity+payload (ObservedAt is metadata only).
func receiptEqual(a, b *ReceiptEvidence) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ProviderKey == b.ProviderKey &&
		a.ReceiptID == b.ReceiptID &&
		a.EvidenceKind == b.EvidenceKind &&
		a.PayloadSHA256 == b.PayloadSHA256
}

// receiptIdentityMatch reports same identity fields (for conflict detection).
func receiptIdentityMatch(a, b *ReceiptEvidence) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ProviderKey == b.ProviderKey &&
		a.ReceiptID == b.ReceiptID &&
		a.EvidenceKind == b.EvidenceKind
}

// terminalStateFromProof maps TerminalEvidence.Outcome to a dispatch terminal state.
func terminalStateFromProof(outcome string) (DispatchState, string, error) {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "completed":
		return DispatchTerminalCompleted, "completed", nil
	case "failed":
		return DispatchTerminalFailed, "failed", nil
	case "cancelled", "canceled":
		return DispatchTerminalCancelled, "cancelled", nil
	default:
		return "", "", fmt.Errorf("invalid terminal outcome %q", outcome)
	}
}

// nowRFC3339Nano is the default wall clock helper for tests to override via store clock.
func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// requiresGateSettlement is the prepare-time settle decision (immutable SettleOwed).
//
// BUG-298: "chat" must be included. A chat-mode run using the built-in Review Loop
// orchestration is flagged flowEngineDriven=true (interactive_service.go) and goes
// through the exact same markPendingFlowGateSettleLocked/pendingFlowGateSettle gate
// as a flow/workflow run — but runMode stays "chat" (rs.runKind), so omitting it
// here left the dispatch ledger's SettleOwed permanently false for every chat-mode
// turn. That silently starved the live settle-drive scheduler
// (maybeScheduleSettleAfterTerminal) of the trigger it needs to re-run
// resumePendingFlowGate after a hub turn (e.g. submit_review_outcome) completes,
// so the run stuck at status=running forever — only a server restart's boot-time
// recovery (drivePendingSettlesOnBoot, which reads pendingFlowGateSettle directly
// and bypasses SettleOwed) ever resolved it. A plain (non-flow-engine-driven) chat
// run never arms pendingFlowGateSettle in the first place, so SettleOwed=true is
// harmless for it — the settle-drive finds status already Completed and allows
// through as normal bookkeeping.
func requiresGateSettlement(runMode, stepID string) bool {
	if strings.TrimSpace(stepID) == "" {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(runMode))
	return m == "flow" || m == "workflow" || m == "chat" || m == ""
}

// allDispatchStates returns every defined state for exhaustive tests.
func allDispatchStates() []DispatchState {
	return []DispatchState{
		DispatchPrepared,
		DispatchSendClaimed,
		DispatchSendStarted,
		DispatchProviderAccepted,
		DispatchUncertain,
		DispatchTerminalCompleted,
		DispatchTerminalFailed,
		DispatchTerminalCancelled,
	}
}

// isLegalEdge reports whether from→to is in the closed legal table (ignoring cancel).
func isLegalEdge(from, to DispatchState) bool {
	for _, ok := range dispatchLegal[from] {
		if ok == to {
			return true
		}
	}
	return false
}
