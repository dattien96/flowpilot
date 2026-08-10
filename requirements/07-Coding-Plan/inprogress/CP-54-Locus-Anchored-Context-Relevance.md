# CP-54: Locus-Anchored Relevance Retrieval For Feature History And Chat Context

## Metadata

- Document ID: `CP-54`
- Title: `Locus-Anchored Relevance Retrieval For Feature History And Chat Context`
- Feature Keys: `context-regression-engine, change-contract`
- Phase: `coding_plan`
- Status: `approved` (2026-07-27 — owner duyệt chẩn đoán "sai đơn vị truy hồi" + hướng locus-anchored; cắt Task con bắt đầu từ P-1)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-24`
- Last Updated: `2026-08-11` (code done P-1→P-6; verification = [CP-54-Test-Steps](./CP-54-Test-Steps.md))
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) (D-3 ordered-history, D-4 no-vector), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-7, AC-8)
- Child Documents: [Task-261: Persist ChangedPaths In Change Ledger](../../08-Task/done/Task-261-Persist-Changed-Paths-In-Change-Ledger.md) (P-1, enabler — **done** 2026-07-27, [CA-422](../../../change-audit/CA-422-persist-changed-paths-in-change-ledger.md)), [Task-262: Shared Retrieval-Locus Builder](../../08-Task/done/Task-262-Shared-Retrieval-Locus-Builder.md) (P-2 — **done** 2026-07-27, [CA-423](../../../change-audit/CA-423-shared-retrieval-locus-builder.md)). **P-3→P-5 (deterministic scorer, selection/ranking, wiring vào `feature.history`) đã KHÔNG cắt tiếp dưới số CP-54 riêng — cùng phạm vi đó được cắt và triển khai dưới [CP-55](./CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) P-6/P-7 thay vì P-3-P-6 gốc ở đây** (`ScoreHistoryEntry`/`RankHistoryEntries`/`SelectHistoryEntries`/`HistorySlotRanked`, xem [Task-268](../../08-Task/done/Task-268-Deterministic-History-Relevance-Scorer.md)/[CA-429](../../../change-audit/CA-429-deterministic-history-relevance-scorer.md), [Task-269](../../08-Task/done/Task-269-Wire-Ranking-Into-Feature-History.md)/[CA-430](../../../change-audit/CA-430-wire-ranking-into-feature-history.md)) — `buildRetrievalLocus` (P-2, this doc) chỉ có production caller đầu tiên qua đúng công việc đó. CP-55 P-8 sau đó là lần đầu `buildRetrievalLocus`/ranking thực sự chạy trên một Flow production thật (trước đó chỉ unwired/unit-tested) — xem [Task-270](../../08-Task/done/Task-270-Migrate-Built-In-Flows-And-E2E-Recovery-Parity-Coverage.md)/[CA-431](../../../change-audit/CA-431-migrate-built-in-flows-and-e2e-recovery-parity-coverage.md). **P-6 (symbol tier, phụ thuộc Task-259) vẫn chưa cắt** — Task-259 tự nó chưa ready (xem ghi chú Q-6 dưới).
- Ghi chú kiến trúc (từ P-2): kiểu `RetrievalLocus` nằm ở **`internal/featurecatalog`** (không phải `internal/runner` như §7 phác) vì chiều import là `runner → featurecatalog → changeledger`; scorer P-3 ở `featurecatalog` sẽ không compile nếu kiểu nằm trong `runner`. Builder `buildRetrievalLocus` + normalizer `isConcreteCodeTarget` ở `runner`.
- Related Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, And Canonical Acceptance](./CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (**cắt và triển khai P-3→P-5 của CP này dưới số CP-55 P-6/P-7/P-8** — xem ghi chú Child Documents ở trên), [CP-43: Change Contract And Canonical Intent Signature](../inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (**quan hệ gần nhất** — `feature.history`/`chat.summary` là các source tiêu thụ tầng retrieval này; `change.contract` là nguồn locus), [CP-43-CATALOG: Context Source Catalog And Test Log](../inprogress/CP-43-Context-Source-Catalog-And-Test-Log.md) (catalog + test battery các context source), [CP-44: Pluggable Context Source Registry](../inprogress/CP-44-Pluggable-Context-Source-Registry.md) (registry substrate), [CP-35: Context And Regression Engine Rollout](../done/CP-35-Context-And-Regression-Engine-Rollout.md) (`changeledger`/`featurecatalog` gốc), [CP-50: Context Source Completion](../done/CP-50-Context-Source-Completion.md) (`source.excerpt` runtime — nguồn locus phụ), [CP-52: Multiple-Agent Context Synchronization](./CP-52-Multiple-Agent-Context-Sync.md) (**tiêu thụ chung** — "chống shared noise" cross-worktree dùng đúng cơ chế xếp hạng theo locus này), [Task-259: source.dependence Context Source](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md) (**chia sẻ hạ tầng** — chuẩn hóa target từ contract), [Task-246: Source-Excerpt Runtime Hint Producers](../../08-Task/done/Task-246-Source-Excerpt-Runtime-Hint-Producers.md) (extract path từ prompt + uncommitted diff — tái dùng cho locus)
- Replaces: `None`
- Tags: `context-regression-engine, change-contract, retrieval, relevance-ranking, feature-history, chat-summary, locus, change-contract, gitnexus, no-vector, deterministic, context-dilution`

## AI Quick View

### Tóm tắt

- **Vấn đề gốc:** context engine hiện truy hồi git-history + chat-summary theo **một khóa duy nhất là `feature_key`**, rồi cắt **N mục mới nhất theo thời gian**. Một `feature_key` "BIG" đời thực (ví dụ `agent-flow-engine`, phục vụ CP-51 + CP-43 + CP-44 + BUG-275/277/288...) gom hàng trăm commit của **nhiều chủ đề con song song**. Kéo context theo khóa thô đó làm prompt **loãng** (dilution): 15 commit mới nhất có thể toàn thuộc chủ đề vừa làm xong, chả liên quan tới việc đang mở lại — "newest = truth" gãy khi feature không tuyến tính.
- **Vì sao tách nhỏ `feature_key` (sub-key) không giải quyết:** mọi cách phân hoạch tĩnh đều **quyết định "cái gì liên quan" tại write-time** (lúc commit dán nhãn). Nhưng "cái gì liên quan" chỉ biết được **tại query-time** — khi đã biết mình *sắp sửa gì*. Sub-key chỉ đổi độ mịn của cú đoán, không sửa việc đoán-sai-thời-điểm.
- **Chẩn đoán:** ta đang sai **đơn vị truy hồi**. `feature_key` là một *nhãn phân loại* mờ, tĩnh, phình dần (`inferFileGlobs` khuếch đại). Thứ thực sự định nghĩa "context liên quan tới một thay đổi" là **vùng code thay đổi đó chạm vào** (tập path/symbol) — và git đã biết chính xác commit nào đụng file nào, deterministic 100%. Ta đang bỏ phí index chính xác này.
- **Giải pháp:** **đảo trục** — truy hồi neo vào **locus** (vùng code của turn, lấy từ `change.contract` + uncommitted diff + path nêu trong prompt), tính bằng **function deterministic** (no-vector, no-AI-at-collect), chạy **có điều kiện** (chỉ khi feature thực sự loãng). Function lo **recall** (thu hẹp hàng trăm → top-N đúng-vùng, rẻ, auditable, một-lần); AI lo **precision** ngữ nghĩa (miễn phí ngay trong lượt đọc prompt). Đây là **superset an toàn** của hành vi hiện tại: không có locus → rơi về recency = đúng cái đang chạy.

### Yêu cầu hiện tại

- Duyệt tài liệu ở trạng thái **draft**. Chốt (1) chẩn đoán "sai đơn vị truy hồi", (2) hướng locus-anchored, (3) phạm vi v1 tối giản (path-overlap + recency). Quyết định nâng `approved` và cắt Task con hay chưa.
- CP này **chỉ đổi read-side (retrieval/ranking) + thêm một field enabler ở ledger.** **Không** đổi cách gán `feature_key` ở write-side, **không** re-key lịch sử, **không** vector/embedding (`SD-17 D-4` giữ nguyên).

### Quyết định chính

- **QĐ-1 — Đảo trục sang locus, không đổi granularity nhãn.** Đơn vị truy hồi chính là **vùng code đang chạm** (path/symbol), không phải `feature_key`. `feature_key` tụt xuống thành *bộ lọc thô tầng-1 (recall rộng) + nhãn hiển thị + fallback*, không còn là khóa quyết định độ-liên-quan.
- **QĐ-2 — Recall = function, Precision = AI. Không phải hai lựa chọn thay thế, mà là hai tầng.** Scoring độ-liên-quan là **thuần function tất định** (đúng bất biến `Fetch` không gọi AI của CP-41/SD-22, và vì "commit nào đụng cùng file" là *fact tra cứu được*, không cần trí thông minh). AI tự tinh chỉnh ngữ nghĩa trên tập nhỏ đã lọc — miễn phí, không cần build. Để AI làm recall thì phải đưa cả đống nhiễu vào → chính là nguyên nhân dilution.
- **QĐ-3 — Deterministic, no-vector, reproducible, auditable.** Retrieval tính **một lần** ở context node, gói tinh gọn được **mọi downstream/mọi provider tái dùng** → token trả một lần, kết quả reproducible và giải thích được ("vì sao commit X bị loại"). Để AI lọc ở collect-time sẽ phá đúng ba thuộc tính này (parity Claude/Codex/Grok, reproducibility, audit).
- **QĐ-4 — Áp dụng có điều kiện = superset an toàn.** Chỉ kích hoạt scoring khi số candidate vượt ngưỡng (đề xuất > 30) **và** có locus khác rỗng. Dưới ngưỡng hoặc locus rỗng → giữ **nguyên** hành vi recency hiện tại. Feature nhỏ không đổi gì; feature BIG hết loãng. Zero-regression theo thiết kế.
- **QĐ-5 — Hạ tầng locus dùng chung với Task-259 & CP-52.** Việc "chuẩn hóa target từ `change.contract`" (symbol/file thật, loại dir-bucket/glob/doc) là **một hạ tầng** mà cả CP-54 (backward: lịch sử quanh vùng), Task-259 `source.dependence` (forward: blast-radius quanh vùng), và CP-52 P-3 (lọc noise cross-worktree) đều cắm vào. Làm một lần, ba nơi xài.
- **QĐ-6 — History và chat dùng bộ tín hiệu KHÁC nhau.** `feature.history` (commit) anchor mạnh bằng **path/symbol-overlap**. `chat.summary` (hội thoại) **không có** `ChangedPaths`/`SourceDocID` — nên anchor bằng **run-match + lexical + recency**. Không ép một mô hình chung cho hai loại dữ liệu khác bản chất.
- **QĐ-7 — Non-fatal, retryable (kế thừa `CP-43 AC-9`).** Mọi bước scoring/locus là best-effort; lỗi/thiếu dữ liệu → degrade êm về recency, không bao giờ chặn turn hay hỏng gói context.

### Ràng buộc

- Mở rộng, **không phá vỡ** `CP-35`/`CP-43` module `internal/{changeledger,featurecatalog}` và context source `feature.history`/`chat.summary`.
- Giữ bất biến **no-vector / deterministic collect** (`SD-17 D-4`, CP-41/SD-22). `Fetch` không gọi AI.
- **Không edit test cũ** — mọi test mới là additive; golden fixture (`TestBuildFlowContextPackageOutputUnchanged...`) phải giữ nguyên kỳ vọng khi locus rỗng (đúng nhánh degrade).
- Ledger là NDJSON last-wins: thêm field phải **backward-compatible** (entry cũ thiếu field → nil → nhánh recency).
- Không đổi write-side phân loại `feature_key`, không đổi `ResolveFeature`/threshold, không re-key lịch sử.

### Câu hỏi mở

- `Q-1` **Nguồn locus ở chat-mode per-turn injection.** `injectFeatureHistoryBody` (per-turn, chat mode) không có `WorkflowRunID` rõ để đọc `change.contract`. Đề xuất: locus = uncommitted diff (`git diff --name-only`) ∪ path nêu trong prompt (tái dùng `Task-246`), contract chỉ dùng khi có run. Cần xác nhận per-turn injection có nên/được phép đọc contract store không.
- `Q-2` **Ngưỡng kích hoạt & top-N.** Đề xuất kích hoạt khi candidate > 30, giữ top-N = 15 (bằng `recentHistoryEntryCount` hiện tại). Cần calibrate bằng dữ liệu thật (§6.4) trước khi cố định.
- `Q-3` **Trọng số path-overlap vs recency.** v1 chỉ 2 tín hiệu; tỉ lệ trọng số cần thử nghiệm. Kỷ luật: bắt đầu path-overlap trội hẳn recency, đo, rồi mới thêm tín hiệu.
- `Q-4` **Backfill `ChangedPaths` cho entry lịch sử cũ.** Lazy (điền khi query, cache lại) hay eager (một lệnh rebuild quét toàn ledger)? Đề xuất eager qua một bước trong `changeledger.Build`/`Compact`; entry chưa backfill → degrade recency.
- `Q-5` **`chat.summary` anchor yếu.** Vì thiếu path/symbol, `chat.summary` chỉ còn run-match + lexical + recency. Có thể v1 **chỉ áp locus-ranking cho `feature.history`** và để `chat.summary` nguyên recency (số summary per feature nhỏ hơn nhiều — `UpsertForRun` gộp 1 dòng/run). Cần chốt v1 có đụng chat.summary không.
- `Q-6` **Symbol tier (P-6) phụ thuộc Task-259.** Tầng symbol/GitNexus thừa hưởng đúng mọi blocker của Task-259 (dir-bucket vs symbol, latency budget, CLI schema chưa verify). Ship v1 path-only trước; thêm symbol khi 259 xong.
  - **Cập nhật 2026-07-27 (verify CLI thật):** blocker đã được xác nhận là **thật và nặng hơn dự kiến** — xem [BUG-323](../../09-BugFix/todo/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md): `structure.gitNexusProvider.Dependents` trả rỗng trong mọi trường hợp (sai flag `--json`, thiếu `--repo`, schema JSON sai hoàn toàn) và GitNexus **chỉ resolve symbol, từ chối file path**. Task-259 chuyển `blocked`. **P-1→P-5 của CP-54 KHÔNG bị ảnh hưởng** (path-overlap thuần từ git `ChangedPaths`, không đụng GitNexus) → tiến hành bình thường; chỉ P-6 chờ BUG-323.

### Nguồn tham chiếu

- `SS-14` US-3 (declare + flag), AC-7 (ghi symbols/files, auditable), AC-8 (context stale được cờ).
- `SD-17` D-3 (ordered history, newest = truth — **CP này tinh chỉnh chứ không bỏ**: newest vẫn là tie-break), D-4 (no-vector — **giữ tuyệt đối**).
- Code đã verify (§5): `apps/local-runner/internal/changeledger/{query.go,ledger.go,parse.go,enrich.go,chat_summary.go}`, `apps/local-runner/internal/featurecatalog/{slots.go,chat_summary_slots.go,catalog.go,resolve.go}`, `apps/local-runner/internal/runner/feature_history.go`.
- `CP-52` §3.5 (context dilution), §5.4 (chống shared noise bằng xếp hạng) — cùng gốc rễ, khác phạm vi.
- **Thứ tự triển khai (inter-CP):** `CP-43` (change contract — `approved`, Task-184 đã done, sản xuất `Contract.DeclaredPaths`) **→ `CP-54`** (tầng này — tiêu thụ `DeclaredPaths` làm locus) **→ `CP-52` P-3** (multi-agent — tái dùng scorer/locus của CP-54). CP-54 là **downstream của CP-43/CP-44**, **prerequisite của CP-52 P-3**. Quyết định 2026-07-27: **không đổi số ID, không gộp** — thứ tự biểu diễn bằng dependency + cross-ref, vì CP-43 đang `approved` và Task-184 đã done (đổi số sẽ phá ID ổn định + mồ côi done-task).

## 1. Mục tiêu

Làm cho context engine truy hồi **lịch sử & thảo luận LIÊN QUAN tới vùng code đang sửa**, thay vì toàn bộ lịch sử của một `feature_key` thô cắt theo thời gian. Cụ thể: khi một turn có mô tả vùng chạm (từ `change.contract`/diff/prompt), gói context ưu tiên các commit/summary **đụng cùng vùng đó** — kể cả khi feature_key gom hàng trăm mục của nhiều chủ đề song song. Đạt điều này **deterministic, no-vector, non-fatal, reproducible**, và là **superset an toàn** của hành vi hiện tại (feature nhỏ / không có locus → không đổi gì).

## 2. Tài liệu đầu vào

- `SD-17` (D-3 ordered-history, D-4 no-vector) — context engine mà CP này tinh chỉnh tầng retrieval.
- `SS-14` (US-3, AC-7, AC-8) — acceptance về code-context/regression mà retrieval phải tiếp tục tôn trọng.
- `CP-43` + `CP-43-CATALOG` — `feature.history`/`chat.summary` là source tiêu thụ; `change.contract` là nguồn locus.
- `CP-35` — module `changeledger`/`featurecatalog` gốc mà CP này đặt code vào.
- `Task-259` / `Task-246` — hạ tầng locus (chuẩn hóa target từ contract; extract path từ prompt/diff) dùng chung.

## 3. Bối cảnh — "context dilution" là rủi ro đã được nêu tên

CP-52 §3.5 liệt kê **context dilution** là một trong ba rủi ro tự-động-hóa của parallel agents ("báo mọi thay đổi cho mọi agent làm ngập context window bằng thông tin không liên quan, khiến chất lượng đầu ra giảm"), và §5.4 coi "một worktree chỉ nhận signal khi scope khai báo giao với blast-radius ≠ ∅" là **acceptance criteria** — tức đã thừa nhận rằng *thread-visibility mà không xếp hạng theo dependency thì biến thành shared noise* (bảng L2, CP-52 §3.4).

CP-54 nhận ra dilution **không chỉ xảy ra cross-worktree**: nó xảy ra **ngay trong một project** mỗi khi một `feature_key` gom nhiều chủ đề con. Cơ chế "xếp hạng theo locus trước khi đưa vào context" mà CP-52 P-3 cần cho cross-worktree **chính là** cơ chế CP-54 xây cho retrieval nội-project. Nên CP-54 là **tầng nền** mà CP-52 P-3 nên gọi lại, không phải xây trùng.

## 4. Mục tiêu người dùng (kịch bản thúc đẩy)

`agent-flow-engine` là một `feature_key` BIG: nó đã nuốt CP-51 (durable dispatch), CP-43 (context), CP-44 (registry), CP-36 (orchestration), BUG-275/277/288... Khi AI mở một turn để sửa một góc nhỏ (ví dụ một hàm trong dispatch state machine), context engine hiện kéo **15 commit mới nhất của cả `agent-flow-engine`** — có thể toàn commit CP-43 context vừa làm hôm trước, không liên quan gì tới dispatch. AI nhận một "Prior work" loãng, phần lớn là nhiễu. Người dùng mô tả đúng: *"1 feat BIG bao gồm nhiều feat nhỏ; chỉ dùng 1 feat id thì context loãng và không liên quan."*

## 5. Tình trạng hiện tại (đã verify bằng code)

### 5.1 Write-side — mỗi commit đã được gắn 4 trục phân loại

`changeledger.Entry` ([ledger.go:19-30](apps/local-runner/internal/changeledger/ledger.go:19)) mang: `FeatureKey`, `SourceDocID` (`Task-\d+|BUG-\d+|CP-\d+`, điền tự động từ subject/body — [parse.go:18,98](apps/local-runner/internal/changeledger/parse.go:18)), `Layer` (ui/api/domain/data), `ChangeType` (feature/bugfix/refactor...). **Nhưng KHÔNG lưu danh sách file commit đó đụng** — dù `git show --name-only` đã được chạy sẵn ở `pathFeatureKey` ([enrich.go:233](apps/local-runner/internal/changeledger/enrich.go:233)) và `changedFilesForCommit` ([catalog.go:179](apps/local-runner/internal/featurecatalog/catalog.go:179)) cho mục đích khác. → Index vàng đang bị vứt sau khi dùng.

### 5.2 Read-side — chỉ dùng 1 trong 4 trục, rồi cắt theo thời gian

- `GetFeatureHistory(featureKey)` ([query.go:10-45](apps/local-runner/internal/changeledger/query.go:10)): filter `e.FeatureKey == featureKey` **exact**, sort theo `CommittedAt`. **Không lọc theo `SourceDocID`/`Layer`/`ChangedPaths`.**
- `HistorySlot` ([slots.go:34-80](apps/local-runner/internal/featurecatalog/slots.go:34)): cắt **15 mục mới nhất** (`recentHistoryEntryCount = 15`), 3 mục cuối mang CA excerpt (`recentHistoryExcerptCount = 3`).
- `ChatSummarySlot` ([chat_summary_slots.go:10-38](apps/local-runner/internal/featurecatalog/chat_summary_slots.go:10)): cắt **3 summary mới nhất** (`recentChatSummaryCount = 3`).
- Resolve prompt → **1 feature key** top-1, threshold 5.0 ([feature_history.go:150-156](apps/local-runner/internal/runner/feature_history.go:150)).

### 5.3 Feedback loop khuếch đại

`inferFileGlobs` ([catalog.go:139-177](apps/local-runner/internal/featurecatalog/catalog.go:139)) gom **mọi path của mọi commit** vào `FileGlobs` của feature key → feature BIG có glob phủ rất rộng → `ResolveFeature`/`SuggestKey` càng dễ cộng điểm cho nó ([resolve.go:42-49,84-97](apps/local-runner/internal/featurecatalog/resolve.go:42)) → nó thành "nam châm" hút thêm prompt/commit. Càng dùng càng phình, càng phình càng loãng.

### 5.4 Chat summary có bản chất dữ liệu khác

`ChatSummaryEntry` ([chat_summary.go:12-19](apps/local-runner/internal/changeledger/chat_summary.go:12)) = `{RunID, TurnID, FeatureKey, StateKey, Summary, CreatedAt}` — **không có** `ChangedPaths` lẫn `SourceDocID`. Nên không thể anchor chat bằng file-overlap; chỉ còn run-match + lexical + recency. `UpsertForRun` gộp 1 dòng/(run,feature) nên số summary/feature = số run (nhỏ hơn số commit nhiều).

## 6. Giải pháp — Locus-anchored relevance

### 6.1 Ý tưởng trung tâm: đảo trục

Thay công thức hiện tại `filter(FeatureKey) → sort(time) → cắt N` bằng:

```
candidates = filter(FeatureKey)                       // recall rộng — giữ feature_key làm lưới thô tầng-1
locus      = buildLocus(contract, diff, prompt)       // vùng code đang chạm
if len(candidates) > threshold && locus != empty:
    ranked = sort(candidates, by=ScoreEntry(e, locus)) // precision — điểm liên quan, không phải thời gian
    return topN(ranked)                                // tie-break: recency
