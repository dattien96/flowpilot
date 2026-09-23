# Task-430 — Decision Payload Contract (Per-Kind Context Cho Inbox)

- Document ID: `Task-430`
- Title: `Decision Payload Contract`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `Task-339/CP-62 P-3 (UserDecisionCard), Task-423 (inline actions), SS-23/SD-27 (worktree merge)`
- Replaces: ``
- Tags: `attention-queue, decision-card-ui, runner, contract`

## AI Quick View

### Summary

- Chuẩn hoá mọi pending decision thành **versioned, durable projection**
  trên Task-429 run snapshot: approval, question, gate/r-reg, ss_lock,
  worktree_merge, dispatch_attention; quota được normalize ở desktop từ
  provider-account authority, không giả làm per-run durable state.
- Reuse authoritative records hiện có; không derive từ transient UI/modal
  fields. Payload absent/invalid/stale → client render "Open" only.

### Current Ask

- Runner: `DecisionPayload[]` projection từ durable pending records, stable
  `decisionId + revision`, bounded context; desktop: discriminated union,
  item identity `(runId, decisionId)`, stale-safe ID-scoped actions.

### Key Decisions

- `T-1` Một run có thể có nhiều actionable records; transport là
  `decisions: []`, không pointer đơn. Sort deterministic theo priority +
  createdAt + decisionId; không silently overwrite approval bởi merge/gate.
- `T-2` Mỗi payload bắt buộc có `decisionId`, `revision`, `kind`, `createdAt`.
  Action submit gửi exact decision id + expected revision/idempotency key;
  server stale/mismatch trả 409 rồi client refresh — không act theo
  "current focused decision".
- `T-3` Go dùng validated tagged projection; TypeScript dùng discriminated
  union per kind để impossible states không compile. Question options giữ
  native `QuestionOption`; chỉ structured decision-card options reuse
  `DecisionCardOption` — không ép mọi kind phải có `consequence` giả.
- `T-4` Builder đọc state đã materialize/persist; không đọc doc/file, chạy
  git diff, account API hay network khi giữ `s.mu`. SS quickView/diffStat/
  output tail được capture + persist tại mutation point hoặc load ngoài lock.
- `T-5` Quota candidate ownership ở desktop/provider-account snapshot
  (`findBestCandidate` hiện là client logic). Runner chỉ emit run/provider
  quota signal nếu có; desktop enrich thành `QuotaDecisionPayload` với
  usage snapshot revision. Không tạo dependency runner → desktop policy.
- `T-6` Fail-closed: unknown kind, missing required field, oversized or
  stale payload → omit actionable controls, preserve lightweight attention
  marker để UI render "Open". Never guess/default an action.

### Constraints

- Additive JSON fields; old client ignores `decisions`, new client falls back
  to existing pending fields when talking to old runner.
- Payload limits: prompt/quickView/outputTail bounded by bytes and lines;
  list counts capped; file paths normalized; no raw tool input/output,
  credentials, account home/auth paths or environment values.
- Provider parity: shared kinds provider-neutral; missing structured options
  is explicit `actionable=false` degradation, not empty options interpreted
  as safe action.
- Projection must recover after runner restart/device switch from durable
  records; transient `interactiveRun` fields alone are insufficient evidence.
- `ss_lock.quickView` missing → preview unavailable/Open-only; never read and
  parse the source document inside `decisionPayloadsForRunLocked`.

### Open Questions

- Gate inline context is closed at bounded test list + command + byte-capped
  tail + diffStat; full diff remains Open-run only.
- Before implementation, confirm which existing gate/ss_lock/merge records
  already carry a durable revision. Missing revision requires extending that
  authoritative record, not synthesizing a RAM counter.

### Source Refs

- `CP-84 P-2`, `user_decision_card.go` (UserDecisionCard/DecisionCardOption/
  parse rules), `attentionQueue.ts` (`pending` payload hiện có),
  `findBestCandidate` (store.ts)

## 1. Goal

Mọi điểm dừng cần con người trong run — approval, câu hỏi, gate, SS lock,
merge decision, quota switch — xuất hiện trên attention surface với đủ
context để quyết inline mà không mở run.

## 2. Parent Links

- coding plan: CP-84 (P-2)
- tech design: SD-27 (worktree merge decision)
- system spec: SS-23 (worktree isolation), SS-13 (document contract)
- specific upstream ids: Task-339 (decision card), Task-423; Task-429 consumes this projection

## 3. Trigger

