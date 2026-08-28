# Task-243: Review Và Capture Các Context Artifact Source

## Metadata

- Document ID: `Task-243`
- Title: `Context Artifact Sources Review And Capture`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15` (đổi số từ Task-239 → Task-243 do trùng; owner chốt đủ Q-1..Q-4; plan thực thi = [CP-50](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md))
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-5), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Child Documents: `None`
- Related Documents: [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [CP-05-06: Jira MCP](../../07-Coding-Plan/done/CP-05-06-Jira-MCP.md), [CP-05-04: Firebase MCP](../../07-Coding-Plan/todo/CP-05-04-Firebase-Mcp.md), [Task-188: Canonical-Head Packing And Admin](../todo/Task-188-Canonical-Head-Packing-And-Admin.md), [Task-192: Migrate Built-in Context Sources](./Task-192-Migrate-Builtin-Context-Sources.md), [Task-194: Per-Flow Context Source Binding](./Task-194-Per-Flow-Context-Source-Binding.md), [Task-195: MCP-Backed Context Source Adapter](./Task-195-MCP-Backed-Context-Source-Adapter.md), [Task-204: Wire Production MCP Driver Adapter](./Task-204-Wire-Production-MCP-Driver-Adapter.md), [Task-229: Jira Issue Context Source And Runtime Target Picker](./Task-229-Jira-Issue-Context-Source-And-Runtime-Target-Picker.md), [Task-231: Firebase Crashlytics Context Source And Runtime Target](./Task-231-Firebase-Crashlytics-Context-Source-And-Runtime-Target.md), [BUG-268](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md), [BUG-269](../../09-BugFix/done/BUG-269-CP45-Artifact-Bound-Context-Sources-Bypass-Unknown-Source-Validation.md), [BUG-270](../../09-BugFix/done/BUG-270-Flow-Ref-Resolution-Errors-Silently-Fall-Back-To-Normal-Chat.md)
- Replaces: `None`
- Tags: `context-source, context-artifact, registry, canonical-head, review, capture, local-runner`

## AI Quick View

### Summary

- Doc review + capture: bản đồ duy nhất về mọi context source của `context_artifact` đang đăng ký trên `ContextSourceRegistry` (hiện có 7), luật precedence/validation/degrade của chúng, và cách Canonical Head (CP-43) thực sự đi vào package.
- Ghi nhận **live-run pivot**: `mcp.driver` / `jira.issue` / `jira.sprint` / `firebase.crashlytics` được đăng ký + chọn được trên UI nhưng **bị filter khỏi `Collect` lúc Plan-time**, thay bằng runtime-target prompt note (mẫu CA-268) — registry entry của chúng chỉ phục vụ validation/UI selection, không tạo section content lúc chạy thật.
- Ghi 8 phát hiện (`G-1`..`G-8`): 2 điểm doc-lệch-code (không có source id `canonical.head`/`change.contract`; Chat-mode prompt không bao giờ mang Head), 1 source "chết lâm sàng" (`source.excerpt` không có producer cấp hints lúc runtime), và 5 mục minor/đã biết.
- Task này không sửa code — mọi finding là ứng viên follow-up; owner đã chốt `Q-1`/`Q-2`/`Q-3` (2026-07-15), `Q-4` đang thảo luận.

### Current Ask

- Capture feature thành tài liệu chuẩn, verify hành vi hiện tại bằng test suite có sẵn, nêu gaps/bugs + câu hỏi mở để owner quyết lịch xử lý.

### Key Decisions

- `T-1` Đây là slice **review/capture**: không sửa production code; mọi gap được ghi tại đây (và tại CP-43 §10 nếu đã biết trước), không âm thầm fix.
- `T-2` Findings xếp hạng theo doc-contract trước: lệch giữa charter CP-44 và code đã ship vẫn là finding kể cả khi bản thân code nhất quán nội bộ và có test.

### Constraints

- Không tự ý resolve `G-1`/`G-2` — cả hai đổi intent ở tầng plan (CP-44 P-6/DOD-7, CP-43 P-5), cần owner quyết trước khi sửa code hay sửa CP. *(Đã quyết: xem `Q-1`/`Q-2`.)*
- Việc implement các follow-up phải là Task/BUG doc mới, không scope-creep vào file này.

### Open Questions

- `Q-1` **(RESOLVED → NÂNG, 2026-07-15, owner)** Nâng `canonical.head` thành source đăng ký thật (first-class id trên registry) **và đưa vào default builtin context set** (cùng nhóm `feature.history`/`chat.summary`/`source.excerpt`). Đồng thời sửa wording CP-43 §4.5/Task-188 cho khớp (Head tách khỏi prepend trong `feature.history`). `change.contract` chờ `Q-4`.
- `Q-2` **(RESOLVED → CÓ, 2026-07-15, owner)** Chat-mode per-turn injection (`composeFeatureBlocks`) cũng phải dẫn đầu bằng Canonical Head, không chỉ Flow Mode.
- `Q-3` **(RESOLVED → LÀM, 2026-07-15, owner)** Không bỏ `source.excerpt` — wire producer thật cho `ChangedPaths`/`ExplicitSourcePaths` ở live flow path (ví dụ derive từ git diff / file được nêu trong prompt).
- `Q-4` **(RESOLVED → CÓ, 2026-07-15, owner)** Thêm source `change.contract` để prompt của các step sau (Coding/validate) mang theo bản khai phạm vi của step hiện tại (nhắc AI giữ scope *trước* khi gate phải cảnh báo *sau*). Trước đây Contract chỉ được gate hook đọc post-turn + desktop panel đọc qua HTTP, chưa từng vào prompt. Lưu ý timing khi implement: Contract được capture *sau* turn code đầu tiên, nên section này rỗng ở turn đầu và có giá trị nhất cho validate/audit/reprompt.

### Source Refs

- `CP-44` P-1..P-11, DOD-1..DOD-9, §11. `CP-43` P-3/P-5, §3, §4.5, §11. `SD-22` D-2/D-3/D-4/D-5/D-7. `SD-23` D-6/F-1. `SD-21` D-3.

## 1. Goal

Một bản capture có thẩm quyền về kiến trúc context_artifact source như đã ship — mọi source đăng ký, wiring, validation, degrade path — cộng danh sách đã-verify các điểm lệch giữa governing docs (CP-43/CP-44/SD-22) và code, để follow-up được lên lịch từ facts thay vì phải điều tra lại.

## 2. Parent Links

- coding plan: `CP-44` (registry substrate), `CP-43` P-5 (Canonical Head packing)
- tech design: `SD-22` (registry), `SD-21` D-3 (Head-first packing), `SD-23` D-6 (artifact-bound source selection)
- system spec: `SS-14` AC-3/AC-8 (qua CP-43), `SS-13` (document contract)
- specific upstream ids: `CP-44 P-6`, `CP-44 DOD-7`, `CP-43 P-5`, `SD-23 D-6`

## 3. Trigger

Yêu cầu của owner (2026-07-15): Canonical Head là một source của `context_artifact`; capture feature này và toàn bộ các context source khác, review code + docs tìm gaps và bugs. Kiến trúc này trải trên 4 CP (CP-41/43/44/45) + 3 CP MCP mà chưa có tài liệu capture tập trung nào.

## 4. Exact Change

- `T-1` **Capture kiến trúc** (review, không sửa code) — trạng thái đã ship tính đến 2026-07-15:
  - **Registry** (`internal/runner/context_source_registry.go`): `Register` từ chối nil/id-rỗng/trùng-id/non-deterministic (bất biến no-vector của CP-41 được ép ngay ở tầng type); `Resolve` fail-fast với id lạ; `Collect` degrade mọi lỗi per-source thành warning trong package và sort section theo `Priority` rồi `SourceType` (CP-44 P-8).
  - **7 source đã đăng ký** (`context_sources_builtin.go`): `feature.history` (prio 2), `source.excerpt` (4), `chat.summary` (5) — default set; cộng opt-in `mcp.driver` (6, Google Drive, adapter production Task-204), `jira.issue` (7), `jira.sprint` (8) (Task-229, REST adapter), `firebase.crashlytics` (9) (Task-231, firebase-tools MCP client). Cả 4 source external dùng chung `mcpBoundedFetch` (timeout cứng 5s, cap 8KB, degrade-thành-warning).
  - **Precedence chọn enabled set** (`resolveEnabledContextSourceIDs`): (a) binding `context_artifact.v1` hướng OUTPUT với `config_json.sources` (CP-45/SD-23 D-6, cao nhất) → (b) `ContextSources` của node (Task-196) → default 3 nguồn built-in. Tier (c) cũ (match key `contexts:` trong YAML flow) đã retire.
  - **Live-run pivot (mẫu CA-268)**: trong `startInlineEntryChain`, 4 source external bị **filter khỏi `Collect`**, mỗi cái thành một runtime-target prompt note (`resolve*TargetForRun` + `append*TargetPrompt`) bảo provider CLI tự dùng MCP tools thật. Các ref trong `FlowContextHints` (`MCPDriverRef`/`JiraIssueRef`/…) **không được populate trên live path** — đường section-trong-package của các source này chỉ còn test/legacy seam dùng.
  - **Điểm vào của Canonical Head**: KHÔNG phải source đăng ký. `featureHistorySource.Fetch` prepend `changecontract.RenderHeadBlock(LoadHead(...))` vào body history (Task-188 T-1), chỉ khi `FeatureConfidence == verified`. Head thiếu/không đọc được → degrade thành không-có-block (AC-9).
  - **Validation**: `ValidateFlowContextSources` (id ở tầng flow + tầng node) và `ValidateFlowArtifactBindings` (resolve required-binding + check nội dung `config_json.sources` từ BUG-269) đều fail-fast lúc load flow; BUG-270 đã fix vụ lỗi bị nuốt rồi âm thầm rơi về Normal Chat.
  - **Render/persist**: 3 source built-in render theo tên (`### Change History` / `### Prior Discussion` / `### Source:`), source khác render generic (`### <sourceType>` + `_Source:_`); luôn có dòng `- **No vector retrieval used**`; package persist bằng `EventFlowContextPackage` (không thêm bảng).
  - **UI chọn source trên desktop** (`WorkflowsSettings.tsx` `contextSourceOptions`): danh sách descriptor 7 id sync tay; Go registry vẫn là validation authority.
