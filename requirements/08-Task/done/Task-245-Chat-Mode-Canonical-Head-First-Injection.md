# Task-245: Chat-Mode Prompt Dẫn Đầu Bằng Canonical Head

## Metadata

- Document ID: `Task-245`
- Title: `Chat-Mode Canonical-Head-First Injection`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md) (P-2), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md) (D-3), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-9)
- Child Documents: `None`
- Related Documents: [Task-243: Review Và Capture Các Context Artifact Source](./Task-243-Context-Artifact-Sources-Review-And-Capture.md) (G-2, Q-2), [Task-244: Canonical Head First-Class Context Source](./Task-244-Canonical-Head-First-Class-Context-Source.md) (làm TRƯỚC task này), [BUG-268](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md), [BUG-277 — xem Task-224 skip conditions]
- Replaces: `None`
- Tags: `canonical-head, chat-mode, prompt-injection, handoff, local-runner`

## AI Quick View

### Summary

- `composeFeatureBlocks` (per-turn injection Chat mode + cross-provider handoff) prepend `RenderHeadBlock` → prompt Chat mode cũng dẫn đầu bằng Canonical Head, không chỉ Flow Mode.
- Behavior change có chủ đích: feature có Head nhưng CHƯA có commit history giờ inject **head-only** (trước đây inject nothing).
- Đóng `G-2` của Task-243; CP-43 P-5 "resolved code turns lead with the Head" thành đúng cho cả 2 mode.

### Current Ask

- Thay nguyên hàm `composeFeatureBlocks` theo skeleton T-1 (đã rehearse 2026-07-15) + 3 test.

### Key Decisions

- `T-D1` Head-only injection khi không có history là CÓ CHỦ ĐÍCH (Head = current truth, giá trị cao nhất) — đừng "sửa lại cho giống cũ".
- `T-D2` KHÔNG sửa các skip-condition trong `injectFeatureHistoryPrompt` — chính chúng ngăn double-Head với Flow package (bài học BUG-268/BUG-277).

### Constraints

- Làm SAU Task-244 (Flow package đã có Head qua source riêng — cần skip-conditions giữ nguyên để không double).
- `LoadHead` nhận **workspace**; hàm này nhận **dotFlowpilotDir** → bắt buộc `filepath.Dir(dotFlowpilotDir)`.
- Head thiếu/lỗi → không block, không error (AC-9). `changeledger.New` lỗi → giữ nguyên return "" sớm (không đổi error handling).

### Open Questions

- Không.

### Source Refs

- `CP-50 §4.2`. `Task-243` G-2, Q-2. `SD-21` D-3. `SS-14` AC-9.

## 1. Goal

Turn Chat mode (và cross-provider handoff) trên feature verified có Head → prompt inject dẫn đầu `## Canonical state`, sau đó `## Prior work`, sau đó prior discussion; feature có Head nhưng chưa có history vẫn nhận Head.

## 2. Parent Links

- coding plan: `CP-50` P-2
- tech design: `SD-21` D-3
- system spec: `SS-14` AC-9
- specific upstream ids: `CP-50 P-2`, `Task-243 Q-2`

## 3. Trigger

Task-243 `G-2`: chỉ Flow-mode Plan-step path có Head; Chat mode (`injectFeatureHistoryPrompt` → `composeFeatureBlocks`) chưa từng load Head. Owner chốt (Q-2): Chat mode cũng phải có.

## 4. Exact Change

- `T-1` **`apps/local-runner/internal/runner/feature_history.go` — thay nguyên hàm `composeFeatureBlocks` + thêm import `"flowpilot-runner/internal/changecontract"`:**

```go
// composeFeatureBlocks returns the Canonical Head + prior-work (+ prior-
// discussion) blocks for a known feature key, or "" when the feature has
// neither a Head nor committed history. Shared by per-turn injection and the
// cross-provider handoff.
//
// Task-245 (CP-50 P-2, Task-243 Q-2): Chat-mode prompts lead with the
// Canonical Head too — matching Flow-mode's canonical.head source (Task-244).
// A missing/unreadable Head degrades to no block (AC-9); a feature with a
// Head but no committed history still injects the Head alone (deliberate).
func composeFeatureBlocks(dotFlowpilotDir string, featureKey string) string {
    ledger, err := changeledger.New(dotFlowpilotDir)
    if err != nil {
        return ""
    }
    history := strings.TrimSpace(featurecatalog.HistorySlot(featureKey, ledger))

    headBlock := ""
    if head, found, herr := changecontract.LoadHead(filepath.Dir(dotFlowpilotDir), featureKey); herr == nil && found {
        headBlock = strings.TrimSpace(changecontract.RenderHeadBlock(head))
    }
    if history == "" && headBlock == "" {
        return ""
    }

    var parts []string
    if headBlock != "" {
        parts = append(parts, headBlock)
    }
    if history != "" {
        parts = append(parts, history)
    }
    if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFlowpilotDir); err == nil {
        if discussion := featurecatalog.ChatSummarySlot(featureKey, summaryLedger); strings.TrimSpace(discussion) != "" {
            parts = append(parts, discussion)
        }
    }
    return strings.Join(parts, "\n\n")
}
```
  Caller thứ hai `handoff_context.go:239` hưởng tự động — KHÔNG sửa gì thêm.

- `T-2` **Tests** (thêm cạnh test hiện có của feature_history; nếu chưa có file phù hợp → tạo `feature_history_canonical_head_test.go`):
  - `TestComposeFeatureBlocksLeadsWithCanonicalHead` — ledger có entry + `changecontract.SaveHead` → kết quả bắt đầu `## Canonical state`, chứa `## Prior work` phía sau.
  - `TestComposeFeatureBlocksHeadOnlyWhenNoHistory` — chỉ SaveHead, không ledger entry → trả head-only (khác "" như trước).
  - `TestComposeFeatureBlocksEmptyWhenNoHeadNoHistory` — không Head + không history → "" (contract cũ giữ nguyên).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/feature_history.go` (+ file test)
- modules: runner chat-mode prompt injection, cross-provider handoff
- routes: không
- tables: không

## 6. Acceptance Check

- DONE Chat-mode turn trên feature verified có Head → prompt inject dẫn đầu `## Canonical state` rồi mới `## Prior work` (+ discussion nếu có).
- DONE Feature có Head, chưa có commit history → inject head-only (có test).
- DONE Flow-mode prompt KHÔNG double-Head: skip-conditions của `injectFeatureHistoryPrompt` nguyên trạng; full `go test ./internal/runner/ -count=1` không fail mới.
- DONE 3 test T-2 pass; build/vet sạch.
- Manual check (sandbox): 1 turn chat thường trên feature có Head → prompt-log dẫn đầu bằng Head. *(skip — e2e live)*

## 7. Out of Scope

- Source `canonical.head` của Flow mode (Task-244).
- Thay đổi bất kỳ skip-condition/feature-resolution nào trong `injectFeatureHistoryPrompt`.
- change.contract / source.excerpt (Task-246/247).

## 8. Completion Notes

- result: `done` (2026-07-15).
- follow-ups: none.
- upstream docs updated: none.
