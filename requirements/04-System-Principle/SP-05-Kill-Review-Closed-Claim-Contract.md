# SP-05: Hợp đồng Kill-Review (Claim đóng)

## Metadata

- Document ID: `SP-05`
- Title: `Kill-Review — Hợp đồng claim đóng & chấm dứt review`
- Phase: `system_principle`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [SP-04 Safe Gate](./SP-04-safe-gate.md)
- Child Documents: skill `kill-review` (portable, **tự chứa** — không phụ thuộc file này khi share sang project khác): [`.agents/skills/kill-review/SKILL.md`](../../.agents/skills/kill-review/SKILL.md) ≡ [`apps/local-runner/internal/skillpack/flow-pack/common/kill-review/SKILL.md`](../../apps/local-runner/internal/skillpack/flow-pack/common/kill-review/SKILL.md) (cùng version; sửa một nơi thì copy sang nơi kia)
- Related Documents: [SS-15 Agent Review Loop Until Clean](../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md), [SS-13 AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [CP-51 Durable Turn Dispatch §10](../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [BUG-288](../09-BugFix/done/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [KR-001 CP-43](../reviews/KR-001-cp43-change-contract-plan.md)
- Replaces: `None`
- Tags: `review, kill-review, closed-claim, dod, termination, anti-loop, process, matrix, protocol`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- **Kill-Review** là quy trình review **thiết kế để chấm dứt (kill)**: đóng băng **claim**, chỉ audit trong claim đó, xuất **một inventory finding**, rồi dừng theo luật rõ.
- Skill agent **không** require đọc file này: toàn bộ luật operational nằm trong `kill-review/SKILL.md` (đồng bộ `.agents` ↔ skillpack).
- Là thuốc giải cho **vòng review mở** (BUG-288 ~20 vòng; CP-51 recovery/ownership nhiều chục vòng): mỗi lần vá một finding cục bộ, reviewer tìm cạnh race kế tiếp của **cùng** protocol.
- **DoD xanh = claim được chứng minh**, không phải “cả hệ thống hết bug”. Bề mặt code luôn lớn hơn probes; residual risk phải ghi ra, không chối.
- Với **protocol** (dispatch, recovery, lease, attach, cancel, crash): claim **bắt buộc** gắn **matrix state × event đóng** + linearization cho mọi external action — cấm “fix Step N rồi review Step N+1”.

### Current Ask

- Dùng contract này mỗi khi review cần **verdict kết thúc được** (plan CP/Task, impl, residual, pre-merge) mà không trôi thành vòng vô hạn.

### Key Decisions

- `D-1` Mọi Kill-Review bắt đầu bằng **Claim một câu** + **artifact in-scope** + **bảng OOS** trước khi ghi finding.
- `D-2` Finding chỉ hợp lệ nếu **nằm trong claim đóng** (hoặc chứng minh claim sai/không an toàn). Còn lại = OOS / work item mới — **không** phải “Vòng N”.
- `D-3` Review **kill** khi: inventory đóng, mọi finding in-claim có ID+evidence+owner, không còn “nghi ngờ không evidence”, và luật §6 kích hoạt.
- `D-4` Bug thật sau kill **không** mở lại review cũ thành Vòng N+1. Phân loại: **lỗ claim** (thiếu probe) vs **ngoài claim** (backlog mới).
- `D-5` DoD = **probe có verifier** (test tên, lệnh, grep assert), không checkbox văn xuôi “works well”.
- `D-6` **Protocol claim** (recovery/lease/attach/…): probes **phải** là matrix đóng (hoặc trỏ artifact matrix đã freeze). Finding “có race A→B” mà không map cell → **mở rộng matrix** (Claim-Revision), **cấm** vá cạnh đơn lẻ rồi review tiếp.

### Constraints

- Không thay SS-15 (vòng review-until-clean sản phẩm); **siết cách định nghĩa “clean”**.
- Không claim cover hết code; bắt buộc honest residual risk.
- Sau inventory: tối đa **một fix batch** + **một delta re-review** cùng claim id.

### Open Questions

- `Q-1` Có promote SP-05 lên SS (policy operator) sau không? Mặc định: principle + skill cho đến khi UI product cần.

### Source Refs

- CP-51 §10 (ledger đóng, coverage boundary, termination).
- BUG-288 đóng cấu trúc qua SD-24/CP-51 (matrix vs vòng ad-hoc).
- Case study: vá finding cục bộ lặp lại trên protocol recovery (xem §12).

---

## 1. Mục tiêu

Định nghĩa **review phải chạy thế nào** để:

1. **Chấm dứt được** với verdict rõ.
2. “Xong” **trung thực** về những gì đã chứng minh.
3. Finding sau kill đi **ingress có kiểm soát**, không re-litigate vô hạn.
4. Human và AI dùng **cùng template** (kèm BAD/GOOD practice cho AI — §11).

## 2. Vấn đề cần giải

| Chế độ hỏng | Hệ quả | Kill-Review đáp |
| --- | --- | --- |
| Acceptance set mở | Mỗi vòng invent tiêu chí mới | Freeze claim trước finding |
| Done-by-inspection | “Nhìn ổn” không verifier | Mỗi DoD row có probe |
| Vá finding cục bộ (protocol) | Reviewer tìm cạnh kế cùng SM | Matrix-first (§7) |
| Scope phình | Dispatch + context + UI một review | Một claim / pass + OOS |
| Green = “hệ thống an toàn” | Bán rẻ omniscience | Coverage boundary bắt buộc |
| Round inflation | Finding → Vòng 21 không đổi model | Fix batch hoặc Claim-Revision |

## 3. Định nghĩa

| Thuật ngữ | Nghĩa |
| --- | --- |
| **Claim** | Một câu khẳng định có thể đúng/sai mà review này chứng minh hoặc bác. |
| **Probe** | Kiểm tra pass/fail (test, lệnh, assert cấu trúc). |
| **DoD / ledger** | Tập probe = “claim thỏa”. |
| **Finding** | Lỗi/gap có evidence **trong claim**. |
| **OOS** | Ngoài scope review này. |
| **Kill** | Chấm dứt pass theo §6 — không phải “hết bug vũ trụ”. |
| **Delta re-review** | Sau fix batch: chỉ re-check ID đang open + probe non-regression. |
| **Lỗ claim (claim hole)** | Bug trong claim đã từng green → probe yếu/thiếu. |
| **Ngoài claim** | Bug class chưa từng nằm trong claim → backlog / KR mới. |
| **Matrix đóng** | Bảng state × event/fault × outcome đã đủ cell (không TBD), mỗi external action có linearization tx. |
| **Linearization transaction** | Điểm CAS/commit durable chốt thứ tự giữa hai side-effect (vd. Stop vs send, lease claim vs attach). |

## 4. Học thuyết (bắt buộc)

### 4.1 Claim ≠ hệ thống sạch

```
Claim thỏa  ⊆  Lớp lỗi đã cover  ⊂  Bề mặt code  ⊂  Thực tế production
```

- DoD xanh ⇒ probes của claim pass.
- DoD xanh ⇏ mọi path an toàn / không còn bug tương lai.

### 4.2 Không gian đóng > soi vô hạn

Ưu tiên mô hình **đếm được**: state × barrier × fault × backend; hoặc AC × failure mode; hoặc API × mã lỗi.

Không có mô hình tự nhiên thì **dựng**:

1. Liệt kê artifact của claim.
2. Mỗi artifact: happy + ≥1 failure mode có tên.
3. Red-team timebox → freeze.

### 4.3 Evidence > nghi ngờ

- Finding phải có: **vị trí** (mục doc / `file:line` / symbol) **hoặc** thiếu verifier mà claim đòi.
- “Cảm giác còn bug” → **residual risk**, không phải finding.

### 4.4 Một batch, rồi delta

Sau freeze inventory: fix C (+ I đã chốt) **một batch** → delta only.  
Class defect mới khi fix → **một** row mới + user ack, hoặc OOS — không soi mở.

### 4.5 Protocol: đóng mô hình trước, code sau

Với recovery / lease / attach / cancel / crash / token callback:

```
SAI:  find edge_i → patch → review → find edge_{i+1} → … (Vòng 20+)
ĐÚNG: freeze plan
      → viết matrix M đóng kín
      → mọi external action có linearization tx
      → sync code guide một lần theo M
      → implement / align
      → MỘT audit cuối: cell xanh | lỗ trong M (mở M một lần, không Vòng 28)
```

**Metric đúng:** số cell TBD trong M → 0.  
**Metric sai:** số finding đã “fix” trong khi đường kính protocol không giảm.

## 5. Quy trình Kill-Review (thứ tự)

### Phase 0 — Intake (chưa finding)

| Trường | Bắt buộc | Ví dụ |
| --- | --- | --- |
| `Subject` | có | `CP-43` / `CP-51 recovery` / diff |
| `Mode` | có | `plan` \| `impl` \| `residual` \| `pre-merge` \| `protocol-matrix` |
| `Timebox` | có | single pass / N giờ |
| `Fix policy` | có | `no` \| `critical-batch` \| `all-in-claim` |
| `Loại subject` | có | `doc-feature` \| `protocol` (nếu protocol → §7) |

Subject mơ hồ → hỏi user (SP-04), **không** fishing scan.

### Phase 1 — Freeze Claim

```markdown
## Claim (đóng băng)
Một câu: …

## Artifact in-scope
- …

## Lớp lỗi in-scope (không gian đóng)
- …

## OOS (không sinh finding ở đây)
| ID | Concern | Track ở đâu |
| --- | --- | --- |

## Coverage boundary
Green nghĩa là: …
Green KHÔNG nghĩa là: …
```

Protocol → Claim **phải** trỏ matrix artifact (hoặc nhúng matrix đủ cell). Không matrix đóng → verdict `KILL_BLOCKED` hoặc `ABORT_RECLAIM`, **không** inventory race ad-hoc.

### Phase 2 — Probes (DoD của review)

| Probe ID | Yêu cầu (binary) | Verifier | Status |
| --- | --- | --- | --- |
| `P-01` | … | test / cmd / assert | ☐/✅/❌ |

**Protocol:** mỗi cell matrix (hoặc cohort cell) = ≥1 probe, hoặc một suite matrix có tên.

Meta-check: còn failure mode **trong claim** chưa có probe? → thêm probe hoặc thu hẹp claim → freeze.

### Phase 3 — Audit → inventory

| ID | Sev | Kind | Evidence | Link in-claim | Owner | Status |
| --- | --- | --- | --- | --- | --- | --- |

**Sev:** C = claim vỡ / mất dữ liệu / safety; I = claim yếu / path sai user-visible; M = polish.

**Kind:** `plan` | `code` | `test-gap` | `doc-contradiction` | `integration-risk` | `matrix-hole` (thiếu/mơ hồ cell).

### Phase 4 — Freeze inventory & kill

| Verdict | Nghĩa |
| --- | --- |
| `KILL_CLEAN` | Probes ✅; không C/I open (hoặc I được accept risk có chữ) |
| `KILL_WITH_FINDINGS` | Inventory đóng; item có owner; **ngừng review** đến fix batch / Claim-Revision |
| `KILL_BLOCKED` | Không đánh giá được (thiếu matrix, P-0, env…) |
| `ABORT_RECLAIM` | Claim sai/quá rộng; viết claim mới (id mới), không “Vòng 2” id cũ |

### Phase 5 — Fix batch tùy chọn + delta

Chỉ khi intake cho phép. Delta: ID open + non-regression. Không re-scan mở.

### Phase 6 — Ghi report

`requirements/reviews/KR-NNN-<slug>.md` — skeleton §10.

## 6. Luật chấm dứt (kill)

Review **phải dừng** khi đủ:

1. Claim đã freeze (không nới lỏng lén).
2. Probe set đã freeze.
3. Inventory đóng (ID, sev, kind, evidence, owner).
4. Timebox hết **hoặc** không còn evidence in-claim mới trong cửa sổ scan cuối.
5. Có một verdict §5 Phase 4.

### Ingress sau kill

| Case | Hành động |
| --- | --- |
| A. Ngoài claim | BUG/Task/KR **mới**. KR cũ vẫn killed. |
| B. Trong claim, probe có nhưng prod fail | **Lỗ claim** → fix + cứng probe; không “Vòng 21”. |
| C. Trong claim, không probe cover | Lỗ claim + **bắt buộc** thêm probe trước khi re-green. |
| D. Chứng minh coverage boundary dối | Sửa boundary + probes; Claim-Revision. |

**Cấm:** mở lại cùng review id thành chuỗi vô hạn không Claim-Revision.

### Hard cap mặc định

| Cap | Mặc định |
| --- | --- |
| Full pass cùng claim id | **1** + tối đa **1 delta** sau fix batch |
| Nới claim giữa pass | **0** (cần user + Claim-Revision) |
| Finding không evidence lúc freeze | **0** |

## 7. Kill-Review protocol (matrix-mandatory)

Áp khi subject chạm: turn dispatch, recovery, lease, attach, reconcile, cancel/Stop linearize, token/callback ownership, crash/restart dual-writer.

### 7.1 Trước khi code / trước khi inventory race

Freeze plan. Viết **một** closure spec / matrix:

| Trục | Ví dụ (min) |
| --- | --- |
| **States** | prepared, send_claimed, send_started, provider_accepted, terminal_*, uncertain, (+ settle phase nếu dính) |
| **Events / faults** | send, cancel/Stop, unknown, terminal proof, attach, lease takeover, token callback, crash, outage, stale write |
| **Mỗi cell** | next state + **CAS/tx linearization nào** + ai giữ lease + có clear intent? + có được gọi provider? |
| **External actions** | bảng riêng: action → linearization transaction (bắt buộc 1-1) |

Matrix **đóng** khi: không cell `TBD` / `depends`; mọi external action có tx; cấm edge lạ ngoài bảng.

### 7.2 Finding hợp lệ trên protocol

| Hợp lệ | Không hợp lệ |
| --- | --- |
| Cell thiếu / mơ hồ (`matrix-hole`) | “Có race nếu làm A rồi B” **không** map cell |
| Code ≠ outcome cell đã freeze | Vá một cạnh rồi yêu cầu review vòng mới |
| Hai cell mâu thuẫn nhau | Mở rộng scope sang transcript/UI “tiện tay” |
| Thiếu linearization cho external action đã liệt kê | “Fix Step 27” khi M còn TBD |

### 7.3 Thứ tự đúng sau khi M đóng

1. Sync **code guide / task** một lần theo M.  
2. Implement / align.  
3. **Một** audit: claim = “code + guide implement M”; finding = cell miss hoặc code≠M.  
4. Cell mới từ audit → **mở rộng M một lần** (Claim-Revision), re-audit — không chuỗi vá cạnh.

### 7.4 Liên hệ CP-51 §10

CP-51 §10 là **mẫu mạnh** của claim đóng (ledger + crash-matrix + coverage boundary).  
SP-05 §7 bắt buộc **cùng tinh thần** cho mọi slice protocol (kể recovery ownership chưa nằm đủ trong matrix cũ).

## 8. “Probes đủ chưa?” — trả lời trung thực

**Không** biết đủ cho mọi bug tương lai.

**Biết đủ cho claim** khi:

1. Mọi động từ trong Claim map ≥1 probe.  
2. Mọi failure class (hoặc mọi cell M) map ≥1 probe.  
3. Sau red-team timebox không nêu thêm failure **in-claim** có evidence.  
4. Có bảng OOS.  
5. Residual risk nêu phần chưa probe.

DoD thiếu → vẫn có bug: nếu **trong claim** = process fail (lỗ claim); nếu **ngoài** = process OK, work mới.

## 9. Residual risk (bắt buộc trong report)

```markdown
## Residual risk (chưa chứng minh)
- …
## Claim tiếp theo gợi ý (không phải review này)
- …
```

## 10. Artifact report

### Path

```
requirements/reviews/KR-NNN-<subject-slug>.md
```

Tăng `NNN` theo file hiện có.

### Skeleton

```markdown
# KR-NNN: Kill-Review — <subject>

## Metadata
- Review ID, Subject, Mode, Claim-Revision, Reviewer, Date, Timebox, Fix policy, Verdict

## 1. Claim (đóng băng)
## 2. Artifact in-scope
## 3. Lớp lỗi / matrix (nếu protocol: link hoặc nhúng M)
## 4. OOS
## 5. Coverage boundary (green nghĩa là / không nghĩa là)
## 6. Probes
## 7. Inventory findings
## 8. Verdict + lý do stop
## 9. Residual risk
## 10. Claim tiếp theo (optional)
```

## 11. BAD practice vs GOOD practice (cho AI và human)

> Mục này **bắt buộc** agent đọc trước khi chạy kill-review hoặc “review rồi fix”.  
> Vi phạm BAD practice = **không tuân SP-05**, dù output dài và “có vẻ kỹ”.

### 11.1 BAD practice (cấm / từ chối)

| ID | BAD | Vì sao chết |
| --- | --- | --- |
| B-1 | Review “đến khi hết issue” **không** claim đóng | Acceptance mở → không kill được |
| B-2 | Ghi finding **trước** khi freeze Claim + OOS + boundary | Scope creep ngay từ dòng đầu |
| B-3 | Finding không evidence (“có thể race”, “nên coi lại”) | Inventory rác; không actionable |
| B-4 | Protocol: **vá một finding → review vòng mới** lặp lại | Lặp BUG-288 / 20+ vòng recovery |
| B-5 | Protocol inventory khi matrix còn cell TBD | Soi cạnh thay vì đóng mô hình |
| B-6 | Nới claim giữa pass vì “liên quan” | Phá freeze; thành mega-review |
| B-7 | Coi DoD/KR xanh = “hệ thống an toàn / hết bug” | Dối coverage boundary |
| B-8 | Mở lại KR-NNN thành Vòng 2…N không Claim-Revision | Vô hiệu luật kill |
| B-9 | Một claim ôm dispatch + context harness + UI + graph | Không đóng được không gian lỗi |
| B-10 | DoD checkbox văn xuôi không verifier | Done-by-inspection |
| B-11 | Tự implement fix khi Fix policy = `no` | Trộn review với coding không kiểm soát |
| B-12 | “Continue reviewing” = full re-scan cùng claim | Phải là delta hoặc reclaim |
| B-13 | Metric tiến độ = số finding đã fix (protocol) | Đường kính protocol có thể không giảm |
| B-14 | Im lặng bỏ OOS / residual risk | Bán rẻ green |
| B-15 | Dùng SS-15 “until clean” như cớ không đóng claim | Clean không định nghĩa = loop |

### 11.2 GOOD practice (bắt buộc / khuyến khích)

| ID | GOOD | Hiệu quả |
| --- | --- | --- |
| G-1 | Freeze Claim + OOS + “green nghĩa là / không” **trước** finding | Review kill được |
| G-2 | Probe binary + verifier có tên | Chứng minh được, không “cảm giác” |
| G-3 | Inventory một pass; severity C/I/M; owner rõ | Fix batch có thứ tự |
| G-4 | Default Fix policy `no` trừ user bảo fix | Tách audit / implement |
| G-5 | Sau fix: **delta only** cùng claim | Không re-litigate |
| G-6 | Protocol: **matrix đóng + linearization tx** trước code guide sync | Chặn vòng 20+ |
| G-7 | Finding protocol map **cell** hoặc `matrix-hole` | M mở rộng có chủ đích |
| G-8 | Metric protocol = **cell TBD → 0** | Tiến độ thật |
| G-9 | Bug sau kill: lỗ claim vs ngoài claim | Ingress sạch |
| G-10 | Residual risk + next claims luôn có | Trung thực |
| G-11 | Timebox red-team rồi **freeze** probes | Không soi vô hạn |
| G-12 | Tách KR: plan doc / impl runtime / protocol matrix | Claim đúng class |
| G-13 | Cite SP-05 + path `KR-*` trong output user | Traceable |
| G-14 | User prompt mở (“hết bug Flow Mode”) → đề xuất 1–3 claim đóng, từ chối open set | SP-05 force |
| G-15 | Conflict với thói quen skill khác về termination → **SP-05 thắng** | Một luật stop |

### 11.3 Bảng nhanh AI (in vào system hành vi)

```
TRƯỚC KHI LIST BUG:
  [ ] Claim một câu đã viết?
  [ ] OOS + coverage boundary đã viết?
  [ ] Nếu protocol: matrix đóng (không TBD) hoặc KILL_BLOCKED?

KHI LIST BUG:
  [ ] Mỗi finding có evidence?
  [ ] Mỗi finding map probe/cell?
  [ ] Không nới claim?

KHI XONG PASS:
  [ ] Verdict kill rõ?
  [ ] Residual risk?
  [ ] Không tự hứa “hết bug hệ thống”?
  [ ] Không hẹn “Vòng review tiếp” cùng claim không revision?
```

## 12. Case study — vòng lặp finding cục bộ (học từ thực tế)

### 12.1 Hiện tượng

- **BUG-288:** nhiều vòng fix gate/Stop/crash; mỗi vòng vá cửa sổ; reviewer (Codex) tìm residual cùng seam → đóng **cấu trúc** bằng SD-24/CP-51 + ledger §10, không phải Vòng 21 ad-hoc.
- **CP-51 / recovery ownership:** lặp lại pattern — “fix Step N, review Step N+1”; mỗi finding là cạnh race của **cùng** protocol (send/cancel/unknown/terminal/attach/lease/token/crash), không phải 27 bug độc lập.
- **Không phải** chỉ “model yếu”: **cách xử lý finding cục bộ** khi chưa đóng mô hình là lỗi process chính.

### 12.2 Sai lầm gốc

| Sai | Đúng |
| --- | --- |
| Xử lý từng finding như unit độc lập | Coi là symptom của **cell chưa có tên** trong matrix |
| Plan/code guide lệch theo từng vòng vá | Freeze plan → đóng M → sync guide **một lần** |
| Review sau mỗi patch nhỏ | **Một** audit sau M đóng |
| Nghĩ thêm vòng review sẽ hội tụ | Hội tụ chỉ khi **M đóng**; không thì đường kính giữ nguyên |

### 12.3 Cách đúng (áp CP-51 recovery ngay)

1. **Freeze** implement + review-từng-step.  
2. Viết **một** closure spec recovery ownership: matrix state × event (send, cancel, unknown, terminal, attach, lease takeover, token callback, crash, …).  
3. Mọi external action → **linearization transaction** tương ứng.  
4. Chỉ khi M **đóng kín** → đồng bộ code guide → implement/align.  
5. **Một** audit cuối (Kill-Review `protocol-matrix` / `impl` trên claim “code implement M”).

### 12.4 Bài học đóng thành rule

- Đã ghi ở `D-6`, §4.5, §7, BAD B-4/B-5/B-13, GOOD G-6/G-7/G-8.
- KR doc/plan (vd. KR-001 CP-43) **khác class**: inventory doc OK; **không** suy ra protocol cũng vá từng F-xx rồi xong.

## 13. Map sang artifact FlowPilot

| Artifact | Vai trò |
| --- | --- |
| CP/Task DoD | Ứng viên probe — chỉ promote row có verifier |
| CP-51 §10 | Mẫu claim đóng mạnh (matrix + ledger) |
| BUG-288 multi-round | Phản ví dụ acceptance mở |
| SS-15 | Loop product; mỗi “clean” cần claim Kill-Review |
| Skill `kill-review` | Thực thi SP-05; §11 BAD/GOOD là force rule |
| Skill `code-review` / single-agent loop | Bọc bằng kill-review khi cần termination |

## 14. DoD adopt SP-05

- [x] Skill `kill-review` **tự chứa**, cùng version ở `.agents/skills` và `skillpack/flow-pack/common` (không mention SP-05 trong skill).
- [x] Có `requirements/reviews/` + KR mẫu (KR-001).
- [x] §11 BAD/GOOD + §12 case study + §7 protocol matrix (principle nội bộ FlowPilot).
- [ ] Team thống nhất: green KR không market “hết bug khu vực subject”.

---

## Phụ lục A — Thẻ 1 trang

```
1. VIẾT CLAIM + OOS + green nghĩa là / không
2. (Protocol?) MATRIX ĐÓNG + linearization tx — không thì BLOCKED
3. PROBES (binary + verifier)
4. TIMEBOX RED-TEAM → FREEZE
5. INVENTORY (evidence hoặc bỏ)
6. VERDICT: CLEAN | WITH_FINDINGS | BLOCKED | RECLAIM
7. DỪNG. Fix batch? → DELTA only
8. Bug sau: lỗ claim vs ngoài claim
```

## Phụ lục B — DoD chỉ là target

Probes = điểm ta **chọn** chứng minh. Code có nhiều part hơn probes.

| Nếu… | Thì… |
| --- | --- |
| Bug part chưa probe **ngoài claim** | Process OK → claim mới |
| Bug part chưa probe **trong claim** | Nợ process (lỗ claim) → thêm probe |
| Bug part đã probe mà vẫn green sai | Hỏng verifier → sửa test |

Kill-Review không xóa (2)/(3); nó **đặt tên** để không giả (1) là “review chưa kill được”.
