# 1.CP 43

## 1.1 Change contract

### Question

- Do Code hay do Ai -> Do AI khai báo DƯỚI CONTROL của RUNNER

### Detail

#### Chat mode - LEGACY. KHAI SAU KHI ĐÃ CODE. Không ok. Dùng cho mấy fix đơn giản, hỏi đáp đơn giản thôi

- Chúng ta có 1 internal skill là context-discipline. skill này nói AI phải khai báo BLOCK change contract.
- Internal skill sẽ đc inject vào skill của các provider

TUY NHIÊN, đây vẫn là 1 cơ chế mềm, AI có thể bỏ qua

Runner sẽ làm thêm gate: r-contract

- r-contract sẽ tìm block trong câu trả lời của Ai -> nếu khồng có thì force re-prompt đúng mẫu: enforce.go:151 -> Lượt sau AI bắt buộc phải in block.

#### Flow mode - Khai Contract trước = 1 step riêng sau đó mới code

- Dùng hẳn 1 Step mới tên là: Planner(AI) -> sẽ đc khai báo là step 1 của FLOW
- agents/contract-planner.md là 1 internal agent -> chạy trước khi coder có chạm code. Toàn bộ khác hẳn cơ chế [Change Contract] của chat mode

preflight*contract_plan → preflight_contract_freeze → coder → reviewer*\* → synthesis
(AI: contract-planner) (Go inline, contract.freeze)

1. Planner chạy như thế nào

- Agent contract-planner.md: "read-only — never edits, writes, or runs commands", chỉ có tools: [Read, Grep, Glob], không có Edit/Write/Bash.
- Nhiệm vụ: đọc issue + chỉ đọc vừa đủ codebase để liệt kê các file cụ thể sẽ bị đụng — "exists to bound scope, not to investigate".
- Output là đúng 1 object JSON, không gì khác — cấm prose, cấm fence, cấm giải thích:
  {"feature_key":"calc-core","intent":"fix rounding","declared_paths":["src/calc.go"],"source_doc_id":"BUG-123"}

2. Ràng buộc read-only — code kiểm chứng, không tin lời AI
   Sau turn planner, runner fingerprint lại worktree và so với fingerprint chụp lúc flow start (trước khi planner chạy — flowStartWorktreeFingerprint): nếu planner đụng bất kỳ file nào → escalate "the contract planner must be read-only" (flow_validate_audit_dispatch.go:1345-1350). Không phải AI tự hứa, là code diff thật

1. Go validate + freeze (node contract.freeze, inline, không phải AI)
1. 1. ParsePreflightDraft — strict JSON.
1. 2. ValidatePreflightDraft (preflight.go:112) — feature_key có, intent non-blank, ≥1 declared path IsConcreteCodeTarget (không dir/glob/doc). Cố ý chưa allowlist feature_key theo catalog (P-3) — sẽ thêm sau.
1. 3. FreezeContract (preflight.go:193) — NormalizeDeclaredCodePaths (escape check, dedup, sort) → mint FrozenContractRecord bất biến, versioned; ContractID = sha256(runID + coderStepID + version + draft + baseSHA + baseline).
1. 4. FrozenStore.SaveFrozen — append + fsync vào .flowpilot/contracts/frozen_contracts.ndjson, strict reload (dòng hỏng = fail mở, không "dung thứ" như legacy Store), rồi reload lại từ đĩa để verify durability trước khi đi tiếp (dòng 1423-1429).

1. Sau freeze — coder bị "khóa" theo contract

- advanceFlowThroughFreezeChain (flow_validate_audit_dispatch.go:1463) đi qua các hop context.produce trung gian rồi spawn coder.
- Coder không bị ràng bởi ScopeDiff/r-scope legacy nữa — bị ràng bởi FrozenContractScopeDrift (frozen_scope.go:100): so WrittenPaths với rec.DeclaredPaths → path ngoài phạm vi = drift fail-closed.
- Cần mở rộng → AmendFrozenContract (frozen_scope.go:161): version+1, Supersedes trỏ contract cũ, union paths, chỉ thực sự tạo version mới khi union có thêm đường mới. Status: frozen → accepted | superseded | abandoned.
- Loại trừ đúng 2 file bookkeeping .flowpilot/contracts/frozen_contracts\*.ndjson khỏi drift so sánh — không loại trừ cả .flowpilot/\*\* như lỗ hổng cũ CA-427 Finding 2 (kẻ đó có thể sửa flow-rules.json ẩn mình).