- `T-2` **Findings** (kết luận review):
  - `G-1` **Không có source id `canonical.head` / `change.contract`** — CP-44 P-6/DOD-7 và CP-43 §11 nói CP-43 "cắm vào registry hệt như feature.history"; nhưng CP-43 §4.5/Task-188 lại ship Head dạng prepend bên trong `feature.history`. Hệ quả: flow giới hạn sources mà không có `feature.history` sẽ mất luôn Head; không thể lấy head-only context; UI không thể liệt kê; `change.contract` hoàn toàn chưa surface qua registry (câu CP-43 §3 chưa được implement). Docs mâu thuẫn nhau bất kể design nào thắng → cần `Q-1`/`Q-4`. **(Q-1 đã chốt: nâng thành source thật + vào default set.)**
  - `G-2` **Chat-mode prompt không bao giờ chứa Canonical Head** — per-turn injection (`injectFeatureHistoryPrompt` → `composeFeatureBlocks`, `feature_history.go`) load history + chat summary nhưng không load Head; chỉ Flow-mode Plan-step path có. Câu "resolved code turns lead with the Head" của CP-43 P-5 chỉ đúng cho Flow Mode → `Q-2`. **(Đã chốt: Chat mode cũng phải có.)**
  - `G-3` **`source.excerpt` chết lâm sàng trên live flow path** — không production caller nào populate `FlowContextHints.ChangedPaths`/`ExplicitSourcePaths` (`behaviorContextProduce` không set cả hai; producer duy nhất của `ChangedPaths:` là `flowgate.TurnResult`, struct khác không liên quan). Source default thứ 3 không đóng góp gì lúc runtime; toàn bộ logic an toàn của nó (workspace/symlink guard, byte cap) chỉ được test exercise → `Q-3`. **(Đã chốt: wire producer, không bỏ.)**
  - `G-4` **Warning bị suppress** — `featureHistorySource` chỉ bắn "no change history found" khi body sau khi compose rỗng; feature có Head nhưng ledger rỗng giờ không còn warning (trước CP-43 có). Semantic drift nhỏ, golden test không thấy (fixture không có Head).
  - `G-5` **Heading lồng sai cấp** — `RenderHeadBlock` xuất `## Canonical state of …` nằm *bên trong* section `### Change History` của package render (H2 lồng dưới H3). Template CP-43 §4.5 để chúng ngang hàng. Cosmetic.
  - `G-6` **`mcpBoundedFetch` cắt byte có thể xé đôi ký tự UTF-8** tại biên 8KB (byte rác cuối chuỗi trong prompt/JSON). Minor.
  - `G-7` **Các mục open đã track sẵn ở upstream** (không mới): Task-188 — chưa có demote/drop-log raw history theo budget, chưa live E2E Claude/Codex xác nhận Head-first; Task-185 — chưa có symbol-level false-drift suppression, chưa có `SS-14 E-4` ignore set.
  - `G-8` **Path separator lẫn lộn trong `SourceRef` trên Windows** (`filepath.Join(...)` + `"/ledger/…"`). Cosmetic.
