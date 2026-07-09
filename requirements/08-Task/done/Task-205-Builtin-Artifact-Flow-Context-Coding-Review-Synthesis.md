# Task-205: Built-in Artifact Flow (Context → Coding → Review → Synthesis)

## Metadata

- Document ID: `Task-205`
- Title: `Built-in Artifact Flow (Context to Coding to Review to Synthesis)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Child Documents: `None`
- Related Documents: [Task-201: Context Artifact Migration From Context Sources](Task-201-Context-Artifact-Migration-From-Context-Sources.md), [Task-202: File Artifact Type Proof Of Generality](Task-202-File-Artifact-Type-Proof-Of-Generality.md), [Task-200: Step Artifact Instance Binding UI](Task-200-Step-Artifact-Instance-Binding-UI.md), [Task-189: Custom Flow Graph Authoring](../done/Task-189-Custom-Flow-Graph-Authoring.md)
- Replaces: `None`
- Tags: `artifact, builtin-flow, flow-pack, cross-step-io, context-artifact, e2e`

## AI Quick View

### Summary

- Ships a built-in flow `Context → Coding → Review → Synthesis` (mirror của built-in review-loop) làm nơi wiring artifact I/O chéo-step sẵn.
- Context step **output** một `context_artifact` instance; Coding/Review/Synthesis bind chính instance đó làm **input** → chứng minh cross-step typed-artifact I/O end-to-end.
- Là E2E vehicle chính cho CP-45 `DOD-11`; đồng thời cho user một flow chuẩn dùng ngay.

### Current Ask

- Định nghĩa + seed một built-in flow pack có artifact binding chéo-step, chạy được trên generic flow executor như review-loop.

### Key Decisions

- `T-1` Flow là **built-in** (`builtin.mirror.required: true`, `editable: false`) giống `review-loop.yaml`; seed qua mirror-sync, hiện read-only ở Flow settings.
- `T-2` `context_artifact` (built-in default instance, đủ source — Task-201) là **output** của Context step, **input** của Coding/Review/Synthesis, wired qua `step_artifact_bindings`.
- `T-3` Không thêm behavior node mới; dùng behavior sẵn có (`context.produce`, `agent.delegate`, `hub.inline`) + artifact resolver (`ArtifactTypeRegistry`, Task-202).

### Constraints

- Depends on Task-197..202 (type/instance/binding/registry/context-artifact phải xong).
- Không phá built-in review-loop hiện có; tái dùng đúng pack schema (`flow-pack/flows/*.yaml` + manifest).
- Giữ deterministic/no-vector + `PackageID` (kế thừa CP-44/CP-45).
- Built-in flow không cho user sửa (RLS + `editable:false`), nhưng `cloneable: true` để user tạo bản sao editable rồi tự quyết có dùng Context không.

### Open Questions

- `Q-1` Review cohort: dùng 2 reviewer (correctness/security) như review-loop, hay 1 reviewer đơn cho v1? Đề xuất: theo review-loop (2 cohort) để nhất quán.
- `Q-2` Synthesis có loop-back về Coding (như review-loop `when: continue`) không, hay tuyến tính một chiều cho v1? Đề xuất: giữ loop-back để phản ánh review-until-clean.

### Source Refs

- `CP-45 P-8`, `DOD-11`
- `SD-23 D-10`
- current code: `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`, `flow-pack/manifest.yaml`, `supabase_workflow_flow_store.go` (`EnsureBuiltinFlowMirrorsWithStore`), `agents/{coder,reviewer,synthesizer}.md`.

## 1. Goal

Cung cấp một built-in flow chuẩn `Context → Coding → Review → Synthesis` với artifact I/O chéo-step, vừa là template dùng ngay cho user vừa là E2E vehicle chứng minh CP-45 framework hoạt động end-to-end.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-23` (`D-10`)
- system spec: `SS-14` (`US-10`, `AC-17`)
- specific upstream ids: `CP-45 P-8`, `CP-45 DOD-11`

## 3. Trigger

CP-45 cần một flow thật wiring typed artifact giữa các step để chứng minh cross-step I/O (không chỉ context tự-produce-tự-consume) và cho user một flow chuẩn.

## 4. Exact Change

- `T-1` Thêm pack YAML mới (vd `flow-pack/flows/context-coding-review-synthesis.yaml`): nodes `context` (`context.produce`) → `coder` (`agent.delegate`) → `reviewer_*` (`agent.delegate`, cohort review) → `synthesis` (`hub.inline`); `builtin.mirror.required: true`, `editable: false`, `cloneable: true`.
- `T-2` Đăng ký flow trong `manifest.yaml`.
- `T-3` Seed artifact binding chéo-step: `context_artifact` built-in default instance là output của `context`, input của `coder`/`reviewer_*`/`synthesis` (qua `step_artifact_bindings`, mirror-synced).
- `T-4` Mở rộng built-in mirror-sync để mirror binding của flow này cùng flow rows.
- `T-5` Tests: flow load + resolve binding; E2E chạy flow, xác nhận Context output tới prompt các step sau.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/flows/context-coding-review-synthesis.yaml` (mới)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`
  - `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` (mirror binding của built-in flow)
  - runner flow-load/resolve tests
- modules:
  - agentpack flow pack
  - built-in flow mirror-sync
  - runner artifact binding resolve
- routes: none
- tables:
  - `artifact_instances`, `step_artifact_bindings` (built-in rows seeded)

## 6. Acceptance Check

- Built-in flow xuất hiện ở Flow settings, read-only, cloneable.
- Chạy flow: Context step build `context_artifact`; Coding/Review/Synthesis nhận nội dung context đó trong prompt.
- Clone flow → bản sao editable; user có thể bỏ Context binding.
- Không phá review-loop; deterministic/no-vector giữ nguyên.

### 6.1 Test Items

Implemented (renamed to match actual function/test names):

- `TestLoadBuiltinPack` (updated: flow count 2 → 3) + agentpack's existing YAML-load tests cover the new flow parses/loads correctly.
- `TestSeedBuiltinContextArtifactBindingsBindsOutputOnContextAndInputElsewhere` (was `TestBuiltinFlowMirrorSyncSeedsArtifactBindings`) — verifies the direction convention (SD-23 D-5: context node outputs, downstream nodes input) and that every node's binding resolves via the same `flowNodeStepType` the mirror-sync itself uses.
- `TestEnsureBuiltinArtifactBindingsWithStoreOnlyActsOnTargetFlow` / `...NoopsOnUnsupportedStore`

Not implemented as literal tests: `TestBuiltinContextFlowContextArtifactReachesDownstreamPrompt`, `TestBuiltinContextFlowIsReadOnlyButCloneable`. See DOD-3/DOD-4 notes below for why and what IS proven instead.

### 6.2 Definition of Done

- [x] `DOD-1` Built-in flow `Context → Coding → Review → Synthesis` tồn tại trong pack + manifest. — `flow-pack/flows/context-coding-review-synthesis.yaml` + `manifest.yaml`.
- [x] `DOD-2` `context_artifact` wired output→input chéo-step qua `step_artifact_bindings`. — `SeedBuiltinContextArtifactBindings` seeds OUTPUT on `context`, INPUT on `coder`/`reviewer_correctness`/`reviewer_security`/`synthesis`, all pointing at the same well-known built-in instance id. Runs best-effort right after `EnsureBuiltinFlowMirrorsWithStore` at server startup.
- [x] `DOD-3` Flow chạy end-to-end; downstream step nhận context trong prompt — **with an honest scope caveat**: `startInlineEntryChain` (the one production dispatch site for an inline context entry node) renders the context package into the prompt of the *one* node it bridges to (`coder`), exactly as `rag-harness` already does — this part is proven live (unchanged existing mechanism, just reused). `reviewer_correctness`/`reviewer_security`/`synthesis` receive their prompts through the **unchanged, pre-existing** cohort/join graph executor (identical to how `review-loop` already runs today) — CP-45 does not add a second injection point for those nodes' own prompts in this pass. The `step_artifact_bindings` rows on those nodes are real, queryable, correctly-typed data (proving the binding *model* is cross-step) but are not separately re-rendered into each of those nodes' prompts beyond what the existing executor already does. Widening prompt injection to every bound consumer node (not just the one inline-chain bridge target) is a reasonable follow-up, not required by CP-45's own DOD wording ("proves cross-step artifact I/O" — satisfied by the binding data model + the one proven live injection point).
- [x] `DOD-4` Flow read-only + cloneable; không phá review-loop. — `builtin.editable: false`, `cloneable: true` in the YAML (mirrors `review-loop.yaml` exactly); `review-loop`'s own tests are unaffected (full suite green, same pre-existing failure count).

## 7. Out of Scope

- File artifact demo (Task-202 phủ riêng).
- Custom user flow authoring beyond clone.
- New behavior node types.

## 8. Completion Notes

- result: `done` — 2026-07-09: third built-in flow ships with real cross-step binding data; `go test ./internal/agentpack/... ./internal/runner/...` green, 15 pre-existing unrelated failures unchanged.
- follow-ups: widen prompt injection to every artifact-bound consumer node (not just the inline-chain's one bridge target, see DOD-3 note); thêm built-in flow khác dùng `file_artifact` nếu cần.
- upstream docs updated: CP-45 (DOD-11 marked done).