#### Q: Tức là step validate này chỉ check 1 việc là Planner không được change code?

Không — node contract.freeze (Go inline) làm 4 việc (flow_validate_audit_dispatch.go:1352-1429):

1. Read-only check — diff fingerprint worktree trước/sau turn planner (dòng 1345-1350).
2. Strict parse — ParsePreflightDraft (JSON chặt, không prose/fence).
3. Structural validation — ValidatePreflightDraft (preflight.go:112): feature_key non-blank, intent non-blank, ≥1 declared path là IsConcreteCodeTarget (không dir/glob/doc).
4. Freeze + persist — normalize paths, mint ContractID sha256, SaveFrozen (append+fsync), rồi reload lại từ đĩa verify durability trước khi đi tiếp.
   Nhưng không check tính đúng-đắn ngữ nghĩa: paths có thực sự khớp issue không, intent có đúng hướng không — và feature_key cố ý không allowlist theo catalog (comment dòng 1356-1359, thêm sau được).

#### Q: Change contract được tạo chủ quan bởi AI, nó đưa ra các declared_paths? Nhưng nếu contract này sai thì sao? Coder cứ theo cái sai này à?

Đúng — declared_paths là phán đoán chủ quan của AI, không được kiểm ngữ nghĩa. Nhưng có 2 lớp chặn, coder không tự do đi theo cái sai một cách yên lặng:

1. Sai kiểu "thiếu file" (scope quá hẹp) → không phải coder mù quáng theo:

- Coder đụng file ngoài declared_paths → FrozenContractScopeDrift (gate_hook.go:778) → BLOCK fail-closed + escalate (dòng 783-800): flow dừng, hiện rõ danh sách path drift.
- Coder không thể tự thoát bằng cách khai bổ sung sau khi viết — test flow_frozen_scope_gate_test.go:212 assert đúng điều đó: "a coder cannot satisfy Flow preflight by declaring the drifted path after the fact".
- Lối thoát đúng là AmendFrozenContract (frozen_scope.go:161): version+1, Supersedes contract cũ, union paths — là một hành động riêng ở mức flow, không phải coder tự ý.

2. Sai kiểu "sai hướng" (intent/feature sai) → bị reviewer chặn ở tầng dưới:

- Coder theo intent sai thì sinh diff sai, nhưng sau coder có 2 reviewer (reviewer_correctness, reviewer_security) review diff so với issue gốc → findings → synthesis quyết định done/continue/escalate (review-loop.yaml:74-105). Flow không kết thúc accepted trên rác: muốn qua cần reviewer duyệt + synthesis done. Contract sai → findings → loop lại coder hoặc escalate.
- Tóm lại: contract là "bộ giới hạn scope", mục đích để bound, không phải kế hoạch hoàn hảo. Hẹp ban đầu không bị phạt (contract-planner.md:30, "a narrow initial scope is not penalized"). Điểm mạnh nằm ở chỗ: sai kiểu thiếu file thì chặn hẳn, sai kiểu sai hướng thì reviewer bắt bài — không có kịch bản "lặng lẽ theo cái sai tới accepted"

#### Q: UI có cho tôi chọn các step này không?

Có

#### Q: 2 Step này có được auto apply không ,nếu tôi tạo flow và bỏ qua nó thì sao?

- Auto apply — có. Plan và freeze là node trong YAML flow (review-loop.yaml:30-43), engine chạy như mọi node khác. Không phải việc thủ công.
- Flow mới có agent.code writer mà không có freeze → hard-block, flow không chạy được. gate_hook.go:693 chặn vô điều kiện: "no frozen contract found for this coding step; a Flow writer requires a contract frozen before it runs". Đây là Flow guarantee hardcoded (không nằm trong configurable flowgate rules), + test an toàn topology bắt mọi builtin flow phải có preflight trước writer (flow_safety_topology_builtin_migration_test.go). Nghĩa là: flow có agent.code thì freeze step gần như bắt buộc.
- Ngoại lệ: flow chỉ dùng writer agent.delegate (legacy) → không cần freeze, frozenOK=false → bỏ qua enforcement, rơi về contract legacy declared/inferred (gate_hook.go:678-680 comment).
-