- `T-3` Chính file Task này (bản capture) — viết theo `FORMAT-REFERENCE-TASK.md`.

## 5. Touched Areas

- files: `requirements/08-Task/done/Task-243-Context-Artifact-Sources-Review-And-Capture.md` (mới; file duy nhất thay đổi — đổi số từ 239 do trùng với `todo/Task-239-Flow-Restore-And-Step-Transition-Log.md`)
- modules: chỉ review — `internal/runner` (context_source_registry, context_sources_builtin, context_source_{mcp,jira,firebase}, flow_context_package, flow_executor, behavior_registry_builtin, artifact_type_registry, feature_history), `internal/changecontract` (head, pack), `apps/desktop-flowpilot` (danh sách source trong WorkflowsSettings)
- routes: không đổi
- tables: không

## 6. Acceptance Check

- [x] Mọi context source đăng ký được liệt kê đủ: id, priority, default/opt-in, adapter backing, hành vi live-path (`T-1`).
- [x] Điểm vào thực tế của Canonical Head vào package được ghi rõ và đối chiếu với text CP-43/CP-44 (`G-1`).
- [x] Mỗi finding nêu đúng file/function và điều khoản governing doc mà nó lệch.
- [x] Verification chạy trong turn này: `go test ./internal/changecontract/...` — 61 passed; `go test ./internal/runner/ -run 'ContextSource|FlowContextPackage|CanonicalHead|RenderFlowContextPackage|McpDriver|JiraIssue|JiraSprint|FirebaseCrash|FeatureHistorySource|ValidateFlowArtifactBindings' -count=1` — 83 passed. Không regression; findings là gap/lệch-doc, không phải test fail.
- [x] Câu hỏi mở (`Q-1`..`Q-4`) đã đưa owner thảo luận; `Q-1`/`Q-2`/`Q-3` đã chốt 2026-07-15.