else:
    return recentN(candidates)                         // degrade = hành vi hiện tại (superset an toàn)
```

Câu thần chú đổi từ *"newest = truth"* thành *"most-relevant-to-what-I'm-touching = truth, tie-break bằng newest"*. `feature_key` vẫn là lưới recall tầng-1 (rẻ, không phải bỏ), nhưng **không còn là thứ quyết định độ-liên-quan**.

### 6.2 Recall (function) vs Precision (AI) — pipeline 2 tầng

| Tầng | Ai làm | Việc | Vì sao |
|---|---|---|---|
| **Recall** | Function tất định | Thu hẹp hàng trăm candidate → top-N *đụng cùng vùng code* | Là tra cứu tập hợp trên fact git/graph; rẻ, khách quan, một-lần, auditable; đúng bất biến no-vector |
| **Precision** | AI (khi đọc prompt) | Trong top-N đã lọc, tự trọng số theo ngữ nghĩa ý định | AI làm tự động & gần như miễn phí; đây là chỗ AI thực sự giỏi hơn function |

Điểm mấu chốt: **AI chỉ lọc được cái nó đã NHÌN THẤY.** "Để AI tự lọc" không xóa vấn đề, chỉ dời chỗ trả giá — hoặc bằng token (đưa hết → dilution), hoặc bằng thời gian (agentic tool → nhiều round-trip). Function prefilter là cách trả rẻ nhất cho phần thu hẹp, để dành AI đúng phần nó giỏi.

### 6.3 Mô hình chấm điểm đa tín hiệu (deterministic)

`ScoreEntry(e, locus)` = tổng có trọng số các tín hiệu — tất cả tính được từ git/graph/text, **không AI, không vector**:

| Tín hiệu | Nguồn | Áp cho history | Áp cho chat | Trọng số (v1) |
|---|---|---|---|---|
| **Path overlap** | `e.ChangedPaths ∩ locus.Paths` | ✅ (vàng) | ❌ (chat không có paths) | cao nhất |
| **Recency** | `e.CommittedAt` / `CreatedAt` | ✅ | ✅ | phụ (+ tie-break) |
| **Run-match** | `e.RunID == locus.RunID` | — | ✅ ("current discussion") | cao cho chat |
| Symbol/blast-radius overlap | GitNexus `Dependents` | ⏳ P-6 | ❌ | (hoãn) |
| SourceDocID / Layer match | `e.SourceDocID`, `e.Layer` | ⏳ sau | ❌ | (hoãn) |
| Lexical | `summary` ∩ `locus.IntentKeywords` | ⏳ sau | ✅ | (chat v1 tùy chọn) |

**v1 tối giản: chỉ path-overlap + recency cho `feature.history`.** Thêm tín hiệu chỉ khi đo thấy cần (`Q-3`). Chat.summary xem `Q-5`.

### 6.4 Áp dụng có điều kiện = superset an toàn

Kích hoạt scoring chỉ khi `len(candidates) > ngưỡng` (đề xuất 30) **và** `locus` khác rỗng. Ngược lại → nhánh recency hiện tại, **byte-for-byte** như trước. Hệ quả: (a) feature nhỏ không đổi; (b) golden fixture giữ nguyên (chúng chạy không có contract → locus rỗng → nhánh cũ); (c) không có contract/diff/prompt-path → tự động về hành vi cũ. Đây là điều làm CP-54 **zero-regression theo thiết kế**.

### 6.5 Hợp nhất với `source.dependence` (Task-259)

Hai thứ là **hai mặt của một bài toán**, cùng neo locus:
- **Task-259 `source.dependence`** — *forward*: "sửa vùng này → **sẽ** ảnh hưởng cái gì" (blast-radius tương lai).
- **CP-54** — *backward*: "vùng này **đã** từng bị ai sửa, khi nào, tại sao" (lịch sử liên quan).

Cả hai đọc `change.contract` và chuẩn hóa target giống nhau (T-3 của Task-259). → §7 P-2 tách phần đó thành **hạ tầng dùng chung** để hai source (và CP-52 P-3) cùng gọi.

## 7. Kế hoạch phase (Work Breakdown — chi tiết)

> Ký hiệu code dưới đây là **đề xuất chữ ký**, chưa implement. File mới đặt trong module có sẵn.

### P-1 — Ghi `ChangedPaths` vào ledger (enabler nền tảng)

**Mục tiêu:** persist danh sách file mỗi commit đụng, để read-side không phải `git show` lúc query.

- Thêm field vào `Entry` ([ledger.go:19](apps/local-runner/internal/changeledger/ledger.go:19)):
  ```go
  ChangedPaths []string `json:"changed_paths,omitempty"` // forward-slash, repo-relative; nil cho entry cũ
  ```
- Điền lúc `EnrichAll`/`enrichEntry` ([enrich.go:29,45](apps/local-runner/internal/changeledger/enrich.go:29)): tái dùng một lần gọi `git show --name-only` (hiện `pathFeatureKey` đã gọi — gộp để không chạy git 2 lần). Chuẩn hóa `filepath.ToSlash`, bỏ rỗng.
- **Backward-compat:** entry cũ đọc lên `ChangedPaths == nil` → read-side coi như "không có locus signal" cho entry đó → degrade recency. Không vỡ file cũ.
- **Backfill (`Q-4`):** một bước trong `changeledger.Build` (hoặc lệnh `Compact`-kèm-enrich) quét entry thiếu `ChangedPaths` và điền. Non-fatal.
- **Acceptance:** parse một commit → `Entry.ChangedPaths` liệt kê đúng file; entry NDJSON cũ (không field) load không lỗi và giữ `nil`; git vắng/commit lỗi → `nil`, không panic.

### P-2 — Hạ tầng locus dùng chung (`internal/runner/retrieval_locus.go` — mới)

**Mục tiêu:** một nơi duy nhất dựng "vùng code đang chạm", dùng chung với Task-259/CP-52.

- Struct:
  ```go
  type RetrievalLocus struct {
      Paths          []string // file code cụ thể (đã chuẩn hóa)
      Symbols        []string // khi có (từ contract.DeclaredSymbols)
      RunID          string   // cho chat run-match
      IntentKeywords []string // token từ intent/prompt (cho lexical, tùy chọn)
  }
  func buildRetrievalLocus(ctx, workspace, runID, prompt string) RetrievalLocus
  ```
- Nguồn (hợp nhất, ưu tiên giảm dần): `change.contract.DeclaredPaths`/`DeclaredSymbols` (`GetLatestForRun` khi có `runID`) → uncommitted diff (`git diff --name-only`, tái dùng `uncommittedChangedPaths` của `source.excerpt`/Task-246) → path nêu trong prompt (`extractPromptSourcePaths`).
- **Chuẩn hóa target dùng chung Task-259 T-3:** chỉ giữ file code cụ thể (có đuôi code, không phải dir-bucket/glob/doc). Đây là hàm hai source cùng gọi.
- **Acceptance:** contract declared có path thật → locus.Paths đầy; contract inferred (dir-bucket) → dir-bucket bị loại, rơi sang diff/prompt; tất cả rỗng → locus rỗng (kích nhánh degrade).

### P-3 — Relevance scorer thuần function (`internal/featurecatalog/relevance.go` — mới)

**Mục tiêu:** hàm tất định chấm điểm 1 entry theo locus.

```go
func ScoreHistoryEntry(e changeledger.Entry, locus RetrievalLocus) float64 // v1: path-overlap + recency
```
- Path-overlap = `|set(e.ChangedPaths) ∩ set(locus.Paths)|` (có thể chuẩn hóa theo min-size); trọng số trội. Recency = hàm đơn điệu theo `CommittedAt` (đã sort), trọng số phụ.
- **Tất định tuyệt đối:** điểm bằng nhau → tie-break bằng `CommitHash` (như `GetFeatureHistory` hiện tại — [query.go:34-39](apps/local-runner/internal/changeledger/query.go:34)), tuyệt đối không phụ thuộc map-iteration (bài học BUG-266).
- **Acceptance:** overlap cao → điểm cao hơn hẳn recency; overlap = 0 → chỉ recency (bằng thứ tự cũ); chạy 2 lần trên cùng input → kết quả y hệt.

### P-4 — Đổi read-side `feature.history` sang ranked, có điều kiện

**Mục tiêu:** cắm scorer vào `HistorySlot` mà không đổi hành vi khi không có locus.

- Thêm biến thể nhận locus, ví dụ `HistorySlotRanked(featureKey string, ledger, locus RetrievalLocus) string` (giữ `HistorySlot` cũ gọi vào với locus rỗng để tương thích).
- Logic: lấy `candidates = GetFeatureHistory(featureKey)`; nếu `len > ngưỡng && locus != rỗng` → `ScoreHistoryEntry` → top-N theo điểm (tie-break recency) → **vẫn giữ marker "current truth" cho commit newest tuyệt đối**, và ghi dòng "(… đã lọc theo vùng đang sửa: hiện N/M mục liên quan …)". Ngược lại → nhánh recency hiện tại nguyên vẹn.
- Điểm nối: `composeFeatureBlocks` ([feature_history.go:96-119](apps/local-runner/internal/runner/feature_history.go:96)) và context source `feature.history` (flow mode) truyền locus xuống; chat-mode per-turn dựng locus theo `Q-1`.
- **Acceptance:** feature BIG (M>30) + locus đụng vùng X → chỉ commit đụng X nổi lên top-N; feature nhỏ (M≤30) → output byte-for-byte như cũ; locus rỗng → như cũ.

### P-5 — `chat.summary` (theo `Q-5`)

**Mục tiêu:** xử lý chat theo bản chất riêng, hoặc để nguyên v1.

- Phương án A (an toàn, đề xuất v1): **để `chat.summary` nguyên recency** — số summary/feature nhỏ (`UpsertForRun` gộp/run), dilution nhẹ hơn nhiều; không đụng để giảm bề mặt rủi ro.
- Phương án B (nếu cần): rank chat bằng `run-match (locus.RunID) + lexical + recency`; không path-overlap.
- **Acceptance:** nếu chọn A → không có thay đổi nào ở `ChatSummarySlot`, ghi rõ lý do; nếu B → summary cùng run hiện tại luôn được giữ (không bị đẩy khỏi top bởi recency của summary run khác).

### P-6 — Symbol/GitNexus tier + telemetry (hoãn phần symbol; làm telemetry sớm)

- **Telemetry/audit (làm sớm, cùng P-4):** log điểm từng entry được chọn/loại vào gate audit (`AC-7` auditable) → trả lời được "vì sao commit X bị loại". Deterministic nên log tái lập được.
- **Symbol tier (hoãn — `Q-6`, phụ thuộc Task-259):** thêm tín hiệu symbol-overlap qua `structure.Dependents` (chung hạ tầng Task-259), GitNexus-only, bounded, degrade. Bắt liên quan gián tiếp mà path phẳng bỏ sót.
- **Acceptance (symbol):** đo lại khi land; kỳ vọng LOW risk vì chỉ cắm vào scorer (không đụng render/append-prompt — xem CP-43-CATALOG §5).

## 8. Vùng bị đụng

- **Đổi:** `internal/changeledger/` (`ledger.go` +field; `enrich.go`/`parse.go` điền `ChangedPaths`; có thể `query.go` thêm biến thể trả kèm điểm). `internal/featurecatalog/` (`slots.go` biến thể ranked; `relevance.go` **mới**). `internal/runner/` (`feature_history.go` truyền locus; `retrieval_locus.go` **mới**; context source `feature.history` truyền locus).
- **Tái dùng không đổi:** `source.excerpt` path-extraction (`Task-246`), `change.contract` store (`GetLatestForRun`), `structure.Dependents` (chỉ P-6).
- **Không đụng:** write-side `feature_key` (`parse.go` tag parsing giữ nguyên), `ResolveFeature`/threshold, registry CP-44, render path (generic pass), `composeFlowNodeAgentPrompt` (HIGH-risk — CP-43-CATALOG §5).
- **Database:** không. Toàn bộ local dưới `<target>/.flowpilot/ledger/`.

## 9. Data / Migration

- **Không Supabase migration.** Thêm field `changed_paths` vào NDJSON `feature_history.ndjson` (optional, omitempty) — backward-compatible.
- **Backfill (`Q-4`):** eager qua `changeledger.Build`/rebuild điền `ChangedPaths` cho entry cũ; entry chưa backfill → nil → degrade recency (không lỗi).
- **`contextsync`:** `changed_paths` là dữ liệu phái sinh từ git local; giữ trong `feature_history.ndjson` theo đúng chế độ sync hiện tại của ledger (không thêm shared-file mới).

## 10. Validation Plan

- **Unit:**
  - `ChangedPaths` parse đúng; entry cũ (thiếu field) load nil không lỗi (P-1).
  - `buildRetrievalLocus`: contract-declared → paths; inferred/dir-bucket → loại → fallback diff/prompt; tất cả rỗng → rỗng (P-2).
  - `ScoreHistoryEntry`: overlap↑→điểm↑; overlap=0→chỉ recency; tất định (2 lần giống nhau); tie-break CommitHash (P-3).
  - `HistorySlotRanked`: M>30 + locus → top-N theo vùng; M≤30 → nhánh cũ; locus rỗng → nhánh cũ (P-4).
- **Integration:** feature BIG synthetic (nhiều commit, ≥3 chủ đề theo SourceDocID/paths khác nhau) + contract khai vùng X → gói context chỉ nêu commit vùng X; đổi contract sang vùng Y → gói đổi theo, không cần re-index.
- **Regression (bắt buộc xanh, không sửa kỳ vọng):** `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor`, `TestBuildFlowContextPackageVerifiedFeatureIncludesHistory`, `TestRenderFlowContextPackageStableSections`, toàn bộ battery §4 CP-43-CATALOG.
- **Manual:** trên repo thật, `feature_key = agent-flow-engine`, mở turn sửa một hàm dispatch → xác nhận "Prior work" nêu commit dispatch, không phải commit CP-43 mới nhất; feature nhỏ → prompt không đổi so với trước.

## 11. Rollout & Fallback

- **Thứ tự:** P-1 (ghi ChangedPaths + backfill) → P-2 (locus) → P-3 (scorer) → P-4 (read-side history, có điều kiện) → P-6 telemetry → (P-5 chat, P-6 symbol khi cần).
- **Feature-flag:** ngưỡng kích hoạt + bật/tắt ranking qua settings; tắt → về đúng hành vi hiện tại (không mất dữ liệu; `changed_paths` chỉ nằm im).
- **Fallback:** mọi tầng degrade về recency; git/contract/GitNexus vắng đều không chặn turn.

## 12. Rủi ro

- `R-1` **Locus yếu khi contract inferred** (chỉ dir-bucket). Giảm nhẹ: hợp nhất diff + prompt-path vào locus (P-2); dir-bucket vẫn cho overlap mức thư mục (vẫn hơn feature-key thô); rỗng hẳn → degrade recency an toàn.
- `R-2` **Over-engineer trọng số.** Giảm nhẹ: v1 chỉ 2 tín hiệu; thêm chỉ khi đo thấy cần (`Q-3`); kỷ luật "đo trước, thêm sau".
- `R-3` **I/O `git show` lúc write.** Giảm nhẹ: gộp lần gọi đã có ở `pathFeatureKey`; persist một lần, đọc nhiều lần; không `git show` lúc query.
- `R-4` **Chat anchor yếu** (`Q-5`). Giảm nhẹ: v1 có thể để chat nguyên recency (Phương án A).
- `R-5` **Đổi ngữ nghĩa "newest = truth"** (`SD-17 D-3`). Giảm nhẹ: newest tuyệt đối vẫn được giữ + đánh dấu; ranking chỉ đổi *thứ tự đưa vào*, không bỏ truth; chỉ bật khi thực sự loãng (M>ngưỡng).
- `R-6` **Symbol tier kéo theo blocker Task-259** (`Q-6`). Giảm nhẹ: tách hẳn khỏi v1 path-only.

## 13. Definition of Done

- [x] `P-1` `Entry.ChangedPaths` được ghi cho commit mới; entry cũ load nil không lỗi; backfill chạy được; git vắng → nil, non-fatal. Done via Task-261 / CA-422.
- [x] `P-2` `buildRetrievalLocus` hợp nhất contract/diff/prompt, chuẩn hóa target (loại dir-bucket/glob/doc dùng chung Task-259), locus rỗng khi không nguồn nào có. Done via Task-262 / CA-423; symbols finalized via GitNexusImpactTargets (CA-436).
- [x] `P-3` `ScoreHistoryEntry` tất định, path-overlap trội recency, tie-break CommitHash; symbol tier thứ hai (SymbolOverlap). Done via Task-268 / CA-429 + CA-436.
- [x] `P-4` `feature.history` ranked **có điều kiện** (M>ngưỡng && locus≠rỗng); dưới ngưỡng/rỗng → output byte-for-byte như cũ; newest vẫn được giữ + marker. Done via Task-269 / CA-430 + CP-55 P-8.
- [x] Superset an toàn: golden/regression test cũ xanh; test mới additive; `go test ./internal/{changeledger,featurecatalog,runner}/...` sạch (2026-08-11).
- [x] Deterministic/no-vector giữ nguyên (`Fetch` không gọi AI); telemetry log điểm chọn/loại (`AC-7`) — `[context-rank]` (CA-436).
- [x] Non-fatal/retryable (`AC-9`): mọi bước degrade về recency, không chặn turn.
- [x] Integration test feature-BIG: `TestFirstCoderContextRanksFeatureHistoryByCurrentLocus` + Flow E2E frozen writer (CA-431); manual 4.M1 in CP-54-Test-Steps.
- [x] (`Q-5`) v1 **không** locus-rank `chat.summary` — giữ recency (số summary/run nhỏ; anchor yếu).
- [x] Satisfies `SS-14` US-3/AC-7/AC-8 ở tầng retrieval; nhất quán `SD-17 D-3` (newest tie-break) và `D-4` (no-vector).

**Deferred (not v1 code):** GitNexus `Dependents` at rank-time — blast-radius stays on `source.dependence` / `r-scope` (CP-43 P-6).

## 14. Non-Goals

- Tách nhỏ / re-key `feature_key` ở write-side (đây là partition tĩnh mà CP này chủ ý **không** dùng làm lời giải).
- Vector/embedding/semantic retrieval (`SD-17 D-4` giữ nguyên).
- Gọi AI lúc collect để chấm relevance (phá bất biến + reproducibility/parity).
- Đổi `ResolveFeature`/threshold hay topology flow.
- Auto-merge/rebase code (thuộc CP-52 non-goals; ngoài phạm vi).

## 15. Notes / Cross-refs

- **Quan hệ CP-43:** CP-54 là *tầng retrieval/ranking* nằm dưới các context source `feature.history`/`chat.summary` mà CP-43/CP-50 đã ship. CP-43 định nghĩa "đọc authority, không đọc log"; CP-54 định nghĩa "trong cái phải đọc, đọc phần liên quan tới vùng đang sửa trước".
- **Quan hệ CP-52:** cùng gốc "context dilution / chống shared noise bằng xếp hạng theo dependency". CP-52 P-3 (cross-worktree) nên **gọi lại** scorer/locus của CP-54 thay vì xây trùng.
- **Quan hệ Task-259:** backward (CP-54) vs forward (259) quanh cùng locus; chia sẻ hàm chuẩn hóa target (P-2).
- **Nguyên tắc dẫn đường:** *đơn vị truy hồi đúng là vùng code đang chạm, không phải cái nhãn feature; và độ-liên-quan phải quyết định tại query-time, không phải write-time.*