#### Q: Sao chat và flow mode không dùng chung 1 logic contract?

Thực ra hiện tại CHAT MODE nên để code kiểu đơn giản

Việc khai báo contract của nó là SAU KHI CODE ĐÃ CHANGE XONG.

Còn với FLOW MODE STRICTLY thì là khai báo trước rồi mới code. code xong verify có tuân theo không -> CÁi này chuẩn hơn. Như kiểu Viết test xong mới code vậy

## 1.2 Scope drift - Cho Flow mode

Scope drift (bản Flow/frozen) chỉ check đúng một thứ: "coder có ghi bất kỳ file nào nằm ngoài declared_paths của frozen contract không?" — so sánh thay đổi thực trên git với scope được phép. Không check nội dung diff, không check intent, không check style.

### Cách check — pipeline 5 bước

1. Lấy ground truth bằng git command, KHÔNG tin AI báo
2. Trích danh sách paths
3. Loại trừ các file không cần so sánh như những file ndjson. Những file doc md CŨNG BỊ GIỮ ĐỂ SO SÁNH -> vì có thể nó đổi luôn cả file CP hay SD SS của mình
4. So sánh Contract path và real path kiểu: normalizeScopePath: ToSlash + TrimSpace + path.Clean (normalize giống cách Mplan làm cho custom metric, Fuzzy)

### Q: Cái này chạy ở step nào? Khi nào?

Đó là post-turn gate của coder (writer) — không phải một node riêng trong flow:

- Chạy trong runChildArtifactOutputGateAtEpoch (gate_hook.go:477), được gọi ngay sau khi coder xong 1 turn (EventTurnCompleted → fin), trước khi flow chuyển sang bước tiếp theo (reviewer). 2 call site: interactive_service.go:3954 (turn bình thường) và :6431 (resume/redelivery turn khi recovery).
- Điều kiện để nhánh frozen-scope kích hoạt: có flow cha (parentID), coderStepID resolve được, và node là agent.code hoặc tồn tại frozen contract cho (parentID, coderStepID) (gate_hook.go:655-688). Node agent.code mà không có contract → block "requires a contract frozen before it runs".
- Chạy lại ở MỌI turn của coder — coder lifecycle: reinvoke, loop qua cạnh synthesis → continue → coder, nên mỗi gate pass re-check drift so với cùng baseline trong frozen record. Restart/reload cũng không thoát (CA-427 Finding 3: enforcement độc lập với topology trong bộ nhớ, dựa vào contract tồn tại trên đĩa)

TÓM LẠI: là code default của runner khi mà 1 step đc def là agent.code.

### Q: Chỉ có ở flow mode à? Vậy chat mode nếu code result sai contract thì sao?

Frozen scope drift (fail-closed block) là Flow-only — nó cần FrozenContractRecord, thứ chỉ tồn tại qua freeze step.

Chat mode không có cơ chế này; thay vào đó là contract legacy với chính sách dễ dãi hơn hẳn:

- Chat mode: prepareChangeContract (gate_hook.go:633) khai/infer contract SAU KHI CODE ĐÃ VIẾT (từ [Change Contract] trong final message hoặc InferFromDiff), ScopeDiff (scope.go:24) tính out-of-scope.
- Vi phạm → rule r-scope mặc định warn; và kể cả user cấu hình block cũng bị hạ xuống warn trừ khi HighSeverity (gate_hook.go:652-654, rules.go) — tức chỉ cảnh báo/reprompt, không dừng run. Thêm nữa: không khai contract thì r-contract chỉ reprompt nhắc khai; nếu không infer được gì → ScopeDiff không có gì để so → không hề báo drift (scope.go:15).
-

## 1.3 Cannonical - Save cả những phương án phủ định

### WHAT?

Canonical Head = record duy nhất, chuẩn mực ("single authoritative statement") về intent hiện tại của một feature (SD-21 §5). Một feature → một Head