## 7. Out of Scope

- Implement bất kỳ mục nào trong `G-1`..`G-8` (mỗi mục cần Task/BUG doc riêng).
- Sửa wording CP-43/CP-44/SD-22 — làm cùng Task follow-up của `Q-1` (và `Q-4` khi chốt).
- Phần residual của Task-185/Task-188 đã track ở CP-43 §10.

## 8. Completion Notes

- result: **done** (2026-07-15). Capture review-only; không sửa production code; 144 test liên quan pass xác nhận hành vi được ghi ở đây đúng là hành vi đã ship.
- follow-ups (theo quyết định của owner 2026-07-15, cả 4 câu hỏi đã chốt): toàn bộ 4 hạng mục (`F-1` canonical.head first-class + default set; `F-2` Chat-mode Head-first; `F-3` producer cho source.excerpt; `F-4` source change.contract) được lên kế hoạch chi tiết tại [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md) — coding guide từng bước + DOD per-phase nằm ở đó. Micro-fix `G-4`/`G-5`/`G-8` được hấp thụ vào CP-50 `P-1`; `G-6` vào CP-50 §4.5.
- **Update (2026-07-16):** `F-1`..`F-4` đã landed qua CP-50 `P-1`..`P-4` / [Task-244](../../08-Task/done/Task-244-Canonical-Head-First-Class-Context-Source.md), [Task-245](../../08-Task/done/Task-245-Chat-Mode-Canonical-Head-First-Injection.md), [Task-246](../../08-Task/done/Task-246-Source-Excerpt-Runtime-Hint-Producers.md), [Task-247](../../08-Task/done/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md) (tất cả `done`). `F-4` cụ thể: `change.contract` context source (priority 3) + append vào prompt node sau qua `composeFlowNodeAgentPrompt` — xem CP-43 §3 note tương ứng.
- upstream docs updated: CP-43 §3 đã thêm note tham chiếu CP-50 P-4/Task-247 (2026-07-16); CP-44 chưa cần sửa thêm (không có claim nào của CP-44 bị ảnh hưởng bởi CP-50).