Inbox inline actions hôm nay chỉ có `approvals`/`questions` của **focused
run** (snapshot chỉ feed từ `cacheRunSnapshot`). Muốn inline cho lane nền
cần payload đi cùng event — và cần cover đủ kind, không chỉ 2 loại.

## 4. Exact Change

- `T-1` `decision_payload.go`: base identity/version fields + validated
  per-kind projection; deterministic `decisionPayloadsForRunLocked` returns
  all durable decisions (not first-match only).
- `T-2` Audit authoritative source + mutation point cho từng kind. Nếu
  gate/ss_lock/merge chưa persist context/revision, extend chính record/
  workflow state đó và recovery codec; không copy vào một RAM-only registry.
- `T-3` `RunRealtimeProjection.Decisions []DecisionPayload` (Task-429);
  existing focused snapshot may expose same projection additively, nhưng
  không có hai builders.
- `T-4` Desktop `contract.ts`: discriminated union; attention queue explode
  decisions thành items keyed `(runId, decisionId)`, deterministic ordering;
  snapshot reconciliation removes stale revisions.
- `T-5` Quota adapter ở desktop enrich provider-account snapshot thành
  quota decision; action carries account usage revision/candidate id and
  revalidates before switch.
- `T-6` Add strict bounding/sanitization helper + redaction tests. Validation
  failure returns non-actionable marker/Open-only, không drop toàn bộ run
  khỏi attention.

## 5. Touched Areas

- files: `internal/runner/types.go`, `internal/runner/decision_payload.go`
  (new), `internal/runner/interactive_service.go`, authoritative gate/ss-lock/
  worktree recovery files identified by T-2 audit (conditional),
  `internal/runner/user_decision_card.go` (reuse chứ không đổi),
  `apps/desktop-flowpilot/src/types/contract.ts`, `src/state/attentionQueue.ts`,
  `src/state/store.ts` (quota enrichment/revalidation)
- modules: `internal/runner`, desktop state/types
- routes: none (field additive trên endpoint hiện có)
- tables: none

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/runner/decision_payload.go
type DecisionKind string
// consts: approval, question, gate, ss_lock, worktree_merge, dispatch_attention

type DiffStat struct {
    Files int `json:"files"`
    Plus  int `json:"plus"`
    Minus int `json:"minus"`
}

type DecisionPayload struct {
    DecisionID string               `json:"decisionId"`
    Revision   int64                `json:"revision"`
    Kind       DecisionKind         `json:"kind"`
    CreatedAt  string               `json:"createdAt"`
    Actionable bool                 `json:"actionable"`
    Prompt     string               `json:"prompt,omitempty"` // bounded + redacted
    Options    []DecisionCardOption `json:"options,omitempty"` // structured cards only
    Questions  []QuestionOption     `json:"questionOptions,omitempty"`
    RegressedTests []string         `json:"regressedTests,omitempty"`
    Command        string           `json:"command,omitempty"`
    ExitCode       *int             `json:"exitCode,omitempty"`
    OutputTail     string           `json:"outputTail,omitempty"`
    DiffStat       *DiffStat        `json:"diffStat,omitempty"`
    DocID          string           `json:"docId,omitempty"`
    QuickView      string           `json:"quickView,omitempty"`
    ConflictPaths  []string         `json:"conflictPaths,omitempty"`
    PatchRef       string           `json:"patchRef,omitempty"`
}

func validateAndBoundDecisionPayload(p DecisionPayload) (DecisionPayload, bool)
// false => caller keeps non-actionable attention marker/Open-only.
```

```go
// apps/local-runner/internal/runner/interactive_service.go
func (s *InteractiveService) decisionPayloadsForRunLocked(rs *interactiveRun) []DecisionPayload
// Caller holds s.mu; pure projection only — no I/O/git/network/document parsing.
```

```ts
// apps/desktop-flowpilot/src/types/contract.ts
export interface DecisionBase {
  decisionId: string; revision: number; createdAt: string; actionable: boolean;
}
export interface QuotaDecisionPayload extends DecisionBase {
  kind: "quota";
  providerKey: string;
  currentAccountId: string;
  candidateId: string;
  candidateLabel: string;
  usageRevision: string;
  remaining5hPercent?: number;
  remaining7dPercent?: number;
}
export interface DecisionActionRef {
  runId: string;
  decisionId: string;
  expectedRevision: number;
}
export type DecisionPayload =
  | (DecisionBase & { kind: "approval"; prompt: string })
  | (DecisionBase & { kind: "question"; prompt: string; questionOptions: QuestionOption[] })
  | (DecisionBase & { kind: "gate"; regressedTests: string[]; command?: string; outputTail?: string; diffStat?: DiffStat })
  | (DecisionBase & { kind: "ss_lock"; docId: string; quickView?: string })
  | (DecisionBase & { kind: "worktree_merge"; conflictPaths: string[]; patchRef?: string; diffStat?: DiffStat })
  | (DecisionBase & { kind: "dispatch_attention"; prompt: string })
  | QuotaDecisionPayload; // desktop-owned enrichment from provider-account snapshot