- BehaviorStatement — câu mô tả hành vi hiện tại (AI viết qua contract intent).
- GoverningDocIDs + GoverningDocHashes — spec dẫn dắt (SS/SD/CP...).
- IntentSignature — sha256(governing docs + behavior), không bao giờ over code bytes → là chìa khóa phát hiện spec/code drift.
- Decisions — tri thức phủ định (Task-187, P-4).
- Status (spec_less/current/spec_drifted/code_drifted/renamed/merged/deprecated) + SpecConfidence + RetiredAt/SupersededBy.

Điểm mấu chốt: AI không tự do viết file này — code sinh nó (BuildHead/UpdateHead), AI chỉ cung cấp text thành phần (intent, chat_summary bullets) rồi code ghép deterministic.

### Q: Dùng khi nào / step nào?

GHI: ĐƯƠNG NHIÊN CHỈ GHI SAU KHI PASS ALL. Vì nếu code lỗi mà đã ghi là sai rồi
— post-turn gate pass, chỉ khi turn gate-passing (không drift/attach-pending):

- Chat mode: updateCanonicalHead (gate_hook.go:2157) — ghi ngay sau gate pass.
- Flow mode: coder gate pass → stagePendingCanonicalHead (gate_hook.go:2188) — chỉ stage vào PendingCanonicalStore, không đụng file thật; finalizePendingCanonicalHeadsForRun chỉ chạy ở terminal done (applyFlowControl "done"), 2-phase (stage hết → rename hết). Validation-fail / review continue / stop → abandon, không ghi.
- Mint: BuildHead khi feature chưa có Head (birth/backfill).

ĐỌC:

- Context source = canonical.head (context_source_canonical_head.go, Task-244/CP-50 P-1 & CP-43 P-5): chèn Head block vào prompt khi feature verified. - Còn nhiều type context src khác nữa
- Retrieval: feature_history.go:112 load Head lấy head_commit/decisions.
- Gate: SpecDrifted/CodeDrifted (drift.go) + rules r-spec-drift/r-code-drift/r-attach-spec/r-retire — đều warn (cần reconcile, không auto block).
- API (canonical_head_handlers.go): GET /canonical-head; POST /rebaseline (RebaselineWithSpec — user confirm, không auto); POST /retire (RetireHead — rename/merge/deprecate, không auto, ghi RetiredAt+SupersededBy, giữ file không xóa).

### Q: Apply cho cả Chat và Flow mode k?

Có

### Q: Canonical file — do code runner ghi, không phải AI

Chuỗi chính xác:
AI (đầu vào): khai [Change Contract] + sửa code + trả lời — KHÔNG đụng canonical
↓
Runner gate sau turn: commitChangeContract (gate_hook.go:2167)
↓
updateCanonicalHead (gate_hook.go:2289)
↓
changecontract.SaveHead (head.go:99) → ghi .flowpilot/canonical/<feature_key>.json (head.go:67)
Chat mode: gate pass → runner ghi Head ngay (với intent_signature tính từ contract)
Flow mode: coder gate pass → runner chỉ stage vào canonical-pending/pending_canonical.ndjson; khi flow terminal done → finalizePendingCanonicalHeadsForRun mới ghi file canonical thật (2-phase, đúng 1 lần — test TestFlowDoneFinalizesCanonicalHead)
AI chỉ cung cấp dữ liệu (intent/declared_paths qua contract); mọi persist đều là runner code

### Q: Lưu như nào?

- .flowpilot/canonical/<feature_key>.json — 1 file JSON mỗi feature, pretty (MarshalIndent). Drive-synced qua contextsync (Task-188) — khác contracts.ndjson chỉ local (head.go:93-95).
- Ghi atomic: StageHeadWrite (tmp .json.tmp) → CommitHeadWrite (Rename) → không lộ file nửa chừng (BUG-289 H3/F-3). LoadHead: file hỏng (torn/Drive conflict) coi như missing để rebuild, không hard-fail gate.
- PendingCanonicalStore (CP-55 P-5): .flowpilot/pending_canonical/ gồm 2 NDJSON (records + events), mỗi record mang HMAC-SHA256 signature (chống coder ghi fake NDJSON line rồi bị finalize tin — CA-427 Critical 1), strict reload.
- Mỗi lần gate-passing update: FoldDecisions → IntentSignature = ComputeSignature(updated) lại (gate_hook.go:2147).

