package runner

import (
	"context"
	"time"
)

// DispatchStore is the only mutation path for durable turn dispatch (SD-24 §6.1).
// All mutations are Revision CAS; stale writes return ErrStaleDispatch.
type DispatchStore interface {
	// create-if-absent; first CreatePrepared for a run also carries atomic V2 activation
	// and creates non-prunable RunStopState in the same commit (ledger V2A).
	CreatePrepared(ctx context.Context, rec DispatchRecord, env DispatchEnvelope) error

	CASAdvance(ctx context.Context, runID, turnID string, expectedRev int64,
		expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error)

	// Recovery-only: same CAS plus exact claim owner + unexpired store-clock lease.
	CASRecoveryAdvance(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
		expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error)

	CASAdvanceSettle(ctx context.Context, runID, turnID string, expectedRev int64,
		expected, next SettlePhase) (int64, error)

	ClaimRecovery(ctx context.Context, runID, turnID string, expectedRev int64, owner string, ttl time.Duration) (int64, error)

	SetCancelRequested(ctx context.Context, runID, turnID string, expectedRev, stopGen int64) (int64, error)

	GetRunStopState(ctx context.Context, runID string) (RunStopState, error)
	RequestRunStop(ctx context.Context, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error)

	// Cross-record atomic commits: dispatch transition + INTENT OWNER's intent clear + audit.
	CommitReceiptAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		receipt ReceiptEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error)

	CommitTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error)

	CommitRecoveredTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		leaseOwner string, proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error)

	CommitRecoveryUnknownOrRequireCancel(ctx context.Context, runID, turnID string, expectedRev int64,
		leaseOwner string) (RecoveryUnknownDecision, int64, error)

	ClaimRecoveryAttach(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string, ttl time.Duration) (RecoveryAttachToken, DispatchRecord, error)
	EnterRecoveryAttach(ctx context.Context, runID, turnID string, token RecoveryAttachToken) error
	RecordRecoveryAttachedEffect(ctx context.Context, runID, turnID string, token RecoveryAttachToken, eventID string, payload AttachedEffectPayload) (bool, error)
	CommitAttachedTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64, token RecoveryAttachToken,
		proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error)

	CommitPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error)

	CommitRecoveryPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
		intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error)

	ResolveUncertain(ctx context.Context, runID, turnID string, expectedRev int64,
		resolutionID string, action ResolveAction, evidence OperatorEvidence) (int64, error)

	// Supersede guard: CAS live intent gen + envelope hash; newer prompt ⇒ ErrSuperseded.
	RetryAsNew(ctx context.Context, runID, oldTurnID string, expectedRev int64,
		resolutionID, newTurnID string, expectedIntentGen int64, expectedEnvelopeHash string) (int64, error)

	RecordEffectDone(ctx context.Context, runID, turnID, effectKind string, payload []byte, payloadHash string) (int64, error)
	CreateReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, intent DurableIntent) (ReleaseManifestItem, error)
	CommitReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev int64, child DispatchRecord, env DispatchEnvelope) (int64, error)
	SuppressReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev, stopGen int64) (int64, error)

	OpenRepair(ctx context.Context, runID, reason string, rawBlob []byte, rawHash string) (int64, error)
	GetOpenRepair(ctx context.Context, runID string) (RepairRecord, bool, error)
	BeginRepairResolution(ctx context.Context, runID string, expectedRepairRev int64,
		resolutionID string, action RepairAction) (int64, []byte, error)
	CommitRepairResolution(ctx context.Context, runID string, attemptRev int64,
		resolutionID string, outcome RepairOutcome, detail string) (int64, error)

	// Run protocol authority (non-prunable activation entry).
	GetRunProtocolVersion(ctx context.Context, runID string) (int, error)

	// Read side.
	Get(ctx context.Context, runID, turnID string) (DispatchRecord, int64, error)
	GetEnvelope(ctx context.Context, runID, turnID string) (DispatchEnvelope, error)
	ListRecoverable(ctx context.Context, runID string) ([]DispatchRecord, error)
	FindActiveByOuterIntent(ctx context.Context, runID, intentKey string, intentGen int64) (DispatchRecord, bool, error)
	ListAttention(ctx context.Context) ([]AttentionItem, error)
	ListAudit(ctx context.Context, runID, turnID string) ([]AuditEntry, error)
	ListEffects(ctx context.Context, runID, turnID string) ([]EffectDone, error)
	GetResolutionResult(ctx context.Context, resolutionID string) (ResolutionResult, bool, error)
	IsIntentCleared(ctx context.Context, ownerRunID, intentKey string, intentGen int64) (bool, error)

	// HasNonTerminal reports whether a run has any non-terminal or unfinalized-settle record
	// (blocks 90-day session prune).
	HasNonTerminal(ctx context.Context, runID string) (bool, error)
}

// IntentClearCallback is invoked after a durable intent-clear commit so the
// InteractiveService can update RAM owner fields under its lock.
type IntentClearCallback func(ownerRunID, intentKey string, intentGen int64)

// intentClearKey is the durable clear-set identity.
type intentClearKey struct {
	Owner string
	Key   string
	Gen   int64
}

func intentKeyOf(owner, key string, gen int64) intentClearKey {
	return intentClearKey{Owner: owner, Key: key, Gen: gen}
}

func recKey(runID, turnID string) string {
	return runID + "\x00" + turnID
}

func effectKey(runID, turnID, kind string) string {
	return runID + "\x00" + turnID + "\x00" + kind
}

func releaseEffectKind(dependentRunID string) string {
	return "release:" + dependentRunID
}

func attachedEffectKind(epoch int64, eventID string) string {
	return "attached:" + itoa64(epoch) + ":" + eventID
}

func itoa64(n int64) string {
	// small helper without strconv import cycle concerns in tiny files
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
