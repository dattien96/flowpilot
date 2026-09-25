package runner

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// CP-84 / Task-430 — DecisionPayload contract
//
// DecisionPayload is the durable, versioned, per-kind projection of every
// record that is waiting on a human decision for a run. It is DERIVED from the
// authoritative records (ProviderApprovalState / ProviderQuestionState durable
// shards, rs.pendingGateBlock, vibe gate fields, ssLockGate, worktreeBinding,
// rs.dispatch mirror) — never a parallel state authority (AGENTS §1).
//
// The mux event plane (Task-429) ships these in `run_decisions` frames; the
// desktop attention inbox renders them without a per-run REST round-trip.
// Payloads are bounded + redacted so an inbox render can never leak secrets or
// carry unbounded model output.
// ============================================================================

// DecisionPayloadVersion is the schema version carried on every payload.
// Unknown/future versions are tolerated by readers that only need id+kind;
// writers always stamp the current version.
const DecisionPayloadVersion = 1

// DecisionKind enumerates the closed set of decision surfaces (Task-430 T-1).
type DecisionKind string

const (
	DecisionKindApproval  DecisionKind = "approval"
	DecisionKindQuestion  DecisionKind = "question"
	DecisionKindGate      DecisionKind = "gate"
	DecisionKindSSLock    DecisionKind = "ss_lock"
	DecisionKindWorktree  DecisionKind = "worktree_merge"
	DecisionKindDispatch  DecisionKind = "dispatch_attention"
	DecisionKindRRequires DecisionKind = "r_requirement"
)

// Bounds (Task-430 T-3): every user/model-derived string field is truncated so
// a frame stays small enough to broadcast for every run in a project.
const (
	decisionFieldMaxLen     = 240
	decisionPromptMaxLen    = 600
	decisionQuickViewMaxLen = 1500
	decisionListMaxItems    = 24
)

// DecisionPayload is one actionable (or recently non-actionable marker) record.
// Revision is an opaque equality token — the client resubmits it on action and
// a mismatch yields 409 from the underlying endpoint. Sources are durable:
// approval/question Revision is a persisted counter; dispatch Revision is the
// record's CAS revision; gate/ss-lock revisions derive from arming stamps.
type DecisionPayload struct {
	Version     int          `json:"version"`
	ID          string       `json:"id"`
	RunID       string       `json:"runId"`
	Kind        DecisionKind `json:"kind"`
	Revision    string       `json:"revision"`
	Status      string       `json:"status"` // pending | resolving | marker
	Actionable  bool         `json:"actionable"`
	CreatedAt   string       `json:"createdAt,omitempty"`
	ExpiresAt   string       `json:"expiresAt,omitempty"`
	ProviderKey ProviderKey  `json:"providerKey,omitempty"`
	TurnID      string       `json:"turnId,omitempty"`
	Prompt      string       `json:"prompt,omitempty"`

	Approval *DecisionApprovalPayload `json:"approval,omitempty"`
	Question *DecisionQuestionPayload `json:"question,omitempty"`
	Gate     *DecisionGatePayload     `json:"gate,omitempty"`
	SSLock   *DecisionSSLockPayload   `json:"ssLock,omitempty"`
	Worktree *DecisionWorktreePayload `json:"worktree,omitempty"`
	Dispatch *DecisionDispatchPayload `json:"dispatch,omitempty"`
}

type DecisionApprovalPayload struct {
	Command   string                   `json:"command,omitempty"`
	Cwd       string                   `json:"cwd,omitempty"`
	Reason    string                   `json:"reason,omitempty"`
	Kind      string                   `json:"kind,omitempty"`
	Decisions []ApprovalDecisionOption `json:"decisions,omitempty"`
}