### Ý là FLow có 1 step riêng ?

Chu kỳ ngắn: Gate pass → (chat: ghi ngay / flow: stage) → đọc lại làm context + drift check → human chỉnh qua rebaseline/retire.

Không — không có step riêng trong YAML. "Ghi ngay / stage" là 2 cách xử lý khác nhau tại cùng một thời điểm: post-turn gate pass của coder (chính cái runChildArtifactOutputGateAtEpoch ta nói trước đó).
Cả 2 đều đi qua cùng 1 hàm computeCanonicalHeadUpdate (gate_hook.go:2145) — tính ra updated (Head sẽ trông như thế nào) — nhưng khác ở chỗ đi tiếp:

- Chat: updateCanonicalHead → SaveHead ghi ngay file thật .flowpilot/canonical/<feature>.json trong chính turn đó.
- Flow: stagePendingCanonicalHead → không ghi file thật, mà stage giá trị updated vào PendingCanonicalStore (NDJSON) kèm HMAC. File thật chỉ bị đụng một lần duy nhất khi flow đạt terminal done, qua finalizePendingCanonicalHeadsForRun (2-phase, fail thì không "done")

# CP-54

## What?

Vai trò: trục INPUT — context feed
Việc chỉ cầm 1 feat-key chung chung trong khi bên trong có rất nhiều part cụ thể. mà khi pick commit lại chỉ lấy N mục mới nhất. Ví dụ cung là feat home nhưng đang cần fix domain nhưng 10 latest commit lại là home_UI thì vô nghĩa

-> update: 1 cơ chế chấm điểm xem feat định làm GẦN VỚI CÁI GÌ NHẤT
Chấm = 1 function

## Cách tìm history "tương đồng" — 2 tầng: locus + score/rank

### Tầng 1 — buildRetrievalLocus (retrieval_locus.go:62)

xác định "vùng code turn này đang đụng", hợp 4 nguồn theo độ tin cậy giảm dần:

- explicitPaths (ExplicitSourcePaths) — caller tự resolve (CP-55 P-8: trong Flow coder, đây chính là FrozenContractRecord.DeclaredPaths, vì legacy Store không bao giờ có contract cho agent.code writer).
- Change contract đã khai của run (legacy Store: DeclaredPaths + DeclaredSymbols).
- Uncommitted diff — cái đã thực sự bị sửa.
- Paths nhắc trong prompt (extractPromptSourcePaths).
- Lọc chỉ giữ IsConcreteCodeTarget, dedup, rồi GitNexusImpactTargets suy symbols, sort cho deterministic. Non-fatal: thiếu gì → locus rỗng → fallback recency.

### Tầng 2 — score + rank (relevance.go)

- ScoreHistoryEntry (relevance.go:122): với mỗi entry:
- PathOverlap = số path trong entry.ChangedPaths khớp chính xác với locus.Paths (cả 2 phía canonicalize, case-sensitive, không fuzzy/prefix).
- SymbolOverlap = số path khớp locus.Symbols trực tiếp hoặc qua derivedSymbolFromPath (basename → CamelCase).
- RankHistoryEntries (relevance.go:196): sort PathOverlap ↓ → SymbolOverlap ↓ → CommitUnix ↓ → CommitHash ↑ (tie-break hash → deterministic, BUG-266).
- SelectHistoryEntries (relevance.go:289): chỉ kích hoạt ranking khi candidates > 30 VÀ locus ≠ rỗng; cap 15; entry mới nhất ("newest = truth") luôn giữ, nếu không vào top thì chèn lên đầu (đẩy 1 entry thấp nhất ra). Ngược lại → fallback nguyên xi recency.

## Push gì vào Context step

featureHistorySource.Fetch (context_sources_builtin.go:178) → HistorySlotRanked (slots.go:121) render block:

- Recency: ## History "<key>" (newest = truth) — tối đa 15 entry, mỗi dòng - [<id> <date>] <summary> ← truth, 3 entry mới nhất kèm CA excerpt ("why"); ghi rõ bao nhiêu entry cũ bị bỏ.
- Ranked: ## History "<key>" (ranked, N/M; ← truth) — 15 top; current-truth luôn có excerpt dù rank thấp.

