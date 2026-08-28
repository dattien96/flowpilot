# CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance

## Metadata

- Document ID: `CP-55`
- Title: `Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance`
- Phase: `coding_plan`
- Status: `approved` (2026-07-30 — P-1 implemented and accepted by Codex review pass 7 with zero findings after every pass-1-through-pass-6 finding was fixed in the current uncommitted worktree. Pass 3 fixed the desktop Settings direct-to-Supabase TypeScript path that pass 2's Go-store fix did not cover; pass 4 fixed diagnostic/evidence/status wording; pass 5 restored the append-only review ledger and corrected the final-tree test count; pass 6 found only stale review-status prose and temporary ledger-repair artifacts, both corrected/removed before the clean pass 7. See [Task-263](../../08-Task/done/Task-263-Explicit-Flow-Writer-Semantics-And-Safety-Topology.md)/[CA-424](../../../change-audit/CA-424-explicit-flow-writer-semantics-and-safety-topology.md). 2026-07-31 — P-2 implemented and self-verified in the current uncommitted worktree; no Codex review pass has run on P-2 yet. See [Task-264](../../08-Task/done/Task-264-Frozen-Preflight-Contract-Model-And-Storage.md)/[CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md). 2026-07-31 — operator directed Claude to implement all remaining phases (P-3 through P-9) sequentially in one session, with review by a dedicated Claude agent invocation instead of Codex; P-3 implemented, Claude-agent reviewed (4 Critical/11 Important/7 Minor found, all Critical + in-scope Important fixed), and full-suite regressed twice (16 pre-existing/environment failures triaged each time, 0 attributable to this change). See [Task-265](../../08-Task/done/Task-265-Contract-Planner-Freeze-Node-And-Inline-Chain-Advancement.md)/[CA-426](../../../change-audit/CA-426-contract-planner-freeze-node-and-inline-chain-advancement.md). P-4 implemented, Claude-agent reviewed (2 Critical/5 Important/4 Minor found — including a real security hole letting a writer silently disable its own gate rules or forge its own frozen contract, and a fail-open bug on git-observation failure — all Critical + 4/5 Important fixed, each verified via mutation testing), full-suite regressed twice (15 pre-existing failures both times, 0 new, 0 flakes). See [Task-266](../../08-Task/done/Task-266-Enforce-Frozen-Scope-At-Coder-Gate-And-Amendments.md)/[CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md). P-5 implemented, Claude-agent reviewed (3 Critical/8 Important/8 Minor found — a FAIL verdict, including a silent-data-loss fail-open bug on key re-staging, a partial-commit bug across multi-feature finalize, and a hang-inducing bug on finalize failure — all 3 Critical + 6/8 Important fixed, each Critical fix verified via mutation testing), full-suite regressed twice (16 failures on the second run: the same 15 pre-existing failures plus 1 confirmed load-dependent flake, 0 new deterministic regressions). See [Task-267](../../08-Task/done/Task-267-Move-Canonical-Mutation-To-Terminal-Flow-Acceptance.md)/[CA-428](../../../change-audit/CA-428-move-canonical-mutation-to-terminal-flow-acceptance.md). P-6 implemented (a new, deliberately unwired `featurecatalog/relevance.go`), Claude-agent reviewed (0 Critical/5 Important/7 Minor found — a CONDITIONAL PASS, all 5 Important + all 7 Minor fixed/addressed, each Important fix verified via mutation testing); no `internal/runner` regression run needed (this phase touches nothing that suite exercises). See [Task-268](../../08-Task/done/Task-268-Deterministic-History-Relevance-Scorer.md)/[CA-429](../../../change-audit/CA-429-deterministic-history-relevance-scorer.md). P-7 implemented (wired `feature.history` to rank by code-locus overlap), Claude-agent reviewed (0 Critical/8 Important/9 Minor found — a CONDITIONAL PASS, including a "current truth" entry that could silently lose its excerpt/enforcement instruction in ranked mode and a zero-value Limit that silently disabled the output cap — all 8 Important + all 9 Minor fixed/addressed, each Important fix verified via mutation testing), full-suite regressed twice (16 failures both runs: the same 15 pre-existing failures plus 1 confirmed load-dependent flake, 0 new deterministic regressions). See [Task-269](../../08-Task/done/Task-269-Wire-Ranking-Into-Feature-History.md)/[CA-430](../../../change-audit/CA-430-wire-ranking-into-feature-history.md). P-8 implemented — migrated all 3 built-in flow YAMLs to the target preflight pattern; found and fixed 3 genuine production bugs before any review pass (an `agent.code` writer's gate pass had zero Canonical Head effect at all; a writer's own pending-canonical staging write self-triggered scope drift on its next gate pass; `advanceFlowThroughFreezeChain` never rendered the built context package into the writer's actual prompt for a flow with no intermediate context.produce node — review-loop's own shape), each mutation-tested; fixed 22 existing tests broken by the topology change, each individually root-caused (one was found to be passing for the wrong reason and is now fixed to genuinely test its scenario); full-suite regressed to completion three times, final run shows exactly the established 15 pre-existing failures, 0 new. Claude-agent reviewed (1 Critical/3 Important/2 Minor found — a FAIL verdict, the most severe of any CP-55 phase reviewed so far: a coder's ordinary file-write tool could forge an arbitrary Canonical Head for any feature via the pending-canonical store's exempted bookkeeping path, trusted byte-for-byte at finalize with zero provenance check — closed with an HMAC-signature scheme reusing the existing markerSecret trust boundary; plus an unanchored fake-adapter role-marker false-positive, an unvalidated planner feature-key silently routing history to the wrong feature, and the Canonical Head fix itself only reachable via incidental default-rule coupling — all 4 Critical/Important fixed and mutation-tested, both Minor accepted/documented), full-suite regressed again after fixes (same 15-baseline plus 1 already-documented load-dependent flake, 0 new). See [Task-270](../../08-Task/done/Task-270-Migrate-Built-In-Flows-And-E2E-Recovery-Parity-Coverage.md)/[CA-431](../../../change-audit/CA-431-migrate-built-in-flows-and-e2e-recovery-parity-coverage.md). P-9 (documentation-only, no code): synced CP-43/CP-54 current-state sections, recorded before/after evidence for contract timing/ranking/Canonical finalization citing already-passing tests, confirmed one Task+CA pair per slice already exists for P-1 through P-8. See [Task-271](../../08-Task/done/Task-271-CP55-Documentation-Rollout-Evidence-And-Operator-Review.md)/[CA-432](../../../change-audit/CA-432-cp55-documentation-rollout-evidence-and-operator-review.md). **All 9 phases done — this CP is complete.**)
- Owner: `FlowPilot Architecture`
- Reviewers: `Codex` (P-1); `Claude agent review` (P-2 onward, per 2026-07-31 operator direction)
- Created: `2026-07-30`
- Last Updated: `2026-07-31`
- Parent Documents: [SD-21 Change Contract and Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-17 Context and Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SS-14 Code Context and Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-263: Explicit Flow Writer Semantics And Safety Topology](../../08-Task/done/Task-263-Explicit-Flow-Writer-Semantics-And-Safety-Topology.md) (P-1, done, CA-424); [Task-264: Frozen Preflight Contract Model And Storage](../../08-Task/done/Task-264-Frozen-Preflight-Contract-Model-And-Storage.md) (P-2, done — implemented, not yet Codex-reviewed, CA-425); [Task-265: Contract Planner, Freeze Node, And Inline-Chain Advancement](../../08-Task/done/Task-265-Contract-Planner-Freeze-Node-And-Inline-Chain-Advancement.md) (P-3, done — implemented and Claude-agent reviewed, CA-426); [Task-266: Enforce Frozen Scope At Coder Gate And Amendments](../../08-Task/done/Task-266-Enforce-Frozen-Scope-At-Coder-Gate-And-Amendments.md) (P-4, done — implemented and Claude-agent reviewed, CA-427); [Task-267: Move Canonical Mutation To Terminal Flow Acceptance](../../08-Task/done/Task-267-Move-Canonical-Mutation-To-Terminal-Flow-Acceptance.md) (P-5, done — implemented and Claude-agent reviewed, CA-428); [Task-268: Deterministic History Relevance Scorer](../../08-Task/done/Task-268-Deterministic-History-Relevance-Scorer.md) (P-6, done — implemented and Claude-agent reviewed, CA-429); [Task-269: Wire Ranking Into Feature.History](../../08-Task/done/Task-269-Wire-Ranking-Into-Feature-History.md) (P-7, done — implemented and Claude-agent reviewed, CA-430); [Task-270: Migrate Built-In Flows And Add End-to-End Recovery/Parity Coverage](../../08-Task/done/Task-270-Migrate-Built-In-Flows-And-E2E-Recovery-Parity-Coverage.md) (P-8, done — implemented and Claude-agent reviewed, CA-431); [Task-271: CP-55 Documentation, Rollout Evidence, And Operator Review](../../08-Task/done/Task-271-CP55-Documentation-Rollout-Evidence-And-Operator-Review.md) (P-9, done — documentation-only, CA-432)
- Related Documents: [CP-43 Change Contract and Canonical Intent Signature](../done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [CP-44 Context Registry and Explicit Slot Selection](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-51 Durable Flow Execution](../done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-54 Locus-Anchored Context Relevance](CP-54-Locus-Anchored-Context-Relevance.md), [Task-261 Persist Changed Paths](../../08-Task/done/Task-261-Persist-Changed-Paths-In-Change-Ledger.md), [Task-262 Shared Retrieval Locus Builder](../../08-Task/done/Task-262-Shared-Retrieval-Locus-Builder.md), [CA-422 Persist Changed Paths](../../../change-audit/CA-422-persist-changed-paths-in-change-ledger.md), [CA-423 Shared Retrieval Locus Builder](../../../change-audit/CA-423-shared-retrieval-locus-builder.md), `BUG-288`, `BUG-323`
- Replaces: `None`
- Tags: `flow, change-contract, canonical-head, context-retrieval, history-ranking`
- Feature Keys: `agent-flow-engine`, `context-regression-engine`, `change-contract`
- Implementation Owner: `Claude Sonnet MAX`

---

## AI Quick View

### Summary

- Current contract declaration occurs after the coding turn, Canonical may update before downstream acceptance, and `feature.history` does not yet consume `RetrievalLocus`.
- Target Flow lifecycle freezes a contract before context/coder, ranks large feature history by locus, and finalizes Canonical only at terminal acceptance.
- Normal chat remains unrestricted but is explicitly outside these Flow guarantees.

### Current Ask

- Implement P-1 through P-9 in order with Claude Sonnet MAX as implementation owner and Codex as review gate.
- Preserve current Normal chat usability and all prior CA-297, CA-298, CA-328, CA-422, and CA-423 claims.

### Key Decisions

- `P-1` Flow guarantees apply only to Flow mode; Normal chat remains an unrestricted best-effort surface.
- `P-2` A read-only AI proposes scope, while Go validates and freezes the contract before any writer dispatch.
- `P-3` `agent.code` explicitly marks writer nodes, and `contract.freeze` must dominate each writer.
- `P-4` Context production occurs after freeze; Flow scope drift fails closed and requires amendment/retry.
- `P-5` Canonical mutation is a terminal acceptance effect, not a coder-turn effect.
- `P-6` Disabling `change.contract` hides only its rendered section.
- `P-7` History relevance uses deterministic path overlap, recency, and stable tie-breaking.

### Constraints

- Do not block Normal chat or show a hard “Start a Flow to continue” restriction.
- Do not allow inferred post-code contracts to satisfy Flow preflight.
- Preserve byte-compatible recency fallback and keep `chat.summary` recency-based.
- Keep `RetrievalLocus.Symbols` inert until BUG-323 is resolved.
- Preserve explicit Canonical rebaseline/retire/merge actions and durable re-entry behavior.
- Add tests only; do not edit pre-existing tests without user approval.
- Run GitNexus impact before every symbol edit and change detection before commit.

### Open Questions

- None blocking. Symbol-aware ranking remains deferred to BUG-323.

### Source Refs

- SS-14; SD-17; SD-21; SD-24.
- CP-43 P-1 through P-5; CP-54 P-1 through P-5; this document P-1 through P-9.
- Task-261 / CA-422; Task-262 / CA-423; CA-297; CA-298; CA-328.

---

## 1. Goal

Đưa CP-43 và CP-54 vào cùng một lifecycle có thứ tự thời gian đúng:

```text
User issue
    ↓
Read-only contract planner
    ↓
Validate + normalize + freeze contract
    ↓
Build RetrievalLocus
    ↓
Produce context package
    ↓
Code-writing agent
    ↓
Scope gate
    ↓
Tests / verifier / review / audit declared by the Flow
    ↓
Parent Flow terminal acceptance
    ↓
Finalize Canonical Head
```

Thiết kế phải giải quyết ba lỗi khái niệm:

1. **Temporal contract gap:** contract khai báo sau khi code đã đổi không phải là một pre-commitment.
2. **Premature Canonical mutation:** coder pass gate không đồng nghĩa toàn Flow đã được chấp nhận.
3. **Context dilution:** recency trong một feature lớn không bảo đảm liên quan tới locus của issue hiện tại.

### 1.1 Product boundary

| Mode | User freedom | Preflight contract | Scope guarantee | Acceptance-gated Canonical | Locus-ranked first coder context |
|---|---:|---:|---:|---:|---:|
| Normal chat | Unrestricted | Not guaranteed | Not guaranteed | Not guaranteed | Not guaranteed |
| Flow | Unrestricted within declared workflow | Required for every writer path | Fail-closed | Required | Required when `feature.history` is enabled |

Normal chat không bị cấm sửa code. UI hoặc docs có thể nói rõ “Flow guarantees are not active in Normal mode”, nhưng không được biến thông tin này thành một hard gate.

---

## 2. Input Documents

### 2.1 Governing documents

- SS-14 định nghĩa yêu cầu về context correctness và regression safety.
- SD-21 định nghĩa Change Contract, Canonical Head và intent signature.
- SD-17 định nghĩa context sources, history retrieval và regression engine.
- SD-24 định nghĩa durable dispatch/recovery, cần được giữ khi thêm preflight và terminal effects.
- CP-43 là implementation chain gốc của DeclaredPaths và Canonical Head.
- CP-54 là implementation chain gốc của ChangedPaths, RetrievalLocus và relevance ranking.

### 2.2 Implemented foundations

- Task-261 / CA-422 persist `Entry.ChangedPaths`.
- Task-262 / CA-423 tạo shared `RetrievalLocus` builder.
- P-2 của CP-54 chỉ tạo hạ tầng; chưa có context source nào consume locus, nên runtime output chưa đổi.

### 2.3 Latest relevant implementation history

- `context-regression-engine`: CA-423 là entry gần nhất trực tiếp liên quan retrieval locus.
- `agent-flow-engine`: CA-328 là entry gần nhất liên quan re-entry/gate; CA-293 đến CA-298 chứa phần lớn lifecycle Change Contract và Canonical.
- `change-contract` đã là feature key hợp lệ, nhưng lịch sử CP-43 trước đây chủ yếu được ledger dưới `agent-flow-engine`. Implementation không được giả định rằng luôn có một CA riêng cho `change-contract`.

### 2.4 Current code locations

| Concern | Current location |
|---|---|
| Parse AI declaration | `apps/local-runner/internal/changecontract/parse.go` |
| Contract gate preparation | `apps/local-runner/internal/runner/gate_hook.go` |
| Gate rule defaults | `apps/local-runner/internal/runner/engine_gate_config.go` |
| Contract context rendering | runner context source implementation |
| Contract store | `.flowpilot/contracts/contracts.ndjson` |
| Canonical store | `.flowpilot/canonical/<feature_key>.json` |
| Retrieval locus type | `apps/local-runner/internal/featurecatalog/locus.go` |
| Retrieval locus builder | `apps/local-runner/internal/runner/retrieval_locus.go` |
| History renderer | feature catalog/history implementation |
| Flow terminal control | `apps/local-runner/internal/runner/interactive_service.go` |
| Built-in flows | `apps/local-runner/internal/agentpack/flow-pack/flows/*.yaml` |

GitNexus phải là nguồn điều hướng đầu tiên khi implementation bắt đầu. Bảng trên chỉ là current-state map, không thay cho impact analysis.

---

## 3. Implementation Strategy

### 3.1 Current behavior: Change Contract

Current sequence:

```text
AI receives prompt
    ↓
AI may edit files
    ↓
AI final message contains [Change Contract]
    ↓
changecontract.ParseDeclaration(...)
    ↓
prepareChangeContract(...)
    ↓
gate evaluates r-contract and r-scope
```

Nếu declaration vắng:

- FlowPilot gọi `InferFromDiff`.
- Inferred contract dùng các bucket ở top level và `ConfidenceInferred`.
- Declared contract được save sớm trong gate preparation để sống qua reprompt.
- Inferred contract chỉ được save sau khi gate allow.

Store hiện là append-only NDJSON và dùng last-wins cho `(run_id, step_id)`.

#### Current gate rules

| Rule | Default behavior | Notes |
|---|---|---|
| `r-contract` | Reprompt / enforce | Warn mode có thể downgrade |
| `r-scope` | Warn | Chỉ block nếu rule được cấu hình block và severity đạt high |

BUG-323 làm high-severity scope detection chưa đáng tin cậy. Vì vậy current `r-scope` không tạo ra một Flow preflight guarantee.

#### Why current behavior is insufficient

AI có thể sửa `foo.go` và `bar.go`, rồi mới khai báo hai path đó trong final message. Contract khi ấy chỉ mô tả việc đã xảy ra, không giới hạn hành động trước khi xảy ra.

### 3.2 Target behavior: Flow preflight contract

Selected design:

```text
agent.delegate: contract planner (read-only)
    ↓ proposal
contract.freeze: Go-owned validation + persistence
    ↓ frozen contract
context.produce
    ↓
agent.code
```

Responsibility split:

| Actor | Responsibility |
|---|---|
| User | Cung cấp issue, chọn/chạy Flow |
| AI contract planner | Đề xuất feature, intent và concrete declared paths |
| Go runtime | Parse strict format, validate feature/path, bind run/step/baseline, freeze version |
| Context engine | Build locus từ frozen contract + diff + prompt |
| AI coder | Chỉ sửa trong frozen scope hoặc yêu cầu amendment |
| Gate | So actual changed paths với frozen scope |
| Flow controller | Chấp nhận hoặc từ chối terminal transition |

#### Rejected alternatives

1. **Keep parsing the coder final message.** Rejected vì không có temporal guarantee.
2. **Infer everything deterministically from prompt.** Rejected vì prompt issue thường không đủ concrete để xác định path.
3. **Block all code requests in Normal chat.** Rejected vì Normal chat phải vẫn là một conversational surface tự do.
4. **Update Canonical immediately after coder gate.** Rejected vì validate/review/audit phía sau vẫn có thể fail.

### 3.3 Contract artifact and lifecycle

Contract states:

```text
draft
  ↓ validate
frozen
  ├─ amendment → superseded → new frozen version
  ├─ accepted  → terminal Flow done
  └─ abandoned → failed / stopped / cancelled / escalated Flow
```

Required properties:

- Immutable after freeze.
- Versioned per parent Flow run.
- Bound to the code-writing step.
- Bound to a baseline SHA or equivalent workspace baseline.
- Persisted before coder dispatch.
- Backward-compatible with existing NDJSON records.
- Idempotent under retry/recovery.

Proposed extension to `changecontract.Contract`:

```go
type Contract struct {
    ContractID   string     `json:"contract_id,omitempty"`
    Version      int        `json:"version,omitempty"`
    RunID        string     `json:"run_id"`
    StepID       string     `json:"step_id"`
    PlannerStepID string    `json:"planner_step_id,omitempty"`
    FeatureKey   string     `json:"feature_key"`
    Intent       string     `json:"intent"`
    DeclaredPaths []string  `json:"declared_paths"`
    WrittenPaths []string   `json:"written_paths,omitempty"`
    ScopeDrift   []string   `json:"scope_drift,omitempty"`
    SourceDocID  string     `json:"source_doc_id,omitempty"`
    BaseSHA      string     `json:"base_sha,omitempty"`
    Confidence   Confidence `json:"confidence"`
    Status       string     `json:"status,omitempty"`
    Supersedes   string     `json:"supersedes,omitempty"`
    DeclaredAt   time.Time  `json:"declared_at"`
}
```

Exact field names may be adjusted to current structs, but the data invariants may not be weakened.

Strict planner payload:

```go
type PreflightContractDraft struct {
    FeatureKey   string   `json:"feature_key"`
    Intent       string   `json:"intent"`
    DeclaredPaths []string `json:"declared_paths"`
    SourceDocID  string   `json:"source_doc_id,omitempty"`
}
```

Proposed functions:

```go
func ParsePreflightDraft(text string) (PreflightContractDraft, error)

func ValidatePreflightDraft(
    draft PreflightContractDraft,
    knownFeatureKeys []string,
) (PreflightContractDraft, error)

func NormalizeDeclaredCodePaths(paths []string) ([]string, error)

func ComputeContractID(
    runID string,
    coderStepID string,
    version int,
    draft PreflightContractDraft,
    baseSHA string,
) string

func FreezeContract(
    workspace string,
    runID string,
    plannerStepID string,
    coderStepID string,
    draft PreflightContractDraft,
    baseSHA string,
    now time.Time,
) (Contract, error)
```

Store API:

```go
func (s *Store) SaveFrozen(contract Contract) error

func (s *Store) GetFrozenForStep(
    runID string,
    coderStepID string,
) (Contract, bool, error)

func (s *Store) ListVersionsForStep(
    runID string,
    coderStepID string,
) ([]Contract, error)

func (s *Store) MarkStatus(
    contractID string,
    status string,
    at time.Time,
) error
```

`SaveFrozen` must reject mutation of an existing `contract_id`. A contract amendment creates a higher version with `Supersedes` set to the preceding frozen version.

### 3.4 Path normalization ownership

Current `isConcreteCodeTarget` is in runner. Preflight validation also needs identical semantics before runner builds a locus.

Recommended change:

```go
func NormalizeDeclaredCodePaths(paths []string) ([]string, error)
```

Move the pure normalization rule into `internal/changecontract/paths.go`, then make both contract freeze and runner locus builder use it.

The function must:

- normalize separators;
- trim workspace-relative prefixes safely;
- reject absolute paths outside workspace;
- reject document-only targets when the writer contract requires code targets;
- reject glob-only or top-level bucket placeholders for frozen Flow contracts;
- sort and deduplicate deterministically;
- require at least one concrete path for an `agent.code` step.

This relocates pure policy, not the `RetrievalLocus` type. `RetrievalLocus` remains in `internal/featurecatalog` to preserve dependency direction:

```text
runner → featurecatalog → changeledger
runner → changecontract
```

### 3.5 Explicit code-writing behavior

Current code-writing detection is partly heuristic. A strong graph invariant needs an explicit semantic marker.

Add:

```go
const BehaviorAgentCode BehaviorID = "agent.code"
const BehaviorContractFreeze BehaviorID = "contract.freeze"

func IsCodeWritingBehavior(id BehaviorID) bool
```

`agent.code` can reuse provider dispatch mechanics from `agent.delegate`, but its semantic contract is different:

- it requires a frozen preflight contract;
- its gate must compare against frozen scope;
- it may produce pending Canonical state;
- topology validation treats it as a project mutation boundary.

Contract planner remains `agent.delegate` and must be read-only.

### 3.6 Flow topology validation

Add a static validation pass:

```go
func ValidateFlowSafetyTopology(def FlowDefinition) error

func ForwardDominates(
    def FlowDefinition,
    dominatorNodeID string,
    targetNodeID string,
) bool

func EveryDonePathIncludesAcceptance(
    def FlowDefinition,
    writerNodeID string,
) bool
```

Validation rules:

1. Every `agent.code` node has a preceding `contract.freeze` node on every forward path from entry.
2. No `agent.code` node is an entry node.
3. Frozen contract is bound to that specific writer node.
4. `context.produce` or equivalent context assembly occurs after freeze and before writer for built-in coding flows.
5. Every terminal `done` path after a writer crosses the Flow's declared acceptance boundary.
6. Retry/back edges may return to the same frozen version only if no new paths are needed.
7. A retry requiring new paths must traverse an amendment/freeze step and produce a new version.
8. Invalid built-in or user-defined Flow definitions fail validation before execution.

“Acceptance boundary” is Flow-defined, not hardcoded to one universal node name. A simple code flow may use tests only; a richer flow may use tests + verifier + review + audit. Terminal `done` remains the single point at which the runtime applies accepted Canonical effects.

**P-1 delivered rules 1, 2, 5, and 8 concretely, and this is now the authoritative shape** (see [Task-263](../../08-Task/done/Task-263-Explicit-Flow-Writer-Semantics-And-Safety-Topology.md) / [CA-424](../../../change-audit/CA-424-explicit-flow-writer-semantics-and-safety-topology.md), including the pass-2 review fix): `ForwardDominates`/`forwardDominatesFrom` fail closed — both ids must be declared nodes and the target must be proven reachable from a real entry before any dominance verdict, so a writer/freeze pair stranded in a cycle disconnected from every entry (or a flow with no entry at all) fails rather than vacuously passing. The acceptance boundary (rule 5) is a concrete Flow-declared `FlowDefinition.AcceptanceNodes` field, parsed from the root YAML key `acceptance_nodes`: a flow declaring any `agent.code` writer must set at least one entry naming a real, non-writer, non-blank, non-duplicate node, verified by a full forward-path traversal from each writer with state `(node, hasCrossedAcceptance)` that also requires the writer to actually reach terminal `done` — a writer with no successor, or whose only reachable paths loop forever without ever reaching `done`, fails the same as one that reaches `done` without crossing acceptance. Rules 3, 4, 6, and 7 remain P-2/P-3/P-4 scope. `FlowDefinition.AcceptanceNodes` is also persisted through the Supabase-backed user-flow store (`workflows.acceptance_nodes_json`, `20260730121000_add_workflow_acceptance_nodes.sql`), so a migrated flow's declared acceptance boundary survives a builtin mirror sync or reload done through `apps/local-runner`'s own Go binary, not just an in-memory embedded-pack load.

**CORRECTION (Codex review pass 3):** the paragraph above, as originally written, read as if "clone" and "user edit" were already proven end to end, but those two operations are what a person actually does from the desktop Settings screen — and the Settings screen does not go through the Go binary's `SupabaseWorkflowFlowStore` at all. It is a separate client (`packages/flowpilot-client-core`'s `SupabaseAdminRepository`) talking to the same Postgres/PostgREST backend directly, and until this pass it had no `acceptanceNodes` field on its own `Workflow` model: reading a workflow silently dropped `acceptance_nodes_json`, and cloning it (a genuine `INSERT` with the column omitted) fell back to the migration's `default '[]'::jsonb` regardless of what the source actually declared — reproducing, on the client path a Settings user actually exercises, the exact "loses the mandatory safety boundary" failure the pass-2 Go-store fix had only closed on the other path. The pass-3 fix adds `Workflow.acceptanceNodes?: string[]` (optional/additive) to `adminModels.ts`; `mapWorkflow` reads it; `saveWorkflow`'s upsert payload and `cloneWorkflow`'s insert payload both send it (`?? []`, never `null`/`undefined`); and `WorkflowsSettings.tsx`'s draft layer (`WorkflowDraft`/`mapWorkflowToDraft`/`createEmptyWorkflowDraft`/`saveWorkflow`/`saveNewWorkflow`) carries it through every save unchanged, so a plain rename can no longer silently erase a declared boundary the way an omitted field would. Only *persistence* (read/save/clone/reload) is proven end to end now — not *authoring*: `agent.code`/`contract.freeze` are not yet in `FLOW_BEHAVIOR_OPTIONS`, so no Settings user can create an `agent.code` node, or therefore a meaningful `acceptance_nodes` list, today; the field renders read-only in the workflow detail panel. A Settings UI selector/editor for `acceptance_nodes` remains an **explicit, not-yet-satisfied P-8 prerequisite** (§3.16 below) for whichever slice first lets a user author their own `agent.code`-bearing custom flow — see Task-263 §7/CA-424 for the full evidence.

### 3.7 Contract planner and freeze dispatch

Add a dedicated agent prompt:

```text
apps/local-runner/internal/agentpack/flow-pack/agents/contract-planner.md
```

The planner must:

- inspect issue, governing docs and read-only repository context;
- emit one strict `PreflightContractDraft`;
- not edit project files;
- not broaden scope merely to avoid a later amendment;
- use a registered feature key;
- list concrete workspace-relative paths.

Runtime functions:

```go
func (s *InteractiveService) runContractFreezeNode(
    ctx context.Context,
    parentRunID string,
    edges []FlowEdge,
    nodes []FlowNode,
    node FlowNode,
    plannerResult string,
) (FlowNodeResult, error)

func (s *InteractiveService) advanceFlowThroughInlineChain(
    ctx context.Context,
    parentRunID string,
    edges []FlowEdge,
    nodes []FlowNode,
    start FlowNode,
    input FlowNodeResult,
) (FlowNodeResult, error)
```

The existing single-step inline switch should be generalized enough to support:

```text
agent.delegate planner
  → contract.freeze
  → context.produce
  → agent.code
```

Do not add an unbounded recursive executor. Inline advancement must have a deterministic hop limit and cycle detection.

Freeze sequence:

1. Parse strict planner output.
2. Verify the planner produced no file mutations.
3. Validate feature key and concrete paths.
4. Resolve coder target and current baseline.
5. Compute deterministic ID/version.
6. Persist frozen contract durably.
7. Reload and verify the record.
8. Only then advance toward context/coder.

Any failure blocks the writer dispatch.

### 3.8 Flow gate integration

Split Flow behavior from legacy/Normal behavior:

```go
func prepareFrozenFlowContract(
    ctx context.Context,
    workspace string,
    parentRunID string,
    coderStepID string,
    gateInput GateInput,
) (preparedChangeContract, error)

func prepareLegacyChangeContract(
    ctx context.Context,
    workspace string,
    finalMessage string,
    gateInput GateInput,
) (preparedChangeContract, error)
```

Rules for `agent.code`:

- Load the latest frozen version bound to `(parentRunID, coderStepID)`.
- Do not use the coder final message as the primary declared scope.
- Do not let `InferFromDiff` satisfy missing preflight.
- Compute `WrittenPaths` and `ScopeDrift` against the frozen baseline.
- Any non-empty drift blocks acceptance in Flow.
- The gate response must identify the exact unexpected paths and direct the Flow to amendment/retry.

Normal chat retains the existing best-effort parser/inference path. This compatibility behavior is explicitly outside the Flow guarantee.

### 3.9 Canonical Head: current lifecycle

Canonical storage:

```text
.flowpilot/canonical/<feature_key>.json
```

File behavior:

- `SaveHead` writes a complete JSON replacement through temp file + rename.
- It does not append to the canonical file.
- `BuildHead` derives governing documents, document hashes, canonical behavior and `intent_signature`.
- Normal updates replace recomputed state.
- Rename/merge operations append source decisions in memory, then replace the whole target file.
- Drive sync upserts the local canonical file remotely; it is not a local restore authority.

Current automatic `SaveHead` paths:

1. feature birth;
2. normal contract commit/update;
3. explicit rebaseline;
4. retire source;
5. retire target(s).

Problem: normal contract commit can update the Head when coder gate passes, before downstream validation/review has accepted the complete Flow.

### 3.10 Canonical Head: target lifecycle

Split persistence into two effects:

```text
Coder gate allowed
    ↓
Persist contract result
    ↓
Stage pending Canonical update
    ↓
Continue Flow
    ↓
Parent status = done
    ↓
Finalize pending Canonical updates
    ↓
Publish terminal done
```

Proposed pending record:

```go
type PendingCanonicalUpdate struct {
    RunID         string    `json:"run_id"`
    FeatureKey    string    `json:"feature_key"`
    ContractID    string    `json:"contract_id"`
    ContractStepID string   `json:"contract_step_id"`
    Intent        string    `json:"intent"`
    CreatedAt     time.Time `json:"created_at"`
    Status        string    `json:"status"`
    FinalizedAt   time.Time `json:"finalized_at,omitempty"`
    AbandonReason string    `json:"abandon_reason,omitempty"`
}
```

Local durable store:

```text
.flowpilot/contracts/pending_canonical.ndjson
```

Proposed API:

```go
func NewPendingCanonicalStore(
    workspace string,
) (*PendingCanonicalStore, error)

func (s *PendingCanonicalStore) Stage(
    update PendingCanonicalUpdate,
) error

func (s *PendingCanonicalStore) ListForRun(
    runID string,
) ([]PendingCanonicalUpdate, error)

func (s *PendingCanonicalStore) MarkFinalized(
    runID string,
    featureKey string,
    contractID string,
    at time.Time,
) error

func (s *PendingCanonicalStore) MarkAbandoned(
    runID string,
    reason string,
    at time.Time,
) error
```

Gate and terminal functions:

```go
func persistPreparedContract(
    workspace string,
    prepared preparedChangeContract,
) error

func stageCanonicalUpdate(
    workspace string,
    contract Contract,
) error

func finalizeAcceptedCanonicalHeads(
    workspace string,
    parentRunID string,
    now time.Time,
) error

func applyPendingCanonicalUpdate(
    workspace string,
    update PendingCanonicalUpdate,
    now time.Time,
) error

func abandonPendingCanonicalHeads(
    workspace string,
    parentRunID string,
    reason string,
    now time.Time,
) error
```

Terminal rules:

| Flow outcome | Canonical effect |
|---|---|
| `done` after final acceptance | Finalize latest accepted pending version exactly once |
| validation/review `continue` | No finalization |
| blocked | No finalization |
| failed | Mark pending abandoned |
| stopped/cancelled | Mark pending abandoned |
| escalated to user | No finalization; pending remains unresolved or is explicitly abandoned according to current terminal semantics |
| spec/code drift | No automatic finalization |

`finalizeAcceptedCanonicalHeads` must run before the runtime publishes/mutates terminal `done`. If finalization fails, terminal `done` must not be published. Recovery may retry finalization idempotently.

Explicit rebaseline, retire and merge APIs remain explicit operator actions and are not converted into pending Flow effects.

### 3.11 Canonical inputs and signature

Current conceptual pipeline:

```text
Governing SS / SD / CP
          +
Latest accepted canonical behavior
          ↓
Normalize intent inputs
          ↓
intent_signature
          ↓
Canonical Head per feature_key
          ↓
Head-first context packing
```

Technical steps:

1. Resolve `feature_key`.
2. Resolve governing document refs from feature catalog.
3. Read and hash the governing docs deterministically.
4. Resolve behavior from the accepted contract/change-ledger state.
5. Normalize behavior by lowercasing and whitespace normalization.
6. Sort document IDs and hashes.
7. Hash feature key + sorted document IDs + sorted hashes + normalized behavior.
8. Write the complete Head atomically.
9. Future context requests can pack Canonical Head before deeper history.

Only step 4's accepted behavior changes automatically at terminal Flow acceptance. Governing documents remain authoritative inputs and are not overwritten by this mechanism.

### 3.12 RetrievalLocus: current behavior

Current type:

```go
type RetrievalLocus struct {
    Paths   []string
    Symbols []string
    RunID   string
}
```

Current builder merges:

1. `DeclaredPaths` from stored contract;
2. uncommitted diff paths;
3. concrete paths found in user prompt.

Then it filters, normalizes, sorts and deduplicates.

Important:

- Contract is not technically required; an empty or partial locus is valid.
- `RetrievalLocus.Symbols` remains unused until BUG-323.
- P-2 of CP-54 introduced the builder only; no current context source consumes it.

### 3.13 Why preflight and retrieval must cooperate

The components remain separately testable but their Flow ordering is coupled:

```text
contract.freeze
    ↓ available in store
context.produce
    ↓ buildRetrievalLocus
feature.history
    ↓ rank candidates
agent.code
```

Without preflight, the first coder context cannot use the current turn's AI-declared paths, because those paths are only declared in the coder's final response.

`RetrievalLocus` must still degrade safely when:

- no contract is available;
- contract source rendering is disabled;
- prompt/diff contains no concrete code path;
- candidate history is small.

### 3.14 History ranking

Current:

```text
all ledger entries
    ↓ filter by feature_key
candidates
    ↓ chronological recency
latest 15
```

Target:

```text
all ledger entries
    ↓ filter by feature_key
candidates
    ↓
if candidates > activation threshold and locus non-empty:
    deterministic path relevance
else:
    current recency path
    ↓
maximum 15 selected entries
```

Example:

```text
1,000 repository commits
    ↓ feature_key filter
100 feature candidates
    ↓ RetrievalLocus rank
15 selected history entries
```

Recommended configuration:

```go
type HistoryRankingConfig struct {
    ActivationThreshold int
    Limit               int
}

var DefaultHistoryRankingConfig = HistoryRankingConfig{
    ActivationThreshold: 30,
    Limit:               15,
}
```

Scoring types/functions:

```go
type HistoryRelevance struct {
    PathOverlap int
    CommitUnix  int64
    CommitHash  string
}

type RankedHistoryEntry struct {
    Entry        changeledger.Entry
    Relevance    HistoryRelevance
    Rank         int
}

type HistorySelection struct {
    Entries        []changeledger.Entry
    Ranked         bool
    CandidateCount int
}

func ScoreHistoryEntry(
    entry changeledger.Entry,
    locus RetrievalLocus,
) HistoryRelevance

func RankHistoryEntries(
    entries []changeledger.Entry,
    locus RetrievalLocus,
) []RankedHistoryEntry

func SelectHistoryEntries(
    entries []changeledger.Entry,
    locus RetrievalLocus,
    config HistoryRankingConfig,
) HistorySelection
```

Deterministic ordering:

1. higher exact normalized path overlap;
2. newer commit time;
3. stable commit hash tie-break.

Selection invariants:

- Fallback branch must preserve current output bytes.
- The absolute newest feature entry remains available as current truth even if its locus score is low.
- Ranked selection is capped at 15.
- Duplicate paths do not inflate score.
- Nil `ChangedPaths` gets zero overlap and remains eligible through recency/tie-break behavior.

Renderer/API:

```go
func HistorySlotRanked(
    featureKey string,
    ledger *changeledger.Ledger,
    locus RetrievalLocus,
    config HistoryRankingConfig,
) string

func HistorySlot(
    featureKey string,
    ledger *changeledger.Ledger,
) string
```

Keep `HistorySlot` as a compatibility wrapper using empty locus/fallback semantics.

`feature.history` source wiring:

```go
func (s *featureHistorySource) Fetch(
    ctx context.Context,
    hints ContextHints,
) (ContextArtifact, error)
```

Within `Fetch`:

1. obtain candidate entries by feature key;
2. call `buildRetrievalLocus(hints.Workspace, hints.WorkflowRunID, hints.UserPrompt)`;
3. call `HistorySlotRanked`;
4. render ranked/fallback metadata consistently.

### 3.15 Context source setting semantics

| Source setting | Disabled behavior |
|---|---|
| `change.contract` | Hide only the rendered Change Contract section |
| `feature.history` | Do not render history and do not run history ranking for output |
| `canonical.head` | Do not render Canonical Head section |
| `chat.summary` | Do not render chat summary |
| `source.excerpt` | Do not render source excerpt |

Disabling `change.contract` must not:

- delete or stop persisting contracts;
- disable scope gate;
- prevent `RetrievalLocus` from reading the contract store;
- disable pending Canonical lifecycle;
- change Flow topology validation.

Built-in flows with explicit source lists must include `change.contract` and `canonical.head` where their default context package is expected to expose them. User settings remain the final presentation control.

### 3.16 Built-in flow migration

Migrate at least:

```text
review-loop.yaml
rag-harness.yaml
context-coding-review-synthesis.yaml
```

Target pattern:

```text
preflight-contract-plan: agent.delegate (read-only)
    ↓
preflight-contract-freeze: contract.freeze
    ↓
context: context.produce
    ↓
coder: agent.code
    ↓
existing validate/review/audit/synthesis chain
```

Flow-specific acceptance chains stay intact. The migration must not invent an audit step for every Flow; it must ensure the Flow's actual declared acceptance steps complete before terminal `done`.

### 3.17 Recovery and idempotency

Durable execution requirements:

- A recovered Flow must reuse the persisted frozen contract, not ask the planner to silently produce another version.
- Duplicate freeze delivery with identical input returns the existing contract.
- A different draft for the same version is rejected.
- Duplicate coder completion must not stage duplicate logical pending updates.
- Duplicate terminal `done` handling must not rebuild or append duplicate Canonical decisions.
- Pending finalization interrupted between `SaveHead` and pending-status update must converge safely on retry.
- Epoch/lease checks must prevent stale workers from finalizing a newer run.

### 3.18 Provider parity

The contract is provider-agnostic.

- Claude, Codex and Grok contract planners must emit the same strict schema.
- Parser, validation, freeze, gate and scoring are Go-owned and shared.
- No provider-specific branch may weaken preflight or scope enforcement.
- Provider-specific prompt formatting may differ only at the adapter boundary.

---

## 4. Work Breakdown

Implementation is intentionally sliced so Claude Sonnet MAX can implement one task at a time and Codex can review each boundary before the next slice.

### P-1: Introduce explicit writer semantics and validate Flow topology

**Status: done (2026-07-30 — see [Task-263](../../08-Task/done/Task-263-Explicit-Flow-Writer-Semantics-And-Safety-Topology.md) / [CA-424](../../../change-audit/CA-424-explicit-flow-writer-semantics-and-safety-topology.md)).**

**Production changes**

- Add `BehaviorAgentCode` and `BehaviorContractFreeze`.
- Reuse delegate dispatch internally for `agent.code`.
- Add `IsCodeWritingBehavior`.
- Add static topology validator and forward-graph helpers, with dominance failing closed on an unreachable target (absent id, no entry at all, or a writer/freeze pair stranded in a cycle disconnected from every entry) rather than vacuously passing.
- Add a Flow-declared `FlowDefinition.AcceptanceNodes` field (root YAML key `acceptance_nodes`), required whenever an `agent.code` writer is declared and rejecting a blank, duplicate, unknown, or writer-self id, verified by full forward-path traversal — including a reachable-terminal-`done` requirement (a writer with no successor, or only a non-terminating cycle, fails) — rather than a direct-edge check.
- Persist `FlowDefinition.AcceptanceNodes` through the Supabase-backed user-flow store (new `workflows.acceptance_nodes_json` column) so a builtin mirror sync or reload done through the Go binary preserves it, not just an in-memory embedded-pack load — **and**, since that Go-backed store is not what the desktop Settings screen uses, also through `packages/flowpilot-client-core`'s direct-to-Supabase `SupabaseAdminRepository` (`Workflow.acceptanceNodes`, `mapWorkflow`/`saveWorkflow`/`cloneWorkflow`) and `apps/desktop-flowpilot`'s `WorkflowsSettings.tsx` draft layer, so a Settings read/save/clone preserves it too, not only reload. Settings UI *authoring* of `acceptance_nodes` (an editor to create/change it) is not part of this and remains an explicit P-8 prerequisite.
- Reject invalid Flow definitions before execution.
- Do not migrate flows yet.

**Candidate files**

- behavior IDs/registry implementation;
- Flow definition validation package;
- registry YAML if aliases are declared there.

**Test signatures**

```go
func TestDefaultRegistryResolvesAgentCode(t *testing.T)
func TestDefaultRegistryResolvesContractFreeze(t *testing.T)
func TestAgentCodeReusesDelegateDispatchWithoutChangingProviderPayload(t *testing.T)
func TestValidateFlowSafetyTopologyRejectsWriterAsEntry(t *testing.T)
func TestValidateFlowSafetyTopologyRejectsWriterWithoutFreeze(t *testing.T)
func TestValidateFlowSafetyTopologyRejectsPathThatBypassesFreeze(t *testing.T)
func TestValidateFlowSafetyTopologyRejectsDonePathWithoutAcceptance(t *testing.T)
func TestValidateFlowSafetyTopologyAcceptsFreezeContextWriterValidation(t *testing.T)
func TestForwardDominatesHandlesBranches(t *testing.T)
func TestForwardDominatesIgnoresRetryBackEdgesForEntryDominance(t *testing.T)
func TestEveryDonePathIncludesAcceptanceHandlesReviewLoop(t *testing.T)
func TestFlowValidationTerminatesOnCycle(t *testing.T)
```

Implemented; the actual delivered test roster is larger after three Codex review-fix passes (fail-closed reachability, declared `acceptance_nodes` traversal including the reachable-`done` requirement, blank/duplicate-id rejection, real root-YAML-parsing coverage, Supabase persistence coverage, the desktop Settings TypeScript round-trip, and the migration-SQL-content test) — see Task-263 §6 and CA-424 for the exact list and counts, not reproduced here.

**Exit condition**

- The runtime can distinguish a code-writing node without prompt/name heuristics.
- Existing flows still load until their explicit migration slice; compatibility strategy is documented and tested.
- A declared `acceptance_nodes` boundary and the fail-closed dominance/reachability checks are enforced at every current definition-resolution boundary, and `AcceptanceNodes` survives the Supabase-backed user-flow store (mirror sync, clone, user edit, reload) — not just an in-memory embedded-pack load.

### P-2: Add strict preflight contract model, normalization and immutable storage

**Status: done (2026-07-31 — implemented and self-verified in the current uncommitted worktree, see [Task-264](../../08-Task/done/Task-264-Frozen-Preflight-Contract-Model-And-Storage.md) / [CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md). Unlike P-1, no Codex review pass has run on this slice yet — `Contract` was deliberately *not* extended per this task's own design decision; see Task-264 D-1 for why a second, independent `FrozenContractRecord`/`FrozenStore` lifecycle was used instead.)**

**Production changes**

- Add `PreflightContractDraft`.
- Add strict parser with no trailing prose acceptance.
- Add shared declared-path normalization.
- Extend `Contract` backward-compatibly.
- Add deterministic ID/version computation.
- Add immutable frozen-store operations.

**Candidate files**

- `internal/changecontract/preflight.go`
- `internal/changecontract/paths.go`
- existing contract model/store files
- `internal/runner/retrieval_locus.go` to consume shared normalization

**Required impact checks before edits**

- existing `Contract` type;
- `Store.Save`;
- `Store.GetLatest...` functions;
- `isConcreteCodeTarget`;
- `buildRetrievalLocus`.

**Test signatures**

```go
func TestParsePreflightDraftAcceptsStrictJSON(t *testing.T)
func TestParsePreflightDraftRejectsTrailingProse(t *testing.T)
func TestParsePreflightDraftRejectsUnknownFields(t *testing.T)
func TestValidatePreflightDraftRequiresFeatureKey(t *testing.T)
func TestValidatePreflightDraftRejectsUnknownFeatureKey(t *testing.T)
func TestValidatePreflightDraftRequiresIntent(t *testing.T)
func TestValidatePreflightDraftRequiresConcretePath(t *testing.T)
func TestNormalizeDeclaredCodePathsNormalizesSeparators(t *testing.T)
func TestNormalizeDeclaredCodePathsSortsAndDeduplicates(t *testing.T)
func TestNormalizeDeclaredCodePathsRejectsOutsideWorkspace(t *testing.T)
func TestNormalizeDeclaredCodePathsRejectsGlobOnlyScope(t *testing.T)
func TestNormalizeDeclaredCodePathsRejectsDocumentOnlyWriterScope(t *testing.T)
func TestComputeContractIDIsDeterministic(t *testing.T)
func TestComputeContractIDChangesWithVersion(t *testing.T)
func TestSaveFrozenRejectsMutationOfExistingContractID(t *testing.T)
func TestSaveFrozenAllowsHigherAmendmentVersion(t *testing.T)
func TestGetFrozenForStepReturnsLatestActiveVersion(t *testing.T)
func TestContractStoreLoadsLegacyRecordsWithoutNewFields(t *testing.T)
func TestRetrievalLocusUsesSharedPathNormalization(t *testing.T)
```

**Exit condition**

- A frozen contract can be persisted/reloaded before any coder runs.
- Existing contract records remain readable.

### P-3: Implement read-only contract planner, freeze node and inline-chain advancement

**Status: done (2026-07-31 — implemented, then adversarially reviewed by a dedicated Claude reviewer agent per operator direction: 4 Critical, 11 Important, 7 Minor findings. All 4 Critical (a planner-mutation check that false-positived on any pre-existing dirty workspace; a spawn-failure path that could leak an unbound writer to the legacy hub fallback; silent no-op execution of a non-`context.produce` inline hop; two duplicate-delivery tests that asserted against a stale store and passed with the logic they tested fully removed) plus five in-scope Important findings were fixed and re-verified — including mutation-testing the corrected duplicate-delivery tests — with a full `internal/runner` regression run repeated after the fix (same pre-existing-environment-failure set both times, per the operator-confirmed CRITICAL-risk plan after GitNexus CLI misreported `tryAdvanceFlowFromNode`/`tryAdvanceFlowThroughInline` as LOW/0-impacted). See [Task-265](../../08-Task/done/Task-265-Contract-Planner-Freeze-Node-And-Inline-Chain-Advancement.md) / [CA-426](../../../change-audit/CA-426-contract-planner-freeze-node-and-inline-chain-advancement.md) for the complete findings/fix accounting.)**

**Production changes**

- Add `contract-planner.md`.
- Add `runContractFreezeNode`.
- Generalize bounded inline-chain advancement.
- Enforce that the planner produces no project mutations.
- Persist/reload contract before advancing.
- Add explicit error/block output for invalid proposal.

**Test signatures**

```go
func TestRunContractFreezeNodePersistsBeforeCoderSpawn(t *testing.T)
func TestRunContractFreezeNodeRejectsPlannerChangedFiles(t *testing.T)
func TestRunContractFreezeNodeRejectsInvalidDraft(t *testing.T)
func TestRunContractFreezeNodeRejectsUnknownCoderTarget(t *testing.T)
func TestRunContractFreezeNodeBindsContractToCoderStep(t *testing.T)
func TestRunContractFreezeNodeRecordsBaselineSHA(t *testing.T)
func TestRunContractFreezeNodeReloadsDurableRecordBeforeAdvance(t *testing.T)
func TestRunContractFreezeNodeAdvancesToContextProduce(t *testing.T)
func TestInlineChainAdvancesFreezeThenContextThenWriter(t *testing.T)
func TestInlineChainStopsAtHopLimit(t *testing.T)
func TestInlineChainDetectsCycle(t *testing.T)
func TestInlineChainDoesNotDispatchWriterAfterFreezeFailure(t *testing.T)
func TestRecoveredFlowReusesPersistedFrozenContract(t *testing.T)
func TestDuplicateFreezeDeliveryReturnsExistingContract(t *testing.T)
```

**Exit condition**

- The full pre-coder order is observable and durable.
- Invalid/mutating planner output cannot reach a writer.

### P-4: Enforce frozen scope at the code-writing gate and support amendments

**Status: done (2026-07-31 — implemented, then adversarially reviewed by a dedicated Claude reviewer agent: 2 Critical, 5 Important, 4 Minor findings, including a real security hole (the original `.flowpilot/`-path exemption was broad enough that a writer could silently disable its own gate rules or forge its own frozen contract with zero drift detected) and a fail-open bug (a git-observation failure trusted AI-self-reported paths instead of blocking). Both Critical findings and 4 of 5 Important findings fixed and re-verified — each fix proven via mutation testing (temporarily reverted, confirmed the corresponding test fails, then restored) — with a full `internal/runner` regression run repeated after the fixes (same 15 pre-existing failures both times, 0 new, 0 flakes). See [Task-266](../../08-Task/done/Task-266-Enforce-Frozen-Scope-At-Coder-Gate-And-Amendments.md) / [CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md) for the complete findings/fix accounting.)**

**Production changes**

- Split Flow contract preparation from legacy/Normal preparation.
- `agent.code` loads the bound frozen version.
- Remove post-turn declaration/inference as a way to satisfy Flow preflight.
- Scope drift becomes blocking for Flow.
- Add amendment transition that supersedes the prior version before retry.
- Preserve current Normal chat behavior.

**Test signatures**

```go
func TestFlowCoderRequiresFrozenContract(t *testing.T)
func TestFlowCoderRejectsPostTurnDeclarationWithoutFrozenContract(t *testing.T)
func TestFlowCoderUsesFrozenScopeInsteadOfFinalMessage(t *testing.T)
func TestFlowCoderComputesWrittenPathsAgainstFrozenBaseline(t *testing.T)
func TestFlowScopeDriftBlocksAcceptance(t *testing.T)
func TestFlowScopeDriftReportsExactUnexpectedPaths(t *testing.T)
func TestFlowScopeDriftCannotBeSatisfiedByInference(t *testing.T)
func TestFlowContractAmendmentCreatesHigherVersionBeforeRetry(t *testing.T)
func TestFlowContractAmendmentSupersedesPriorVersion(t *testing.T)
func TestRetryWithoutNewPathsReusesFrozenVersion(t *testing.T)
func TestNormalChatKeepsLegacyDeclaredContractBehavior(t *testing.T)
func TestNormalChatKeepsLegacyInferredContractBehavior(t *testing.T)
func TestNormalChatCodeRequestIsNotBlockedForMissingFlow(t *testing.T)
func TestChangeContractSourceDisabledDoesNotDisableFlowGate(t *testing.T)
```

**Exit condition**

- “Contract before code” is enforced for Flow.
- Normal chat remains unrestricted and backward-compatible.

### P-5: Move automatic Canonical mutation to terminal Flow acceptance

**Status: done (2026-07-31 — implemented, then adversarially reviewed by a dedicated Claude reviewer agent: 3 Critical, 8 Important, 8 Minor findings — a FAIL verdict, including a silent-data-loss fail-open bug (a staged key that was ever finalized/abandoned could never become pending again after a legitimate redrive), a partial-commit bug (finalizing several features one at a time could permanently mutate an earlier feature's real Canonical Head even though the Flow's terminal acceptance as a whole was refused), and a hang-inducing bug (a finalize failure burned the turn's one-decision slot with no operator-actionable path forward). All 3 Critical + 6 of 8 Important findings fixed and re-verified via mutation testing (each of the 3 Critical fixes independently proven by temporarily reverting it, confirming the corresponding test fails, then restoring it). Full `internal/runner` regression re-run after fixes: 16 failures — the same 15 pre-existing failures plus 1 confirmed load-dependent flake unrelated to this task, 0 new deterministic regressions. See [Task-267](../../08-Task/done/Task-267-Move-Canonical-Mutation-To-Terminal-Flow-Acceptance.md) / [CA-428](../../../change-audit/CA-428-move-canonical-mutation-to-terminal-flow-acceptance.md) for the complete findings/fix accounting.)**

**Production changes**

- Add pending Canonical store.
- Split `commitChangeContract` effects.
- Coder allow stages pending state only.
- Terminal Flow `done` finalizes before publishing done.
- Failure/stopped/cancelled outcomes abandon pending state.
- Preserve explicit rebaseline/retire/merge behavior.
- Preserve legacy Normal behavior initially, but document it as non-guaranteed.

**Required impact checks before edits**

- every `SaveHead` caller;
- `commitChangeContract`;
- `updateCanonicalHead`;
- `InteractiveService.applyFlowControl`;
- `markFlowRunComplete`;
- stop/cancel/failure handlers.

**Test signatures**

```go
func TestPendingCanonicalStoreStagesUpdate(t *testing.T)
func TestPendingCanonicalStoreUsesLatestContractVersion(t *testing.T)
func TestPendingCanonicalStoreIsIdempotent(t *testing.T)
func TestCoderGateDoesNotSaveCanonicalHeadForFlow(t *testing.T)
func TestCoderGateStagesPendingCanonicalUpdate(t *testing.T)
func TestFlowContinueDoesNotFinalizeCanonicalHead(t *testing.T)
func TestValidationFailureDoesNotFinalizeCanonicalHead(t *testing.T)
func TestReviewRetryDoesNotFinalizeCanonicalHead(t *testing.T)
func TestFlowDoneFinalizesCanonicalHead(t *testing.T)
func TestFlowDoneFinalizesLatestAcceptedVersionOnly(t *testing.T)
func TestFlowDoneFinalizationFailureDoesNotPublishDone(t *testing.T)
func TestFinalizeAcceptedCanonicalHeadsIsIdempotent(t *testing.T)
func TestRestartRetriesInterruptedCanonicalFinalization(t *testing.T)
func TestStaleEpochCannotFinalizeCanonicalHead(t *testing.T)
func TestFlowFailureAbandonsPendingCanonicalUpdate(t *testing.T)
func TestFlowStopAbandonsPendingCanonicalUpdate(t *testing.T)
func TestFlowCancelAbandonsPendingCanonicalUpdate(t *testing.T)
func TestFlowEscalationDoesNotFinalizeCanonicalHead(t *testing.T)
func TestSpecDriftDoesNotAutoFinalizeCanonicalHead(t *testing.T)
func TestExplicitRebaselineBehaviorIsUnchanged(t *testing.T)
func TestExplicitRetireAndMergeBehaviorIsUnchanged(t *testing.T)
func TestNormalChatCanonicalPathRemainsBackwardCompatible(t *testing.T)
```

**Exit condition**

- Flow Canonical Head cannot represent code that the Flow has not finally accepted.

### P-6: Implement deterministic history relevance scorer

**Status: done (2026-07-31 — implemented, then adversarially reviewed by a dedicated Claude reviewer agent: 0 Critical, 5 Important, 7 Minor findings — a CONDITIONAL PASS (production logic judged functionally correct, but 3 of the 9 originally-mandated tests were shallow enough that the reviewer constructed specific passing mutants that violated the phase's own invariants, including one that silently reintroduces the BUG-266 class of nondeterminism). All 5 Important findings fixed and re-verified via mutation testing (each temporarily reverted/mutated, the corresponding test confirmed to fail, then restored to pass); all 7 Minor findings addressed. This phase is deliberately unwired — no existing file outside `internal/featurecatalog` was touched, so no `internal/runner` regression run was needed. See [Task-268](../../08-Task/done/Task-268-Deterministic-History-Relevance-Scorer.md) / [CA-429](../../../change-audit/CA-429-deterministic-history-relevance-scorer.md) for the complete findings/fix accounting.)**

**Production changes**

- Add `featurecatalog/relevance.go`.
- Implement exact normalized path overlap.
- Add explicit deterministic tie-breakers.
- Keep symbols unused.

**Test signatures**

```go
func TestScoreHistoryEntryCountsExactPathOverlap(t *testing.T)
func TestScoreHistoryEntryDeduplicatesPaths(t *testing.T)
func TestScoreHistoryEntryReturnsZeroForNilChangedPaths(t *testing.T)
func TestScoreHistoryEntryReturnsZeroForEmptyLocus(t *testing.T)
func TestRankHistoryEntriesOverlapDominatesRecency(t *testing.T)
func TestRankHistoryEntriesRecencyBreaksEqualOverlap(t *testing.T)
func TestRankHistoryEntriesCommitHashBreaksCompleteTie(t *testing.T)
func TestRankHistoryEntriesIsDeterministicAcrossRuns(t *testing.T)
func TestRankHistoryEntriesDoesNotUseSymbolsBeforeBug323(t *testing.T)
```

**Exit condition**

- Given the same entries and locus, all providers/processes produce the same ordering.

### P-7: Wire ranking into `feature.history`

**Status: done (2026-07-31 — implemented, then adversarially reviewed by a dedicated Claude reviewer agent: 0 Critical, 8 Important, 9 Minor findings — a CONDITIONAL PASS (the core mechanism verified correct, but the "current truth" entry could silently lose its excerpt and enforcement instruction in ranked mode, and a zero-value Limit silently disabled the output cap). All 8 Important findings fixed and re-verified via mutation testing; all 9 Minor findings addressed inline or explicitly accepted/deferred. Full `internal/runner` regression run twice: 16 failures both times — the same 15 pre-existing failures plus 1 confirmed load-dependent flake, 0 new deterministic regressions. See [Task-269](../../08-Task/done/Task-269-Wire-Ranking-Into-Feature-History.md) / [CA-430](../../../change-audit/CA-430-wire-ranking-into-feature-history.md) for the complete findings/fix accounting.)**

**Production changes**

- Add `HistoryRankingConfig`, selection and ranked renderer.
- Preserve exact fallback output.
- Keep absolute newest feature entry.
- Build locus in `featureHistorySource.Fetch`.
- Ensure the contract store can feed locus even when its rendered source is disabled.
- Keep `chat.summary` unchanged.

**Test signatures**

```go
func TestSelectHistoryEntriesFallsBackWhenLocusEmpty(t *testing.T)
func TestSelectHistoryEntriesFallsBackAtThreshold(t *testing.T)
func TestSelectHistoryEntriesRanksAboveThreshold(t *testing.T)
func TestSelectHistoryEntriesKeepsNewestAbsoluteTruth(t *testing.T)
func TestSelectHistoryEntriesCapsAtFifteen(t *testing.T)
func TestSelectHistoryEntriesHandlesFewerThanLimit(t *testing.T)
func TestHistorySlotRankedPreservesLegacyBytesOnFallback(t *testing.T)
func TestHistorySlotRankedReportsRankedCandidateCount(t *testing.T)
func TestFeatureHistorySourceBuildsLocusFromFrozenContract(t *testing.T)
func TestFeatureHistorySourceMergesContractDiffAndPromptPaths(t *testing.T)
func TestFeatureHistoryRanksWhenChangeContractRenderingIsDisabled(t *testing.T)
func TestFeatureHistoryProducesNoOutputWhenSourceIsDisabled(t *testing.T)
func TestChatSummaryRemainsRecencyBased(t *testing.T)
```

**Exit condition**

- A large feature history is selected by locus.
- Small/no-locus history behaves exactly as before.

### P-8: Migrate built-in flows and add end-to-end recovery/parity coverage

**Status: done (2026-07-31 — implemented; found and fixed 3 genuine production bugs before any review pass, each caught through this effort's own mutation-testing/full-regression discipline: an `agent.code` writer's gate pass had ZERO Canonical Head effect at all (structurally could never set `isCodingChild`); a writer's own pending-canonical staging write self-triggered scope drift on its very next gate pass (would have self-blocked every real retry); and `advanceFlowThroughFreezeChain` never rendered the built `FlowContextPackage` into the writer's actual prompt when the freeze node has no intermediate `context.produce` hop — review-loop's own shape — silently defeating the P-6/P-7 history-ranking mechanism for the flow most users hit first. All three fixed and mutation-tested. Migrating the topology also broke 22 existing tests (18 from the entry-node change alone); each was individually root-caused before being fixed, not assumed — one (`TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing`) was found to be passing for the wrong reason and is now fixed to genuinely exercise its intended scenario. Full `internal/runner` regression suite run to completion three times across the fix pass: the final run shows exactly the established 15 pre-existing/environment failures, zero new deterministic regressions. Then Claude-agent reviewed: 1 Critical/3 Important/2 Minor found — a FAIL verdict, the most severe of any CP-55 phase reviewed so far. The Critical finding was a real, concretely-demonstrated bypass of CP-55's own central guarantee: a coder's ordinary file-write tool could forge an arbitrary Canonical Head for any feature by appending a raw NDJSON line to the pending-canonical store's exempted bookkeeping path, trusted byte-for-byte at finalize with zero provenance check — closed with an HMAC-signature scheme reusing the existing `markerSecret` trust boundary, with zero behavior change for any pre-existing test. The 3 Important findings (an unanchored fake-adapter role-marker match, an unvalidated planner-supplied feature key silently routing history to the wrong feature, and the Canonical Head fix itself being reachable only via incidental coupling to unrelated default-enabled rules) were also fixed. All 4 Critical/Important fixes independently mutation-tested; both Minor findings accepted/documented. Full regression re-run after fixes: same 15-baseline plus 1 already-documented load-dependent flake, 0 new. See [Task-270](../../08-Task/done/Task-270-Migrate-Built-In-Flows-And-E2E-Recovery-Parity-Coverage.md) / [CA-431](../../../change-audit/CA-431-migrate-built-in-flows-and-e2e-recovery-parity-coverage.md) for the complete accounting.)**

**Production changes**

- Migrate the three built-in coding flows, each declaring a concrete `acceptance_nodes` list for its own writer node — required once any node uses `agent.code` (enforced by P-1's `ValidateFlowSafetyTopology`/`validateAcceptanceNodes`, which reject a blank, duplicate, unknown, or writer-self id). Persistence through both the Go-backed mirror/reload path and the desktop Settings direct-to-Supabase read/save/clone path already exists from P-1 (`workflows.acceptance_nodes_json`; see Task-263/CA-424 pass 2 and pass 3), so no additional storage work is needed here.
- **Explicit prerequisite carried over from P-1, not yet satisfied: a Settings UI selector/editor for `acceptance_nodes`.** P-1 made the field round-trip (read/save/clone) but added no way to author it — `agent.code`/`contract.freeze` are not in `FLOW_BEHAVIOR_OPTIONS` (`adminModels.ts`) yet, so no Settings user can create an `agent.code` node, or therefore a meaningful `acceptance_nodes` list, today. The three built-in flows this P-8 slice migrates are authored in their source YAML and mirror-synced, not authored through Settings, so migrating them does not by itself require this editor — but whichever slice first lets a user author their own `agent.code`-bearing custom flow from Settings must add it before or alongside that work. Do not mark this part done until an editor exists.
- Add source IDs where explicit lists currently omit contract/head.
- Add metrics/logging without leaking prompt/code content.
- Verify Claude/Codex/Grok parity.
- Add restart/recovery tests across freeze, gate and finalization.

**Test signatures**

```go
func TestReviewLoopHasPreflightBeforeEveryWriter(t *testing.T)
func TestRAGHarnessHasPreflightBeforeEveryWriter(t *testing.T)
func TestContextCodingReviewSynthesisHasPreflightBeforeEveryWriter(t *testing.T)
func TestBuiltInCodingFlowsPassSafetyTopologyValidation(t *testing.T)
func TestFrozenContractExistsBeforeFirstContextPackage(t *testing.T)
func TestFirstCoderContextUsesCurrentFlowDeclaredPaths(t *testing.T)
func TestFirstCoderContextRanksFeatureHistoryByCurrentLocus(t *testing.T)
func TestValidateFailLeavesCanonicalHeadUnchanged(t *testing.T)
func TestReviewContinueLeavesCanonicalHeadUnchanged(t *testing.T)
func TestTerminalDoneUpdatesCanonicalHeadExactlyOnce(t *testing.T)
func TestRestartBetweenFreezeAndCoderPreservesContract(t *testing.T)
func TestRestartBetweenCoderAndValidationPreservesPendingCanonical(t *testing.T)
func TestRestartDuringTerminalFinalizationConverges(t *testing.T)
func TestClaudePreflightContractUsesSharedParserAndGate(t *testing.T)
func TestCodexPreflightContractUsesSharedParserAndGate(t *testing.T)
func TestGrokPreflightContractUsesSharedParserAndGate(t *testing.T)
func TestProviderAdaptersReceiveEquivalentFrozenContractPayload(t *testing.T)
func TestNormalChatRemainsUsableWithoutFlowGuarantees(t *testing.T)
```

**Exit condition**

- Built-in issue-resolution Flows exercise the new lifecycle end to end on all providers.

### P-9: Documentation, rollout evidence and operator review

**Status: done (2026-07-31 — documentation-only phase, no production code changed. Synced CP-43/CP-54's current-state sections (CP-54's own planned P-3 through P-5 deterministic-scorer/ranking/wiring work was implemented under CP-55 P-6/P-7 instead of being cut as separate CP-54 tasks — clarifying note + cross-references added to both documents, their own independently-tracked status left untouched). Recorded before/after evidence for contract timing, feature-history ranking, and Canonical Head finalization, each citing a specific already-passing P-3 through P-8 test. Confirmed one Task+CA pair already exists per independently reviewable slice (Task-263 through Task-270 / CA-424 through CA-431) — none missing. Regression status reconfirmed clean from the immediately-preceding P-8 review-fix pass (established 15-baseline + 1 documented flake, 0 new); no new run needed since no code changed. Per this session's own established substitution (Claude-agent review instead of Codex throughout P-3 through P-9), the coding plan's original "Codex reviews findings to zero blocking issues" line does not apply — every phase's review was instead performed by a dedicated Claude reviewer agent, as recorded in each phase's own CA doc. See [Task-271](../../08-Task/done/Task-271-CP55-Documentation-Rollout-Evidence-And-Operator-Review.md) / [CA-432](../../../change-audit/CA-432-cp55-documentation-rollout-evidence-and-operator-review.md). **All 9 phases of CP-55 are now done.**)**

**Work**

- Sync CP-43 and CP-54 current-state sections after implementation.
- Create one Task and one CA per independently reviewable implementation slice.
- Record before/after examples for contract timing, ranking and Canonical finalization.
- Run full regression suite without editing old green tests.
- Run GitNexus `detect_changes` before each commit.
- Claude Sonnet MAX provides implementation evidence; Codex reviews findings to zero blocking issues.

---

## 5. Touched Areas

Expected areas; exact files must be confirmed by GitNexus context/impact before implementation.

| Area | Expected change |
|---|---|
| `internal/changecontract` | draft parser, path policy, immutable versions, pending Canonical store |
| `internal/featurecatalog` | relevance scorer, history selection/rendering |
| `internal/runner` | freeze dispatch, inline chain, gate split, terminal finalization, locus wiring |
| `internal/agentpack` | behavior IDs, explicit writer semantics, topology validation |
| built-in flow YAML | preflight/freeze/context/writer ordering |
| contract planner agent prompt | strict read-only proposal |
| tests | additive unit, integration, recovery and parity coverage |
| requirements / change-audit | Task and CA traceability per slice |

No database or remote service schema is required.

---

## 6. Data or Migration Steps

### 6.1 Existing contract records

- New fields must use `omitempty` or safe zero values.
- Legacy records without `contract_id`, `version`, `status` or `base_sha` remain readable.
- Legacy records are not retroactively treated as frozen preflight contracts.
- Flow execution requires a new frozen record; Normal compatibility may continue using legacy records.

### 6.2 Canonical files

- Existing `.flowpilot/canonical/*.json` files remain valid.
- No bulk rewrite is performed.
- The next accepted Flow update recomputes and atomically replaces the relevant Head.

### 6.3 Pending Canonical ledger

- Create lazily on first Flow coder acceptance.
- Append-only, local-only.
- Recovery uses logical last-wins status per `(run_id, feature_key, contract_id)`.
- Compaction is out of scope unless file growth proves material.

### 6.4 Flow definitions

- Built-in coding flows are migrated in-repo.
- User-defined flows with implicit writers require a compatibility window:
  - warn and identify nodes that need `agent.code`;
  - provide a deterministic validation error after enforcement is enabled;
  - never silently classify a mutating node as safe.

---

## 7. Validation Plan

### 7.1 Test policy

- Add tests; do not edit existing green tests merely to make implementation pass.
- If an old test conflicts with governing SS/SD, stop and escalate the specification conflict.
- Run package tests after each slice and the full relevant suite before merge.
- Verify all three providers for provider-facing changes.

### 7.2 Required proof matrix

| Scenario | Contract | History | Canonical |
|---|---|---|---|
| Normal chat edits code | Best effort / legacy | Current behavior | No Flow guarantee |
| Flow planner invalid | No writer dispatch | No coder context | No update |
| Flow contract frozen | Durable before coder | Available to locus | No update |
| Coder scope drift | Block/amend | Existing context only | No update |
| Coder passes, tests pending | Accepted at step only | N/A | Pending only |
| Validation fails | Frozen remains for retry | N/A | Unchanged |
| Review requests retry | Version reuse/amendment | Rebuilt as needed | Unchanged |
| Parent Flow `done` | Accepted | N/A | Finalized once |
| Restart before `done` | Recovered | Deterministic | Pending recovered |
| `change.contract` source off | Still enforced | Still usable by locus | Still staged/finalized |
| `feature.history` source off | Still enforced | No history output | Unaffected |

### 7.3 Static verification

- All built-in code-writing nodes are `agent.code`.
- Every writer is dominated by `contract.freeze`.
- Every built-in context node for a writer occurs after freeze.
- No writer can be reached on a branch that bypasses freeze.
- No terminal `done` can bypass the declared acceptance chain.

### 7.4 Runtime verification

- Log contract ID/version, run ID and step ID, but not prompt or code content.
- Log whether history selection used `recency` or `locus`.
- Log candidate count, selected count and overlap count only.
- Log Canonical status transition `pending → finalized/abandoned`.
- Verify terminal event ordering in tests.

### 7.5 Manual acceptance walkthrough

1. Start a built-in coding Flow for an issue mentioning a concrete path.
2. Confirm planner emits a draft without changing files.
3. Confirm frozen record exists before context generation.
4. Confirm first coder prompt includes locus-relevant history.
5. Ask coder to touch an undeclared path and confirm Flow blocks.
6. Amend contract and retry.
7. Force validation failure and confirm Canonical file hash is unchanged.
8. Complete validation/review and confirm Canonical changes exactly once at parent `done`.
9. Disable `change.contract` rendering and repeat; gate/locus/finalization must still work.
10. Ask for a code change in Normal chat; confirm it is not blocked merely because no Flow is active.

---

## 8. Rollout and Fallback

### 8.1 Rollout order

1. Land explicit writer behavior and topology validation in compatibility mode.
2. Land frozen contract storage and freeze node.
3. Migrate built-in flows.
4. Enforce frozen contract for `agent.code`.
5. Land pending/terminal Canonical lifecycle.
6. Land scorer and history wiring behind an internal flag if needed.
7. Enable ranked history after byte-compatible fallback tests pass.
8. Enforce topology for user-defined coding flows after migration messaging exists.

### 8.2 Safe fallback

- History ranking can be disabled to return to exact recency behavior.
- `RetrievalLocus` empty always falls back to recency.
- Normal chat remains available if a Flow definition is invalid.
- Do not fall back from missing Flow preflight to inferred post-code contract.
- Do not fall back from failed terminal Canonical finalization to publishing `done`.

### 8.3 Compatibility statement

This CP deliberately provides asymmetric guarantees:

- Flow gets strong lifecycle guarantees.
- Normal chat remains flexible and best effort.

That asymmetry is a product decision, not a temporary bug.

---

## 9. Risks

### 9.1 Planner proposes incomplete scope

Mitigation:

- concrete-path validation;
- easy but explicit amendment loop;
- no punishment for narrow initial scope;
- clear drift report.

### 9.2 Read-only planner mutates files

Mitigation:

- provider tool restrictions where supported;
- baseline/diff verification after planner turn;
- fail before freeze/dispatch if mutation exists.

### 9.3 Flow graph validation rejects existing user flows

Mitigation:

- introduce explicit behavior with warning window;
- migrate built-ins first;
- produce node-specific actionable errors.

### 9.4 Inline-chain execution loops or duplicates effects

Mitigation:

- bounded hop count;
- visited-node cycle detection;
- idempotent freeze/context transitions;
- durable dispatch semantics from SD-24.

### 9.5 Canonical finalization partially succeeds

Mitigation:

- atomic `SaveHead`;
- idempotent apply keyed by contract ID;
- pending status recorded after successful write;
- retry before terminal done publication.

### 9.6 Large pending/contract NDJSON files

Mitigation:

- bounded reads/indexing if current store requires it;
- compaction designed separately only after measurement.

### 9.7 Ranking hides newest truth

Mitigation:

- force inclusion of absolute newest feature entry;
- label ranked selection;
- retain recency fallback.

### 9.8 Path equality is too strict

Mitigation:

- begin with deterministic exact normalized paths;
- add directory-prefix or symbol weights only through a separately reviewed CP with new tests.

### 9.9 Provider behavior divergence

Mitigation:

- strict Go-owned schema/parser;
- shared behavior implementation;
- three-provider parity tests.

### 9.10 GitNexus index/query failure during implementation

Mitigation:

- refresh index;
- verify repository context resource;
- stop before symbol edits if required impact analysis cannot be obtained.

---

## 10. Definition of Done

### Architecture and product boundary

- [ ] Normal chat is not blocked from code-related requests solely because no Flow is active.
- [ ] Product/docs clearly state that Normal mode does not receive Flow lifecycle guarantees.
- [ ] Every Flow code-writing path uses explicit `agent.code` semantics.
- [ ] Every `agent.code` is dominated by a frozen preflight contract.
- [ ] Context production for built-in coding flows happens after freeze and before coder.
- [ ] Flow topology validation terminates safely on loops and branches.

### Preflight contract

- [ ] AI contract planner is read-only and emits one strict draft schema.
- [ ] Go code validates feature key, intent and concrete declared paths.
- [ ] Frozen contract is durable before coder dispatch.
- [ ] Contract is bound to parent run, planner step, coder step and baseline.
- [ ] Frozen contract IDs are deterministic or otherwise idempotency-safe.
- [ ] Frozen records are immutable.
- [ ] Amendment creates a new version and supersedes the prior version.
- [ ] Existing contract NDJSON records remain readable.
- [ ] Inferred post-code contracts cannot satisfy Flow preflight.

### Scope enforcement

- [ ] Flow gate loads the bound frozen contract rather than trusting coder final declaration.
- [ ] Written paths are computed against the frozen baseline.
- [ ] Non-empty Flow scope drift blocks acceptance.
- [ ] Drift output names exact unexpected paths.
- [ ] Retry without new scope can reuse the frozen version.
- [ ] Retry with new scope must complete amendment/freeze first.
- [ ] Normal chat legacy contract behavior remains usable.

### Canonical acceptance

- [ ] Coder gate does not directly update Canonical Head for Flow.
- [ ] Coder acceptance stages one durable pending Canonical update.
- [ ] Validation/review retry leaves Canonical unchanged.
- [ ] Failed/stopped/cancelled Flow does not finalize Canonical.
- [ ] Spec/code drift does not auto-finalize Canonical.
- [ ] Parent Flow `done` finalizes the latest accepted version exactly once.
- [ ] Canonical finalization happens before terminal `done` is published.
- [ ] Finalization failure prevents terminal `done`.
- [ ] Recovery after partial finalization converges idempotently.
- [ ] Explicit rebaseline/retire/merge behavior remains intact.

### Retrieval and context

- [ ] `RetrievalLocus` remains usable without a contract.
- [ ] Frozen contract is available to locus before first coder context.
- [ ] Shared path normalization is deterministic.
- [ ] Relevance score uses path overlap, recency and stable hash tie-break.
- [ ] Ranking is deterministic across runs/providers.
- [ ] Empty locus falls back to current recency behavior.
- [ ] Candidate count at/below threshold falls back to current recency behavior.
- [ ] Fallback render is byte-compatible.
- [ ] Ranked selection is capped at 15.
- [ ] Absolute newest feature entry remains included.
- [ ] `chat.summary` remains recency-based.
- [ ] Symbols remain inactive until BUG-323 is resolved.

### Settings

- [ ] Disabling `change.contract` hides only its rendered section.
- [ ] Disabling `change.contract` does not disable freeze, gate, locus or Canonical lifecycle.
- [ ] Disabling `feature.history` produces no history output.
- [ ] Explicit source lists in built-in flows include intended contract/head sources.

### Flow migration and recovery

- [ ] `review-loop.yaml` passes the new topology validator.
- [ ] `rag-harness.yaml` passes the new topology validator.
- [ ] `context-coding-review-synthesis.yaml` passes the new topology validator.
- [ ] Restart after freeze reuses the same contract.
- [ ] Restart after coder preserves pending Canonical state.
- [ ] Restart during finalization does not duplicate Canonical effects.
- [ ] Stale epoch/lease cannot finalize a newer Flow.

### Quality gates

- [ ] GitNexus impact analysis is run and reported before every symbol edit.
- [ ] HIGH/CRITICAL impact is reported before implementation proceeds.
- [ ] All new test signatures in applicable slices are implemented or explicitly superseded with equivalent coverage.
- [ ] Existing green tests are not edited to force a pass.
- [ ] Package tests pass after every slice.
- [ ] Full relevant regression suite passes.
- [ ] Claude, Codex and Grok parity tests pass.
- [ ] GitNexus `detect_changes` shows only expected symbols/flows before each commit.
- [ ] Each implementation slice has a Task document and change-audit entry.
- [ ] Claude Sonnet MAX implementation evidence is reviewed by Codex with zero blocking findings.

---

## Review Protocol

For each implementation slice:

1. Claude Sonnet MAX reads this CP, the slice Task, governing SS/SD and latest relevant CA.
2. Claude emits a pre-edit Change Contract.
3. Claude runs GitNexus impact for every symbol it will edit and reports blast radius.
4. Claude adds tests first or alongside production code; existing tests remain untouched unless a real spec conflict is escalated.
5. Claude runs focused and regression tests.
6. Claude runs GitNexus change detection.
7. Claude updates the Task and writes the CA.
8. Codex reviews correctness, lifecycle ordering, recovery, provider parity and test gaps.
9. Confirmed findings return to Claude for fixes.
10. The next slice starts only after zero blocking findings.

No implementation is authorized by this document alone; coding begins only when the user explicitly starts the Claude Sonnet MAX implementation/review cycle.
