# Task-261: Persist `ChangedPaths` In The Change Ledger (CP-54 P-1 enabler)

## Metadata

- Document ID: `Task-261`
- Title: `Persist ChangedPaths In Change Ledger`
- Phase: `task`
- Status: `done` (2026-07-27 — implement + verify xong, xem [CA-422](../../../change-audit/CA-422-persist-changed-paths-in-change-ledger.md))
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-27`
- Last Updated: `2026-07-27`
- Feature Keys: `context-regression-engine`
- Parent Documents: [CP-54: Locus-Anchored Relevance Retrieval](../../07-Coding-Plan/todo/CP-54-Locus-Anchored-Context-Relevance.md) (P-1), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) (D-3 ordered-history, D-4 no-vector), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-7 ghi files changed)
- Child Documents: `none`
- Related Documents: [CP-35](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md) (§4.1 `changeledger` gốc), [CP-43](../../07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [BUG-266] (determinism tie-break `CommitHash` — phải giữ), [BUG-323](../../09-BugFix/todo/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md) (**không chặn task này** — P-1 thuần git, không đụng GitNexus)
- Replaces: `None`
- Tags: `context-regression-engine, changeledger, locus, retrieval, enabler, additive, backward-compatible`

## AI Quick View

### Summary

Persist danh sách file mà **mỗi commit đụng vào** ngay trong `changeledger.Entry`, để tầng retrieval của CP-54 chấm điểm độ-liên-quan theo **vùng code** mà không phải chạy `git show` lúc query. Đây là **enabler nền tảng**: không có field này thì P-2→P-5 của CP-54 không có gì để so khớp.

Thuần additive, backward-compatible, **không đụng GitNexus** (nên không bị BUG-323 chặn), không đổi write-side phân loại `feature_key`, không đổi thứ tự đọc hiện tại.

### Current Ask

Thêm `Entry.ChangedPaths []string` + điền nó khi build ledger, với chi phí git **không tăng theo số commit**.

### Key Decisions

- `T-1` **Một lượt `git log --name-only` riêng, KHÔNG sửa `parseRecord`.** CP-54 §7 P-1 đề xuất "tái dùng lệnh `git show --name-only` mà `pathFeatureKey` đã gọi" — **giả định này sai**: `pathFeatureKey` chỉ chạy ở **Priority 3** của `enrichEntry` ([enrich.go:88](../../../apps/local-runner/internal/changeledger/enrich.go)), trong khi phần lớn entry return sớm ở Priority 0/1 ([dòng 59](../../../apps/local-runner/internal/changeledger/enrich.go), [72](../../../apps/local-runner/internal/changeledger/enrich.go)) và **không bao giờ gọi git show**. Điền theo đường đó sẽ biến thành **N subprocess** (một per commit).
  ⇒ Thay vào đó: thêm **một** lệnh `git log --name-only --pretty=format:%H<sep>` trên **cùng dải commit** mà `ParseRepo` đang dùng, dựng `map[hash][]paths`, rồi gán. Tổng chi phí: **+1 subprocess cho cả lượt build**, không phụ thuộc số commit.
- `T-2` **Không đổi `gitLogFormat`/`parseRecord`.** Nhét `--name-only` vào lệnh `git log` sẵn có sẽ làm file list chen vào giữa các record `\x1e`, đổi hợp đồng parse và **buộc phải sửa test cũ** — vi phạm `additive-tests-only`. Lượt thứ hai tách biệt giữ `parseRecord` nguyên vẹn.
- `T-3` **Backward-compatible tuyệt đối.** Entry NDJSON cũ không có field → unmarshal ra `nil` → read-side coi như "không có locus signal" → degrade recency (đúng hành vi hiện tại). `omitempty` để entry không có path không phình file.
- `T-4` **Chuẩn hóa `filepath.ToSlash`**, repo-relative, bỏ dòng rỗng, **sort** để tất định (BUG-266: không phụ thuộc thứ tự map/git).
- `T-5` **Non-fatal.** Git vắng / lệnh lỗi / commit không có file (empty, merge) → `ChangedPaths = nil`, không panic, không chặn build ledger. Giống tinh thần `ParseRepo` hiện tại ([parse.go:40-43](../../../apps/local-runner/internal/changeledger/parse.go)).

### Constraints

- **Không sửa test cũ** (`additive-tests-only`, `oracle-rule`). Test `changeledger` hiện có phải xanh nguyên trạng.
- Không đổi thứ tự/tie-break của `GetFeatureHistory` (BUG-266 `CommitHash` tie-break giữ nguyên).
- Không thêm dependency, không thêm bảng DB, không vector (`SD-17 D-4`).
- Ledger là NDJSON last-wins theo `commit_hash` — thêm field không được phá `loadFromDisk`.

### Open Questions

- `Q-1` **Backfill entry cũ** (CP-54 `Q-4`): eager (một lượt quét khi `Build` thấy entry thiếu field) hay lazy (điền dần)? Đề xuất **eager trong `Build`**, non-fatal, vì đằng nào cũng đã có sẵn `map[hash][]paths` của lượt `git log` — chỉ cần mở rộng dải commit khi phát hiện entry thiếu. Chốt lúc implement.
- `Q-2` Có cap số path mỗi entry không (commit refactor lớn có thể đụng hàng trăm file)? Đề xuất **chưa cap** ở P-1 (dữ liệu thô nên đầy đủ); việc cap là chuyện của scorer P-3.

### Source Refs

- Code: [ledger.go:19-30](../../../apps/local-runner/internal/changeledger/ledger.go) (`Entry`), [ledger.go:150](../../../apps/local-runner/internal/changeledger/ledger.go) (`Build`), [parse.go:14](../../../apps/local-runner/internal/changeledger/parse.go) (`gitLogFormat`), [parse.go:30](../../../apps/local-runner/internal/changeledger/parse.go) (`ParseRepo`), [enrich.go:29](../../../apps/local-runner/internal/changeledger/enrich.go) (`EnrichAll`), [enrich.go:233](../../../apps/local-runner/internal/changeledger/enrich.go) (`pathFeatureKey`).
- Impact analysis (chạy 2026-07-27, `npx gitnexus impact <sym> --repo flowpilot`): `ParseRepo` → **LOW**, impacted 3, processes 0, direct caller `Build` → `runEngineInit` (indirect). `EnrichAll` → **LOW**, cùng hình dạng. An toàn.

## 1. Goal

Sau khi build ledger, mỗi `Entry` mang đúng danh sách file repo-relative mà commit đó chạm vào, để CP-54 P-3 có thể tính path-overlap với locus của turn — deterministic, không cần git lúc query.

## 2. Parent Links

- coding plan: `CP-54` P-1
- tech design: `SD-17` D-3 / D-4
- system spec: `SS-14` AC-7

## 3. Trigger

CP-54 chẩn đoán context bị loãng vì truy hồi neo vào `feature_key` thô rồi cắt theo thời gian. Muốn neo vào **vùng code**, read-side phải biết commit nào đụng file nào. Git biết chính xác điều đó; ledger hiện **không lưu**. Task này lấp đúng khoảng trống đó.

## 4. Exact Change

- `T-A` `Entry` ([ledger.go:19](../../../apps/local-runner/internal/changeledger/ledger.go)) — thêm:
  ```go
  ChangedPaths []string `json:"changed_paths,omitempty"` // repo-relative, forward-slash, sorted; nil cho entry cũ
  ```
- `T-B` `changeledger` — hàm mới (file mới `changed_paths.go`, giữ `parse.go` nguyên):
  ```go
  // changedPathsByCommit chạy MỘT lệnh `git log --name-only` trên cùng dải commit
  // và trả map hash → danh sách path đã chuẩn hóa + sort. Non-fatal: lỗi → map rỗng.
  func changedPathsByCommit(repoDir, commitRange string) map[string][]string
  ```
- `T-C` `ParseRepo` ([parse.go:30](../../../apps/local-runner/internal/changeledger/parse.go)) — sau khi parse xong entries, gọi `changedPathsByCommit` **một lần** với đúng dải commit đang dùng (`cursor..HEAD` hoặc toàn bộ) và gán `e.ChangedPaths`. Không đổi `gitLogFormat`, không đổi `parseRecord`.
- `T-D` Backfill (`Q-1`) — trong `Build` ([ledger.go:150](../../../apps/local-runner/internal/changeledger/ledger.go)): entry đã có trong ledger nhưng thiếu `ChangedPaths` được điền từ cùng map (mở rộng dải nếu cần). Non-fatal.
- `T-E` Test **additive**, file mới `changed_paths_test.go`.

## 5. Touched Areas

- files: `apps/local-runner/internal/changeledger/ledger.go` (thêm field), `.../parse.go` (gán sau parse), `.../changed_paths.go` (**mới**), `.../changed_paths_test.go` (**mới**)
- modules: `changeledger` (direct), `runner` (indirect qua `runEngineInit` — không sửa)
- routes: none
- tables: none (NDJSON local)

## 6. Acceptance Check (DoD)

- [x] Build ledger trên repo git thật → `Entry.ChangedPaths` liệt kê **đúng** file của commit đó. `TestParseRepoAttachesChangedPaths` + `TestChangedPathsByCommitReportsEachCommitsFiles` chạy trên repo git thật (temp); format đối chiếu trực tiếp trên repo flowpilot (§Verification của CA-422).
- [x] Chi phí git **không** tăng theo số commit — **+1** lệnh `git log` cho cả dải. `Q-1` đã chốt: **không** đi qua `pathFeatureKey` (nó chỉ chạy ở Priority 3 ⇒ sẽ thành N subprocess), xem `T-1`.
- [x] Entry NDJSON **cũ** load không lỗi, `ChangedPaths == nil` — `TestEntryWithoutChangedPathsFieldLoadsAsNil`; `omitempty` xác nhận bởi `TestEntryWithoutChangedPathsOmitsKeyOnMarshal`.
- [x] Path `ToSlash`, repo-relative, **sorted**, bỏ dòng rỗng; 2 lần chạy y hệt — `TestParseChangedPathsChunkSortsForDeterminism`, `TestChangedPathsByCommitIsDeterministic`.
- [x] Git vắng / không phải repo / cursor không phải ancestor / commit không file → `nil`/map rỗng, không panic — `TestChangedPathsByCommitNonFatalWithoutGitRepo`, `...WithBadCursor`, `TestParseChangedPathsChunkCommitWithNoFiles`.
- [x] Backfill entry cũ — `TestBuildBackfillsChangedPathsOnLegacyEntries` (cursor đặt tại HEAD nên chỉ backfill mới điền được); bỏ qua git khi không thiếu gì — `TestBackfillChangedPathsSkipsGitWhenNothingMissing`. **Residual đã ghi:** commit unreachable/không-file không điền được ⇒ lượt `git log` lặp mỗi engine-init (bounded, non-fatal).
- [x] **Test cũ `changeledger` xanh nguyên trạng** — `git status --short` chỉ có `ledger.go`+`parse.go` sửa và 2 file mới; **không file test nào bị đụng**.
- [x] Test mới additive — 15 case trong `changed_paths_test.go`, không case nào skip.
- [x] `go build ./...` + `go vet` sạch; `go test ./internal/changeledger/... -count=1` → **42 passed**. Downstream `internal/runner` (context/feature-history/canonical-head/change-contract) → ok.
- [x] CA note: [CA-422](../../../change-audit/CA-422-persist-changed-paths-in-change-ledger.md).
- [x] Cross-provider: Case 1 agnostic — grep `providerKey|ProviderKey|Provider` trong `internal/changeledger/` → **0 kết quả** (ghi trong CA-422).

## 7. Out of Scope

- Locus builder (CP-54 P-2), scorer (P-3), wiring vào `HistorySlot` (P-4), `chat.summary` (P-5), symbol tier (P-6).
- Đổi cách gán `feature_key`, đổi `ResolveFeature`/threshold, re-key lịch sử.
- Bất cứ gì đụng GitNexus/`structure` (thuộc BUG-323).

## 8. Cross-Provider Note

`changeledger` là **provider-agnostic**: nó đọc git + change-audit, không nhận `providerKey`, không branch theo provider. Bằng chứng cần ghi lúc implement: grep `providerKey|ProviderKey` trong `internal/changeledger/` → không kết quả. Không cần ma trận Claude/Codex/Grok cho task này (`cross-provider-parity` Case 1), nhưng phải **nêu bằng chứng tường minh** trong CA note.