type DecisionQuestionPayload struct {
	Options     []QuestionOption `json:"options,omitempty"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
	// Quota is the structured candidate table when the question is a
	// quota_route_required gate (Task-450) — nil on ordinary questions.
	Quota *QuotaRouteDecision `json:"quota,omitempty"`
}

type DecisionGatePayload struct {
	Options        []string `json:"options,omitempty"`
	RegressedTests []string `json:"regressedTests,omitempty"`
	StepID         string   `json:"stepId,omitempty"`
	ResumeFrom     string   `json:"resumeFrom,omitempty"`
}

type DecisionSSLockPayload struct {
	FeatureBase string `json:"featureBase,omitempty"`
	DraftSSPath string `json:"draftSsPath,omitempty"`
	DraftSDPath string `json:"draftSdPath,omitempty"`
	// QuickView is the bounded head of the draft SS so the inbox can render a
	// preview without loading the full document.
	QuickView string `json:"quickView,omitempty"`
}

type DecisionWorktreePayload struct {
	Path          string   `json:"path,omitempty"`
	Branch        string   `json:"branch,omitempty"`
	BaseCommit    string   `json:"baseCommit,omitempty"`
	ConflictPaths []string `json:"conflictPaths,omitempty"`
	PatchRef      string   `json:"patchRef,omitempty"`
}

type DecisionDispatchPayload struct {
	TurnID        string `json:"turnId,omitempty"`
	AttentionKind string `json:"attentionKind,omitempty"` // uncertain | repair_required | cancel_required | settle_pending
	Reason        string `json:"reason,omitempty"`
}

// decisionPayloadsForRunLocked projects every pending decision for rs.
// Caller holds s.mu. Pure read — never mutates records.
func (s *InteractiveService) decisionPayloadsForRunLocked(rs *interactiveRun) []DecisionPayload {
	if rs == nil {
		return nil
	}
	out := make([]DecisionPayload, 0, 4)
	for _, rec := range s.approvals {
		if rec == nil || rec.runID != rs.id {
			continue
		}
		if rec.status != "pending" && rec.status != "resolving" {
			continue
		}
		out = append(out, approvalDecisionPayload(rs, rec))
	}
	for _, rec := range s.questions {
		if rec == nil || rec.runID != rs.id {
			continue
		}
		if rec.status != "pending" && rec.status != "resolving" {
			continue
		}
		out = append(out, questionDecisionPayload(rs, rec))
	}
	if d, ok := gateDecisionPayload(rs); ok {
		out = append(out, d)
	}
	if d, ok := s.ssLockDecisionPayload(rs); ok {
		out = append(out, d)
	}
	if d, ok := worktreeDecisionPayload(rs); ok {
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// decisionFingerprint is the cheap change-detector for the mux flusher: it
// changes iff the projected actionable decision set changes.
func decisionFingerprint(ds []DecisionPayload) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.ID)
		b.WriteByte('|')
		b.WriteString(d.Revision)
		b.WriteByte('|')
		b.WriteString(d.Status)
		b.WriteByte(';')
	}
	return b.String()
}

func approvalDecisionPayload(rs *interactiveRun, rec *approvalRecord) DecisionPayload {
	d := DecisionPayload{
		Version:     DecisionPayloadVersion,
		ID:          "approval:" + rec.id,
		RunID:       rs.id,
		Kind:        DecisionKindApproval,
		Revision:    strconv.FormatInt(rec.revision, 10),
		Status:      rec.status,
		Actionable:  rec.status == "pending",
		CreatedAt:   rec.createdAt,
		ExpiresAt:   rec.expiresAt,
		ProviderKey: rs.providerKey,
		TurnID:      rs.currentTurnID,
		Prompt:      boundDecisionText(decisionFirstNonEmpty(rec.details.Reason, rec.details.Command, "Approval required"), decisionPromptMaxLen),
		Approval: &DecisionApprovalPayload{
			Command:   boundDecisionText(rec.details.Command, decisionFieldMaxLen*4),
			Cwd:       boundDecisionText(rec.details.Cwd, decisionFieldMaxLen),
			Reason:    boundDecisionText(rec.details.Reason, decisionFieldMaxLen*2),
			Kind:      rec.details.Kind,
			Decisions: append([]ApprovalDecisionOption(nil), rec.details.Decisions...),
		},
	}
	return d
}

func questionDecisionPayload(rs *interactiveRun, rec *questionRecord) DecisionPayload {
	opts := make([]QuestionOption, 0, len(rec.options))
	for _, o := range rec.options {
		opts = append(opts, QuestionOption{
			Label:       boundDecisionText(o.Label, decisionFieldMaxLen),
			Description: boundDecisionText(o.Description, decisionFieldMaxLen),
			Value:       o.Value,
		})
	}
	return DecisionPayload{
		Version:     DecisionPayloadVersion,
		ID:          "question:" + rec.id,
		RunID:       rs.id,
		Kind:        DecisionKindQuestion,
		Revision:    strconv.FormatInt(rec.revision, 10),
		Status:      rec.status,
		Actionable:  rec.status == "pending",
		CreatedAt:   rec.createdAt,
		ExpiresAt:   rec.expiresAt,
		ProviderKey: rs.providerKey,
		TurnID:      rs.currentTurnID,
		Prompt:      boundDecisionText(rec.prompt, decisionPromptMaxLen),
		Question:    &DecisionQuestionPayload{Options: opts, MultiSelect: rec.multiSelect, Quota: quotaDecisionForRecord(rec)},
	}
}

// gateDecisionPayload covers the two parked-gate surfaces: the post-turn flow
// gate r-reg card (rs.pendingGateBlock) and the vibe user.confirm locks
// (sprint-boundary / resume-confirm), mirroring runSnapshot's PendingGate
// projection order.
func gateDecisionPayload(rs *interactiveRun) (DecisionPayload, bool) {
	if rs.pendingGateBlock != nil {
		info := rs.pendingGateBlock
		return DecisionPayload{
			Version:     DecisionPayloadVersion,
			ID:          "gate:" + rs.id,
			RunID:       rs.id,
			Kind:        DecisionKindGate,
			Revision:    info.createdAt,
			Status:      "pending",
			Actionable:  true,
			CreatedAt:   info.createdAt,
			ProviderKey: rs.providerKey,
			TurnID:      rs.currentTurnID,
			Prompt:      boundDecisionText(decisionFirstNonEmpty(info.message, "Flow gate blocked"), decisionPromptMaxLen),
			Gate: &DecisionGatePayload{
				Options:        append([]string(nil), info.options...),
				RegressedTests: boundStringList(info.regressedTests, decisionListMaxItems, decisionFieldMaxLen),
				StepID:         info.stepID,
			},
		}, true
	}
	if rs.vibeSprintBoundaryPending {
		return vibeGateDecision(rs, "vibe-gate-boundary:"+rs.id,
			vibeSprintBoundaryResumeLabel(rs.vibeTaskPlan, rs.vibeSprintIndex, rs.vibeSprintBoundaryTask),
			"Sprint boundary reached — continue to next sprint?"), true
	}
	if rs.vibeResumeConfirm {
		return vibeGateDecision(rs, "vibe-gate-resume:"+rs.id,
			rs.vibeResumeFromNode, "Resume from checkpoint?"), true
	}
	return DecisionPayload{}, false
}

func vibeGateDecision(rs *interactiveRun, id, resumeFrom, prompt string) DecisionPayload {
	return DecisionPayload{
		Version:     DecisionPayloadVersion,
		ID:          id,
		RunID:       rs.id,
		Kind:        DecisionKindGate,
		Revision:    fmt.Sprintf("vibe:%d:%s", rs.vibeSprintIndex, resumeFrom),
		Status:      "pending",
		Actionable:  true,
		ProviderKey: rs.providerKey,
		Prompt:      prompt,
		Gate:        &DecisionGatePayload{Options: []string{"ok", "cancel"}, ResumeFrom: resumeFrom},
	}
}

// ssLockDecisionPayload surfaces the Task-333 standardize SS-Lock gate. The
// gate record is process-local (synthetic standardize runs are not in s.runs),
// so a reconstructed run in waiting_user_confirm without a live gate yields a
// non-actionable marker — the user must open the run, matching today's gate.
func (s *InteractiveService) ssLockDecisionPayload(rs *interactiveRun) (DecisionPayload, bool) {
	g := s.ssLockGateFor(rs.id)
	if g == nil {
		if rs.status != RunStatus(ssLockWaiting) {
			return DecisionPayload{}, false
		}
		return DecisionPayload{
			Version:    DecisionPayloadVersion,
			ID:         "ss_lock:" + rs.id,
			RunID:      rs.id,
			Kind:       DecisionKindSSLock,
			Revision:   "marker:" + rs.updatedAt,
			Status:     "marker",
			Actionable: false,
			Prompt:     "SS-Lock gate is awaiting confirmation (open the run to decide)",
			SSLock:     &DecisionSSLockPayload{},
		}, true
	}
	g.mu.Lock()
	status := g.status
	base := g.featureBase
	ssPath := g.draftSSPath
	sdPath := g.draftSDPath
	quick := g.draftSSContent
	g.mu.Unlock()
	if status != ssLockWaiting {
		return DecisionPayload{}, false
	}
	return DecisionPayload{
		Version:    DecisionPayloadVersion,
		ID:         "ss_lock:" + rs.id,
		RunID:      rs.id,
		Kind:       DecisionKindSSLock,
		Revision:   "ss:" + status + ":" + ssPath,
		Status:     "pending",
		Actionable: strings.TrimSpace(quick) != "",
		CreatedAt:  rs.createdAt,
		Prompt:     boundDecisionText("Confirm System Specs draft for "+base, decisionPromptMaxLen),
		SSLock: &DecisionSSLockPayload{
			FeatureBase: base,
			DraftSSPath: ssPath,
			DraftSDPath: sdPath,
			QuickView:   boundDecisionText(quickViewHead(quick, 40), decisionQuickViewMaxLen),
		},
	}, true
}

// worktreeDecisionPayload surfaces a merge_pending binding (CP-71/CP-83). The
// conflict list + patch ref come from the latest merge-requested event input,
// which is durable (events sidecar survives restart).
func worktreeDecisionPayload(rs *interactiveRun) (DecisionPayload, bool) {
	if rs.worktree == nil || rs.worktree.State != "merge_pending" {
		return DecisionPayload{}, false
	}
	var conflicts []string
	patchRef := ""
	for i := len(rs.events) - 1; i >= 0; i-- {
		ev := rs.events[i]
		if ev.Type != EventWorktreeMergeRequested {
			continue
		}
		if m, ok := ev.Input.(map[string]any); ok {
			if v, ok := m["conflict_paths"].([]string); ok {
				conflicts = v
			} else if v, ok := m["conflictPaths"].([]string); ok {
				conflicts = v
			}
			// The merge-requested emitter writes `patchArtifactRef`
			// (run_worktree_merge.go); keep the snake/short spellings so
			// durable events written by any older build still project.
			if v, ok := m["patchArtifactRef"].(string); ok {
				patchRef = v
			} else if v, ok := m["patch_ref"].(string); ok {
				patchRef = v
			} else if v, ok := m["patchRef"].(string); ok {
				patchRef = v
			}
		}
		break
	}
	return DecisionPayload{
		Version:     DecisionPayloadVersion,
		ID:          "worktree_merge:" + rs.id,
		RunID:       rs.id,
		Kind:        DecisionKindWorktree,
		Revision:    "wt:" + rs.worktree.State + ":" + rs.updatedAt,
		Status:      "pending",
		Actionable:  true,
		CreatedAt:   rs.updatedAt,
		ProviderKey: rs.providerKey,
		Prompt:      "Worktree merge pending — apply patch, keep branch, or discard",
		Worktree: &DecisionWorktreePayload{
			Path:          rs.worktree.Path,
			Branch:        rs.worktree.Branch,
			BaseCommit:    rs.worktree.BaseCommit,
			ConflictPaths: boundStringList(conflicts, decisionListMaxItems, decisionFieldMaxLen),
			PatchRef:      boundDecisionText(patchRef, decisionFieldMaxLen),
		},
	}, true
}

// dispatchDecisionFromAttention converts one durable dispatch AttentionItem
// (the dispatchStore is the authority — rs.dispatch is a mirror written by
// turn goroutines outside s.mu and must not be iterated here) into a decision
// payload. Turn-scoped kinds key on turn id; repairs key on run id.
func dispatchDecisionFromAttention(rs *interactiveRun, item AttentionItem) DecisionPayload {
	id := "dispatch:" + item.TurnID
	if item.Kind == "repair_required" || item.TurnID == "" {
		id = "dispatch-repair:" + item.RunID
	}
	return DecisionPayload{
		Version:     DecisionPayloadVersion,
		ID:          id,
		RunID:       rs.id,
		Kind:        DecisionKindDispatch,
		Revision:    strconv.FormatInt(item.Revision, 10),
		Status:      "pending",
		Actionable:  item.Kind != "settle_pending", // settle is operational, not a human decision
		CreatedAt:   item.UpdatedAt,
		ProviderKey: rs.providerKey,
		TurnID:      item.TurnID,
		Prompt:      boundDecisionText(decisionFirstNonEmpty(item.Reason, "dispatch attention required"), decisionPromptMaxLen),
		Dispatch: &DecisionDispatchPayload{
			TurnID:        item.TurnID,
			AttentionKind: item.Kind,
			Reason:        boundDecisionText(item.Reason, decisionFieldMaxLen*2),
		},
	}
}

// ---- bounding / redaction helpers ------------------------------------------

var decisionRedactKeys = []string{
	"token", "secret", "password", "passwd", "api_key", "apikey",
	"authorization", "auth", "cookie", "credential", "private_key",
}

// boundDecisionText redacts credential-shaped fragments then truncates.
func boundDecisionText(s string, max int) string {
	s = redactDecisionText(strings.TrimSpace(s))
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func boundStringList(in []string, maxItems, maxLen int) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(in), maxItems))
	for i, v := range in {
		if i >= maxItems {
			break
		}
		out = append(out, boundDecisionText(v, maxLen))
	}
	return out
}

// redactDecisionText masks `KEY=value` / `KEY: value` fragments whose key
// looks credential-shaped so an inbox preview can never leak a secret that a
// model embedded in a command/prompt (Task-430 T-3).
func redactDecisionText(s string) string {
	if s == "" {
		return s
	}
	fields := strings.Fields(s)
	for i, f := range fields {
		for _, k := range decisionRedactKeys {
			lf := strings.ToLower(f)
			if strings.HasPrefix(lf, k+"=") || strings.HasPrefix(lf, k+":") || strings.HasPrefix(lf, "--"+k+"=") {
				idx := strings.IndexAny(f, "=:")
				if idx > 0 && idx+1 < len(f) {
					fields[i] = f[:idx+1] + "***"
				}
			}
		}
	}
	return strings.Join(fields, " ")
}

func quickViewHead(s string, maxLines int) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func decisionFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ============================================================================
// CP-84 / Task-429 — multiplexed realtime run-updates plane
// (GET /client/events/stream)
//
// One SSE connection carries LEVEL-TRIGGERED RunRealtimeProjection frames for
// every user-visible lane — it is not a durable global event log (T-1) and
// never mirrors raw ProviderEvent/transcript traffic (T-2). The first burst
// is a chunked authoritative snapshot of every non-terminal lane; live frames
// then report the latest projection per run.
//
// Backpressure is per-subscriber dirty-set coalescing: a dirty mark retains
// the runID until the subscriber drains the LATEST projection — nothing is
// queued per event, so activity storms cannot starve attention (T-5). A
// subscriber whose dirty set exceeds runUpdateDirtyCap is closed retryable:
// the client reconnects for a fresh snapshot. At-least-once, never silent
// loss (AGENTS §2).
// ============================================================================

// RunRealtimeFrameKind enumerates mux frame kinds (Task-429 T-1).
type RunRealtimeFrameKind string

const (
	// RunRealtimeSnapshot is a bounded chunk of the authoritative lane set;
	// the client stages chunks and reconciles atomically on Complete (T-4).
	RunRealtimeSnapshot RunRealtimeFrameKind = "snapshot"
	// RunRealtimeUpsert carries the latest projection for one run.
	RunRealtimeUpsert RunRealtimeFrameKind = "upsert"
	// RunRealtimeRemove tombstones a terminal/deleted/no-longer-visible run.
	RunRealtimeRemove RunRealtimeFrameKind = "remove"
	// RunRealtimeResync closes an overflowed subscriber retryable — the
	// client must reconnect for a fresh authoritative snapshot (T-5).
	RunRealtimeResync RunRealtimeFrameKind = "resync"
)

// RunRealtimeProjection is the bounded state of ONE user-visible lane. It
// carries no transcript delta, tool payload, or raw ProviderEvent — only what
// the attention surface needs (T-2).
type RunRealtimeProjection struct {
	RunID     string `json:"runId"`
	ProjectID string `json:"projectId"`
	ChatID    string `json:"chatId,omitempty"`
	// Revision is the mux-emission sequence stamped at send time — a client
	// dedupe token only, NEVER a reconnect cursor (T-6: reconnect-by-snapshot
	// is the closed decision). Stamped from s.runUpdateSeq at every emit so
	// decision-only changes (which leave rs.seq untouched) still advance it.
	Revision    int64             `json:"revision"`
	ProviderKey string            `json:"providerKey,omitempty"`
	Status      RunStatus         `json:"status"`
	UpdatedAt   string            `json:"updatedAt"`
	LastSummary string            `json:"lastSummary,omitempty"`
	Decisions   []DecisionPayload `json:"decisions,omitempty"`
}

// RunRealtimeFrame is one SSE data payload on the mux stream.
type RunRealtimeFrame struct {
	Kind       RunRealtimeFrameKind    `json:"kind"`
	SnapshotID string                  `json:"snapshotId,omitempty"` // connection-local staging id, not a replay cursor
	Complete   bool                    `json:"complete,omitempty"`
	RunID      string                  `json:"runId,omitempty"`     // remove / upsert key
	Run        *RunRealtimeProjection  `json:"run,omitempty"`       // upsert
	Runs       []RunRealtimeProjection `json:"runs,omitempty"`      // snapshot chunk
	Retryable  bool                    `json:"retryable,omitempty"` // resync — reconnect is safe
}

// runUpdateSub is one mux subscriber. Its dirty SET coalesces repeated marks
// into one latest-projection drain (T-5) — memory is bounded by run count,
// not event count.
type runUpdateSub struct {
	id        int64
	dirty     map[string]bool
	wake      chan struct{} // cap-1; the SSE handler drains on signal
	closed    bool          // dirty cap exceeded — handler must close retryable
	retryable bool
	// removed tracks runIds whose remove frame was already sent — a terminal
	// lane re-marked dirty (settle emits several trailing events) must not
	// re-emit remove on every drain.
	removed map[string]bool
	// fps is the per-subscriber last-sent projection fingerprint per run. The
	// suppression check MUST live on the subscriber, not the run: a shared
	// fingerprint lets the first drainer's update mark the run "sent" for
	// every other subscriber, silently starving them.
	fps map[string]string
}

// runUpdateDirtyCap bounds per-subscriber dirty memory; past it the
// subscriber is closed retryable rather than growing RAM unboundedly (T-5).
const runUpdateDirtyCap = 4096

// runUpdateSnapshotChunk bounds each snapshot frame's run count (T-4).
const runUpdateSnapshotChunk = 32

// isRealtimeVisibleRun limits the mux to top-level/user-visible lanes (T-3):
// delegated child runs (parentRunID != "") are excluded — their progress
// continues through the parent's graph/event projection.
func isRealtimeVisibleRun(rs *interactiveRun) bool {
	return rs != nil && rs.parentRunID == ""
}

// runStatusTerminal is the lane-removal predicate for the mux. It is sourced
// from the recovered run STATUS only — never from an event type (T-9/V10:
// EventTurnCompleted is not terminal while a post-turn gate is armed).
func runStatusTerminal(st RunStatus) bool {
	return st == RunStatusCompleted || st == RunStatusFailed || st == RunStatusCancelled
}

// isLaneRelevant keeps a terminal run in the lane set while it still carries
// an actionable decision — a merge_pending worktree (CP-83) is completed yet
// still waits on a human merge choice. Losing that lane would strand the
// decision until the next poll.
func isLaneRelevant(p RunRealtimeProjection) bool {
	if !runStatusTerminal(p.Status) {
		return true
	}
	for _, d := range p.Decisions {
		if d.Actionable {
			return true
		}
	}
	return false
}

// projectRealtimeRunLocked builds one lane projection. Caller holds s.mu;
// pure memory read — no marshal, I/O, or network (T-4). Dispatch attention is
// merged separately via dispatchAttentionByRun (durable store read, outside
// s.mu).
func (s *InteractiveService) projectRealtimeRunLocked(rs *interactiveRun) RunRealtimeProjection {
	return RunRealtimeProjection{
		RunID:       rs.id,
		ProjectID:   rs.projectID,
		ChatID:      rs.chatID,
		Revision:    rs.seq,
		Status:      rs.status,
		UpdatedAt:   rs.updatedAt,
		LastSummary: boundDecisionText(rs.lastMessage, decisionFieldMaxLen),
		ProviderKey: string(rs.providerKey),
		Decisions:   s.decisionPayloadsForRunLocked(rs),
	}
}

// realtimeFingerprint identifies a MEANINGFUL lane change: status, last
// completed-activity summary, leg identity, and the actionable decision set.
// Revision/UpdatedAt are deliberately excluded — both advance on transcript
// noise, and including them would re-emit frames for every delta (T-2).
func realtimeFingerprint(p RunRealtimeProjection) string {
	var b strings.Builder
	b.WriteString(string(p.Status))
	b.WriteByte('|')
	b.WriteString(p.LastSummary)
	b.WriteByte('|')
	b.WriteString(p.ChatID)
	b.WriteByte('|')
	b.WriteString(decisionFingerprint(p.Decisions))
	return b.String()
}

// dispatchAttentionByRun reads the durable dispatch store OUTSIDE s.mu — the
// store is the authority; rs.dispatch is a mirror written by turn goroutines
// without s.mu and must not be iterated here (CP-84 race review).
func dispatchAttentionByRun(store DispatchStore) map[string][]AttentionItem {
	byRun := map[string][]AttentionItem{}
	if store == nil {
		return byRun
	}
	if list, err := store.ListAttention(context.Background()); err == nil {
		for _, item := range list {
			byRun[item.RunID] = append(byRun[item.RunID], item)
		}
	}
	return byRun
}

func sortDecisions(ds []DecisionPayload) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].CreatedAt != ds[j].CreatedAt {
			return ds[i].CreatedAt < ds[j].CreatedAt
		}
		return ds[i].ID < ds[j].ID
	})
}

// subscribeRunUpdates registers a subscriber and returns its id, wake
// channel, and the authoritative snapshot of every user-visible non-terminal
// lane. Registration and the snapshot copy happen inside one critical section
// (T-4) — no event between the two is missed: a mutation lands either in the
// projection or in a dirty mark drained later.
func (s *InteractiveService) subscribeRunUpdates() (int64, <-chan struct{}, []RunRealtimeProjection) {
	s.mu.Lock()
	if s.runUpdateSubs == nil {
		s.runUpdateSubs = map[int64]*runUpdateSub{}
	}
	sub := &runUpdateSub{
		id:      s.runUpdateNextID,
		dirty:   map[string]bool{},
		removed: map[string]bool{},
		fps:     map[string]string{},
		wake:    make(chan struct{}, 1),
	}
	s.runUpdateNextID++
	s.runUpdateSubs[sub.id] = sub
	store := s.dispatchStore

	var runs []*interactiveRun
	var snap []RunRealtimeProjection
	for _, rs := range s.runs {
		if !isRealtimeVisibleRun(rs) {
			continue
		}
		runs = append(runs, rs)
		snap = append(snap, s.projectRealtimeRunLocked(rs))
	}
	s.mu.Unlock()

	byRun := dispatchAttentionByRun(store)

	s.mu.Lock()
	defer s.mu.Unlock()
	kept := snap[:0]
	for i, rs := range runs {
		if s.runs[rs.id] != rs {
			continue // deleted between phases — never broadcast, nothing to remove
		}
		for _, item := range byRun[rs.id] {
			snap[i].Decisions = append(snap[i].Decisions, dispatchDecisionFromAttention(rs, item))
		}
		sortDecisions(snap[i].Decisions)
		if !isLaneRelevant(snap[i]) {
			continue // terminal with nothing actionable leaves the lane set
		}
		s.runUpdateSeq++
		snap[i].Revision = s.runUpdateSeq
		sub.fps[snap[i].RunID] = realtimeFingerprint(snap[i])
		kept = append(kept, snap[i])
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].RunID < kept[j].RunID })
	return sub.id, sub.wake, kept
}

// unsubscribeRunUpdates removes a subscriber. The wake channel needs no close:
// only the owning handler reads it, and a stale wake is harmless.
func (s *InteractiveService) unsubscribeRunUpdates(subID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runUpdateSubs, subID)
}

// runUpdateSubCount reports the live subscriber count (T-7 telemetry).
func (s *InteractiveService) runUpdateSubCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.runUpdateSubs)
}

// markRunRealtimeDirtyLocked coalesces a run into every live subscriber's
// dirty set. Caller holds s.mu. runID need not exist in s.runs (ss-lock
// synthetic runs and just-removed runs both resolve at drain time). With no
// subscribers this is a no-op — emitLocked stays free of bookkeeping when
// nobody listens.
func (s *InteractiveService) markRunRealtimeDirtyLocked(runID string) {
	if runID == "" {
		return
	}
	for _, sub := range s.runUpdateSubs {
		if sub.closed {
			continue
		}
		sub.dirty[runID] = true
		if len(sub.dirty) > runUpdateDirtyCap {
			sub.closed = true
			sub.retryable = true
			sub.dirty = nil
		}
		select {
		case sub.wake <- struct{}{}:
		default:
		}
	}
}

// markRunRealtimeDirty is the locking variant for callers outside s.mu
// (ss-lock gate ops run under the gate mutex, dispatch mutations on bridge
// goroutines).
func (s *InteractiveService) markRunRealtimeDirty(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markRunRealtimeDirtyLocked(runID)
}

// drainRunUpdates swaps out the subscriber's dirty set and returns the latest
// frame per run: upsert for visible non-terminal lanes, remove for
// terminal/deleted ones. Two-phase: durable dispatch attention is read
// outside s.mu, then merged; fingerprints suppress frames whose meaningful
// state did not move (T-2/T-5).
func (s *InteractiveService) drainRunUpdates(subID int64) []RunRealtimeFrame {
	s.mu.Lock()
	sub := s.runUpdateSubs[subID]
	if sub == nil {
		s.mu.Unlock()
		return nil
	}
	if sub.closed {
		delete(s.runUpdateSubs, subID)
		s.mu.Unlock()
		return []RunRealtimeFrame{{Kind: RunRealtimeResync, Retryable: sub.retryable}}
	}
	dirty := sub.dirty
	sub.dirty = map[string]bool{}

	type cand struct {
		rs   *interactiveRun
		proj RunRealtimeProjection
	}
	var cands []cand
	var removes []string
	var gates []*ssLockGate
	for runID := range dirty {
		rs := s.runs[runID]
		if rs == nil {
			if g := s.ssLockGateFor(runID); g != nil {
				gates = append(gates, g)
				continue
			}
			removes = append(removes, runID)
			continue
		}
		if !isRealtimeVisibleRun(rs) {
			continue // delegated child runs never enter the lane set (T-3)
		}
		cands = append(cands, cand{rs: rs, proj: s.projectRealtimeRunLocked(rs)})
	}
	store := s.dispatchStore
	s.mu.Unlock()

	byRun := dispatchAttentionByRun(store)

	s.mu.Lock()
	defer s.mu.Unlock()
	frames := make([]RunRealtimeFrame, 0, len(cands)+len(removes)+len(gates))
	emitRemove := func(id string) {
		if sub.removed[id] {
			return // remove already sent — terminal lanes re-mark dirty on settle
		}
		sub.removed[id] = true
		delete(sub.fps, id) // lane is gone — a later upsert must fire fresh
		frames = append(frames, RunRealtimeFrame{Kind: RunRealtimeRemove, RunID: id})
	}
	for _, c := range cands {
		if s.runs[c.rs.id] != c.rs {
			emitRemove(c.rs.id)
			continue
		}
		proj := c.proj
		for _, item := range byRun[c.rs.id] {
			proj.Decisions = append(proj.Decisions, dispatchDecisionFromAttention(c.rs, item))
		}
		sortDecisions(proj.Decisions)
		if !isLaneRelevant(proj) {
			emitRemove(c.rs.id)
			continue
		}
		fp := realtimeFingerprint(proj)
		if fp == sub.fps[c.rs.id] {
			continue
		}
		sub.fps[c.rs.id] = fp
		delete(sub.removed, c.rs.id) // lane is live again — a later remove must fire
		p := proj
		s.runUpdateSeq++
		p.Revision = s.runUpdateSeq
		frames = append(frames, RunRealtimeFrame{Kind: RunRealtimeUpsert, RunID: c.rs.id, Run: &p})
	}
	for _, g := range gates {
		if f, ok := ssLockSyntheticFrame(g); ok {
			if f.Kind == RunRealtimeRemove && sub.removed[f.RunID] {
				continue
			}
			if f.Kind == RunRealtimeRemove {
				sub.removed[f.RunID] = true
				delete(sub.fps, f.RunID)
			} else {
				delete(sub.removed, f.RunID)
				if f.Run != nil {
					s.runUpdateSeq++
					f.Run.Revision = s.runUpdateSeq
				}
			}
			frames = append(frames, f)
		}
	}
	for _, id := range removes {
		emitRemove(id)
	}
	return frames
}

// ssLockSyntheticFrame emits an upsert for a Task-333 standardize gate whose
// runID has no interactiveRun, or a remove once the gate leaves waiting.
func ssLockSyntheticFrame(g *ssLockGate) (RunRealtimeFrame, bool) {
	g.mu.Lock()
	status := g.status
	base, ssPath, sdPath, quick := g.featureBase, g.draftSSPath, g.draftSDPath, g.draftSSContent
	g.mu.Unlock()
	if status != ssLockWaiting {
		return RunRealtimeFrame{Kind: RunRealtimeRemove, RunID: g.runID}, true
	}
	d := DecisionPayload{
		Version:    DecisionPayloadVersion,
		ID:         "ss_lock:" + g.runID,
		RunID:      g.runID,
		Kind:       DecisionKindSSLock,
		Revision:   "ss:" + status + ":" + ssPath,
		Status:     "pending",
		Actionable: strings.TrimSpace(quick) != "",
		Prompt:     boundDecisionText("Confirm System Specs draft for "+base, decisionPromptMaxLen),
		SSLock: &DecisionSSLockPayload{
			FeatureBase: base, DraftSSPath: ssPath, DraftSDPath: sdPath,
			QuickView: boundDecisionText(quickViewHead(quick, 40), decisionQuickViewMaxLen),
		},
	}
	proj := RunRealtimeProjection{
		RunID:     g.runID,
		Status:    RunStatus(ssLockWaiting),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Decisions: []DecisionPayload{d},
	}
	return RunRealtimeFrame{Kind: RunRealtimeUpsert, RunID: g.runID, Run: &proj}, true
}

// muxHeartbeatInterval keeps proxies/clients from idle-closing the mux.
// var (not const) so tests can shrink it — production value stays 25s.
var muxHeartbeatInterval = 25 * time.Second