Block này là 1 section trong FlowContextPackage, cùng với canonical.head (priority 1), change.contract (3), source.excerpt (4), chat.summary (5) — sắp theo priority (flow_context_package.go:455-486) rồi render vào prompt của coder

## Chat vs Flow — context sources khác nhau

|                   | Chat mode                                                                                                        | Flow mode                                                                                                                                                                                                                         |
| :---------------- | :--------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Cơ chế**        | `flat prepend` – `injectFeatureHistoryPromptCtx` $\rightarrow$ `composeFeatureBlocks` (`feature_history.go:106`) | `FlowContextPackage` – các section `ContextSource`, priority-ordered                                                                                                                                                              |
| **Sources**       | chỉ **3**: Canonical Head + `HistorySlot` + `ChatSummarySlot`                                                    | **6 default** (`context_sources_builtin.go:28`): `canonical.head`, `feature.history`, `change.contract`, `dependence`, `chat.summary`, `source.excerpt` (+ flow có thể khai `contexts.sources` riêng / per-node `ContextSources`) |
| **History**       | `HistorySlot` – **recency thuần**, KHÔNG ranking                                                                 | `HistorySlotRanked` – **rank theo locus** (`FrozenContractRecord.DeclaredPaths` làm locus)                                                                                                                                        |
| **Locus**         | không dùng                                                                                                       | `buildRetrievalLocus` + symbols                                                                                                                                                                                                   |
| **Verified gate** | `resolveInjectionFeature` (prompt tự resolve / continuation thừa kế)                                             | `ConfidenceVerified` + `FeatureKey` mới load (`context_sources_builtin.go:180`)                                                                                                                                                   |

chat = 3 block dán thẳng, theo recency;
flow = gói section rank theo vùng code, và mỗi node/flow khai được nguồn riêng. Chính vụ ranking locus (CP-54) chỉ được nối dây ở flow context package (CP-55 P-7) — chat chưa dùng.

## Q: Nói buildRetrievalLocus dựa trên 4 src trong đó có explicitPaths?

Nhưng đây là khai báo context trước khi code? làm sao đã có real path

Vì có AI đọc code trước đó rồi — không phải đoán. Trong Flow, ExplicitSourcePaths = rec.DeclaredPaths (flow_validate_audit_dispatch.go:1496), và rec.DeclaredPaths là những file concrete thật do contract-planner khai từ bước trước coder:

- Planner chạy read-only với tools: [Read, Grep, Glob] (contract-planner.md:5) — nó đọc codebase thật để chỉ ra các file cụ thể sẽ bị đụng, ví dụ src/calc.go — đúng path file tồn tại trên đĩa (hoặc sẽ được tạo).
- Đến freeze, paths này được kiểm/normalize: escape check, symlink check, IsConcreteCodeTarget (phải có extension, không dir/glob) — không phải chuỗi bừa.
- Đây chính là case CP-55 P-8 (retrieval_locus.go:47-61): context đầu tiên của coder được build trước khi coder viết gì hoặc nói gì → 3 nguồn còn lại đều rỗng → thiếu explicitPaths thì locus rỗng → rank rớt về recency. explicitPaths lấp đúng lỗ hổng đó.
  Còn trong chat mode, ExplicitSourcePaths đến từ prompt (extractPromptSourcePaths, behavior_registry_builtin.go:112) + ChangedPaths — best-effort, có thể rỗng. Chỉ flow frozen mới có "real paths" chắc chắn trước khi code.

## Q: Nếu PLan step, AI nó khai sai contract + sai path thì sao ?

### Ảnh hưởng tới context step

Path khai sai → locus sai → history được rank theo vùng code sai → context cho coder gây hiểu lầm. Nhưng ranking không biết phát hiện "sai" — nó chỉ dùng locus được cung cấp; vấn đề này không nằm ở tầng rank, mà nằm ở chất lượng input planner.

### Ảnh hưởng kết quả flow

#### Sai kiểu "thiếu file" (scope hẹp hơn cần)