```

```ts
// apps/desktop-flowpilot/src/state/attentionQueue.ts
export interface AttentionItem {
  runId: string;
  decisionId: string; // item identity is (runId, decisionId), not runId alone
  decision?: DecisionPayload;
}
export function reconcileRunDecisions(run: RunRealtimeProjection): void
```

## 7. Test Signatures

- `TestDecisionPayloads_MultiplePendingRecordsAreStableAndOrdered` — không
  mất decision; sort priority/createdAt/id deterministic.
- `TestDecisionPayloads_RecoverSameIDsAndRevisionsAfterRestart` — durable
  projection qua kill/restart; không RAM-generated revision.
- `TestDecisionPayloads_GateCarriesBoundedContext` — test list/tail/diffStat
  đủ context nhưng obey byte/count caps.
- `TestDecisionPayloads_MissingSSQuickViewIsNonActionable` — vẫn có marker,
  `actionable=false`, UI phải Open-only.
- `TestDecisionPayloads_MergeUsesDurableBindingRevision` — conflict paths +
  patch ref + stable revision từ worktree authority.
- `TestDecisionPayloads_QuestionKeepsNativeQuestionOptions` — không ép
  consequence giả/không drop valid question.
- `TestDecisionPayloads_RedactsSecretsAndCapsPayload` — auth path/token/env
  không xuất hiện; oversized fields truncate deterministically.
- `TestDecisionPayloads_ProjectorPerformsNoIOWhileLocked` — builder thuần;
  persisted precomputed context được dùng.
- `test("reconcile decisions keys items by runId and decisionId", ...)` —
  nhiều decision cùng run không overwrite nhau.
- `test("older decision revision cannot restore a resolved item", ...)`.
- `test("quota enrichment uses provider account snapshot and revalidates candidate", ...)`.
- `test("old runner without decisions falls back to pending fields/Open-only", ...)`.

## 8. Acceptance Check

- Live: park một run với ≥2 pending records (hoặc gate + merge pending)
  rồi restart runner → mux snapshot trả lại cùng `decisionId/revision`, đủ
  bounded context, deterministic order.
- Resolve một item bằng exact id/revision rồi replay frame cũ → item không
  sống lại; stale action trả 409 + refresh.
- Client mới với runner cũ (không `decisions`) vẫn dùng legacy pending/Open-only,
  không crash; payload chứa secret-shaped text không leak ra SSE/log.

## 9. Out of Scope

- Inbox UI controls/preview rendering (Task-431), modal routing (Task-434),
  thêm decision kind ngoài danh sách đã freeze; provider adapter behavior.
  Việc extend authoritative durable record/recovery codec để có revision/context
  **nằm trong scope** nếu audit T-2 chứng minh field chưa tồn tại.

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity: shared projection không nhánh provider; capability thiếu → `actionable=false` typed degradation (R2)
- [x] Every actionable item has durable `decisionId + revision`; restart/replay/stale-action tests green
- [x] No I/O/git/network/doc parsing under `s.mu`; payload size/redaction limits tested
- [x] Quota enrichment remains desktop/provider-account-owned; no runner dependency on `findBestCandidate`
- [x] Multiple decisions per run survive deterministic projection and reconciliation
- [x] `feature_key` = `attention-queue` (hoặc `decision-card-ui` nếu reviewer thấy khớp hơn); CA entry written
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus impact run on each authoritative pending-record mutation/recovery seam before code
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: DONE — DecisionPayload v1 (stable durable ids, opaque revision → 409 resubmit token, bounded + redacted) projected from durable records: approval/question shards (new revision/createdAt fields), gate-block events, ss-lock gates, worktree binding, dispatch store (AttentionItem.Revision). Provider-agnostic; missing capability → actionable=false.
- follow-ups: quota enrichment stays desktop-owned per DoD; Supabase column mapping for new revision fields degrades to zero-value (local ndjson is authoritative).
- upstream docs updated: CP-84, CA-931.