Đây là an toàn nhất: coder đụng file ngoài declared_paths → FrozenContractScopeDrift → BLOCK fail-closed + escalate, flow dừng, hiện rõ path drift. Không có kịch bản âm thầm. Lối thoát = AmendFrozenContract (version+1, supersede) — hành động riêng ở mức flow. Planner cũng được dạy "narrow initial scope is not penalized" (contract-planner.md:30)

#### Sai kiểu "thừa file" (khai cả file không nên đụng)

Đây là điểm yếu thật: file trong declared_paths thì gate drift không chặn — coder viết gì vào đó cũng hợp lệ. Lớp chặn duy nhất là reviewer (correctness/security) review diff so với issue gốc → findings → synthesis decide done/continue/escalate. Flow không đạt accepted trên rác, nhưng tốn vòng loop.

#### Sai kiểu "sai hướng" (intent/feature_key sai)

- Intent sai → coder prompt = rec.Intent → implement sai → reviewer bắt bài (so với issue), loop/escalate.
- Feature_key sai → đây là điểm yếu nhất: feature.history + canonical.head + chat.summary đều key theo hints.FeatureKey = rec.FeatureKey (context_sources_builtin.go:180) → context bị poisoned bằng history của feature KHÁC; Canonical Head staging cũng nhắm sai feature_key, finalize ở done sẽ ghi sai chỗ. Cố ý không allowlist feature_key theo catalog lúc freeze (flow_validate_audit_dispatch.go:1356-1359, "trusted for now") — đó là lỗ hổng thiết kế được ghi nhận, hẹn bổ sung sau.

## Q: Nói buildRetrievalLocus dựa trên 4 src trong đó có Change contract?

Tức là cùng 1 step Context trong Flow , luôn phải chạy get Change contract từ Ai trước rồi mới build đc rank phải không ?

Đúng thứ tự, nhưng sai cơ chế. Trong flow frozen writer, thứ tự là:
preflight_contract_plan (AI planner, đọc code, khai draft)
→ preflight_contract_freeze (Go validate + freeze → FrozenContractRecord)
→ context.produce (Context step: buildRetrievalLocus + rank)
→ coder

- Context step KHÔNG gọi AI "get contract" — nó chỉ đọc record đã frozen sẵn từ FrozenStore rồi đưa rec.DeclaredPaths làm explicitPaths. Contract "từ AI" đã xong ở bước planner trước đó, không chạy lại trong Context step.
- Lưu ý: contract này là FrozenContractRecord, KHÔNG phải legacy change.contract — agent.code writer không bao giờ gọi prepareChangeContract (CP-55 P-4), nên nguồn số 2 trong buildRetrievalLocus (legacy Store) vốn bỏ trống với nó.
  Nhưng "luôn phải có contract AI" là sai nếu hiểu là bắt buộc. buildRetrievalLocus non-fatal (retrieval_locus.go:40-42): locus có thể rỗng, và ranking chỉ kích hoạt khi candidates > 30 VÀ locus ≠ rỗng. Nghĩa là:
- Nguồn locus có thể là bất kỳ cái nào ≠ rỗng — uncommitted diff (nguồn 3) hoặc paths trong prompt (nguồn 4) cũng đủ để rank.
- Không có contract, không diff, không path → locus rỗng → fallback recency bình thường, không lỗi.
  Nên câu trả lời gọn: flow frozen thì "contract từ AI (planner)" có mặt trước Context step và là nguồn locus chính đáng tin cậy nhất, nhưng không phải điều kiện tiên quyết — chỉ cần locus ≠ rỗng từ bất kỳ nguồn nào là rank chạy được; rỗng thì chịu, về recency.

# CP-55

CP-55 không thay CP-43/CP-54; nó **đóng timing Flow của 43** và **ship ranking path-only của 54**.

CP-43 đã có contract + drift + Canonical Head,
CP-54 đã có locus builder

nhưng thời điểm trong Flow sai:

- contract có thể capture muộn
- Canonical Head mutate ngay mỗi gate pass (Flow fail giữa chừng vẫn “ăn” Head). Vì flow failed hoặc k pass review thì RESULT ĐÓ không nên là 1 approve ver của feat
