---
name: kill-review
description: |
  Chạy Kill-Review — review thiết kế để chấm dứt được: freeze claim đóng, probes có verifier,
  một inventory finding, luật stop rõ. Protocol (recovery/lease/attach/cancel/crash) bắt buộc
  matrix state×event đóng trước khi vá race. Dùng khi user gọi kill-review, KR, closed-claim,
  audit plan/impl không vòng vô hạn, hoặc tránh loop “vá finding → review → finding kế”.
version: 6
---

# kill-review

Skill **tự chứa** (portable). Không phụ thuộc file principle/spec của một repo cụ thể.  
Mọi luật cần thiết nằm trong file này. Xung đột với skill review khác về **cách chấm dứt** → **skill này thắng**.

---

## Mục tiêu

1. Review **chấm dứt được** với verdict rõ.
2. “Xong / green” **trung thực**: chỉ claim đã freeze được chứng minh — không phải cả hệ thống hết bug.
3. Finding sau khi kill đi **ingress có kiểm soát** (lỗ claim vs ngoài claim), không “Vòng N” vô hạn.
4. Protocol: **đóng mô hình (matrix) trước**, không vá từng cạnh race.

---

## Khi nào dùng / không dùng

**Dùng khi:**

- User: kill-review, KR, closed-claim, “list bug một lần rồi stop”.
- Cần audit plan / impl / residual / pre-merge với **verdict kết thúc**.
- Subject protocol: dispatch, recovery, lease, attach, reconcile, cancel/Stop, token/callback, crash — mode `protocol-matrix`.

**Không dùng khi:**

- Brainstorm không subject.
- “Tìm hết bug product” mà user **không** chịu thu hẹp thành claim đóng.
- Chỉ implement, không yêu cầu review.

---

## Định nghĩa ngắn

| Thuật ngữ                     | Nghĩa                                                                                           |
| ----------------------------- | ----------------------------------------------------------------------------------------------- |
| **Claim**                     | Một câu khẳng định có thể đúng/sai mà pass này chứng minh hoặc bác.                             |
| **Probe**                     | Kiểm tra binary pass/fail (test tên, lệnh, assert cấu trúc).                                    |
| **Finding**                   | Lỗi/gap **có evidence**, nằm trong claim.                                                       |
| **OOS**                       | Ngoài scope — không sinh finding ở pass này.                                                    |
| **Kill**                      | Chấm dứt pass với verdict — không nghĩa “hết bug vũ trụ”.                                       |
| **Delta**                     | Sau fix batch: chỉ re-check finding ID đang open + probe non-regression.                        |
| **Lỗ claim**                  | Bug thuộc claim đã từng green → probe thiếu/yếu.                                                |
| **Ngoài claim**               | Bug class chưa nằm trong claim → backlog / KR mới.                                              |
| **Matrix đóng**               | Bảng state × event/fault × outcome **không cell TBD**; mọi external action có linearization tx. |
| **Linearization transaction** | CAS/commit durable chốt thứ tự giữa hai side-effect (vd. Stop vs send).                         |

**Công thức trung thực:**

```
Claim thỏa  ⊆  Lớp lỗi đã cover  ⊂  Bề mặt code  ⊂  Production
```

---

## BAD practice vs GOOD practice (bắt buộc cho AI)

### BAD (cấm)

| ID   | Không được làm                                                      |
| ---- | ------------------------------------------------------------------- |
| B-1  | Review “đến hết issue” / until clean **không** claim đóng           |
| B-2  | List finding **trước** Claim + OOS + coverage boundary              |
| B-3  | Finding không evidence (“có thể race”, “nên xem lại”)               |
| B-4  | Protocol: vá 1 finding → review vòng mới → lặp nhiều vòng           |
| B-5  | Inventory race khi matrix còn cell TBD                              |
| B-6  | Nới claim giữa pass vì “liên quan”                                  |
| B-7  | Nói green KR = “hệ thống an toàn / hết bug”                         |
| B-8  | Mở lại cùng review id thành Vòng 2…N không Claim-Revision           |
| B-9  | Một claim ôm nhiều seam không liên quan (protocol+UI+docs+…)        |
| B-10 | DoD/checkbox văn xuôi không verifier                                |
| B-11 | Tự implement khi Fix policy = `no`                                  |
| B-12 | “Review tiếp” = full re-scan cùng claim (phải delta hoặc reclaim)   |
| B-13 | Đo tiến độ protocol bằng “số finding đã fix” thay vì “cell TBD → 0” |
| B-14 | Bỏ OOS hoặc residual risk                                           |
| B-15 | Dùng vòng review product “until clean” làm cớ không đóng claim      |

### GOOD (bắt buộc / nên)

| ID   | Phải làm                                                                   |
| ---- | -------------------------------------------------------------------------- |
| G-1  | Freeze Claim + OOS + “green nghĩa là / không” **trước** mọi finding        |
| G-2  | Probe binary + verifier có tên                                             |
| G-3  | Một inventory; severity C/I/M; owner rõ                                    |
| G-4  | Default Fix policy `no` trừ user bảo fix                                   |
| G-5  | Sau fix: **delta only** cùng claim                                         |
| G-6  | Protocol: matrix đóng + linearization tx **trước** sync guide / audit race |
| G-7  | Finding protocol map **cell** matrix hoặc kind `matrix-hole`               |
| G-8  | Metric protocol: **số cell TBD → 0**                                       |
| G-9  | Bug sau kill: phân **lỗ claim** vs **ngoài claim**                         |
| G-10 | Mọi report có residual risk + gợi ý claim sau (nếu có)                     |
| G-11 | Red-team timebox rồi **freeze** probes                                     |
| G-12 | Tách KR theo class: plan / impl / protocol-matrix                          |
| G-13 | Output user: path report (nếu ghi file) + verdict + boundary + đếm sev     |
| G-14 | Prompt mở → đề xuất 1–3 claim đóng; từ chối open set                       |
| G-15 | Luật chấm dứt của skill này **thắng** skill review khác khi conflict       |

### Checklist trước khi trả user

```
[ ] Claim + OOS + boundary đã freeze?
[ ] Protocol? → matrix đóng hoặc verdict KILL_BLOCKED?
[ ] Mọi finding có evidence + map probe/cell?
[ ] Verdict kill rõ? Residual risk?
[ ] Không hứa “hết bug hệ thống”? Không hẹn Vòng N cùng claim?
[ ] Fix policy no → chưa implement?
```

---

## Workflow (đúng thứ tự)

### 0. Intake

Ghi nhận:

| Trường       | Bắt buộc | Giá trị                                                            |
| ------------ | -------- | ------------------------------------------------------------------ |
| Subject      | có       | CP/Task/path/diff/mô tả                                            |
| Mode         | có       | `plan` \| `impl` \| `residual` \| `pre-merge` \| `protocol-matrix` |
| Timebox      | có       | single pass / N giờ                                                |
| Fix policy   | có       | default **`no`** \| `critical-batch` \| `all-in-claim`             |
| Loại subject | có       | `doc-feature` \| `protocol`                                        |

Claim không suy ra được → hỏi user **một lần** (vài option claim), **không** fishing scan.

### 1. Freeze claim

Viết (trong report hoặc đầu output):

```markdown
## Claim (đóng băng)

Một câu: …

## Artifact in-scope

- …

## Lớp lỗi in-scope (không gian đóng)

- … # protocol: state × event matrix — xem mục Protocol

## OOS

| ID  | Concern | Track / ghi chú |
| --- | ------- | --------------- |

## Coverage boundary

- Green nghĩa là: …
- Green KHÔNG nghĩa là: …
```

**Protocol:** matrix chưa đóng (còn TBD) → verdict `KILL_BLOCKED` hoặc `ABORT_RECLAIM` — **cấm** list race ad-hoc.

### 2. Probes

| Probe ID | Yêu cầu (binary) | Verifier             | Status  |
| -------- | ---------------- | -------------------- | ------- |
| P-01     | …                | test / lệnh / assert | ☐/✅/❌ |

Meta-check: còn failure mode **trong claim** chưa có probe? → thêm probe hoặc thu hẹp claim → freeze.

### 3. Inventory findings

Chỉ in-scope.

| ID  | Sev | Kind | Evidence | Link in-claim | Owner | Status |
| --- | --- | ---- | -------- | ------------- | ----- | ------ |

- **Sev:** `C` claim vỡ / mất dữ liệu / safety · `I` claim yếu / sai user-visible · `M` polish
- **Kind:** `plan` \| `code` \| `test-gap` \| `doc-contradiction` \| `integration-risk` \| `matrix-hole`
- Không evidence → không phải finding (đưa residual risk nếu cần)

### 4. Kill — verdict

| Verdict              | Khi                                                                            |
| -------------------- | ------------------------------------------------------------------------------ |
| `KILL_CLEAN`         | Mọi probe ✅; không C/I open (hoặc I được user accept risk có chữ)             |
| `KILL_WITH_FINDINGS` | Inventory đóng; item có owner; **ngừng review** đến fix batch / Claim-Revision |
| `KILL_BLOCKED`       | Không đánh giá được (thiếu matrix, thiếu env, thiếu input…)                    |
| `ABORT_RECLAIM`      | Claim sai/quá rộng → viết claim mới (id mới), **không** “Vòng 2” id cũ         |

### 5. Fix batch + delta (optional)

Chỉ khi Fix policy ≠ `no`.  
Fix C (+ I đã chốt) **một batch** → re-review **delta only**.  
Class defect mới khi fix → một row mới + user ack, hoặc OOS — không full re-scan.

### 6. Ghi report

**Ưu tiên (nếu repo có cấu trúc tương thích):**

```text
requirements/reviews/KR-NNN-<subject-slug>.md
```

Tăng `NNN` theo file hiện có; tạo thư mục nếu thiếu.

**Nếu repo không có `requirements/reviews/`:** ghi report đầy đủ trong chat, hoặc path user chỉ định. Không bịa dependency file principle bên ngoài.

Skeleton report:

```markdown
# KR-NNN: Kill-Review — <subject>

## Metadata

- Review ID, Subject, Mode, Claim-Revision, Reviewer, Date, Timebox, Fix policy, Verdict

## 1. Claim (đóng băng)

## 2. Artifact in-scope

## 3. Lớp lỗi / matrix (protocol: nhúng hoặc mô tả M)

## 4. OOS

## 5. Coverage boundary

## 6. Probes

## 7. Inventory findings

## 8. Verdict + lý do stop

## 9. Residual risk

## 10. Claim tiếp theo (optional)
```

---

## Protocol (matrix-mandatory)

Áp khi subject chạm: turn dispatch, recovery, lease, attach, reconcile, cancel/Stop, token/callback ownership, crash/restart, dual-writer.

### Thứ tự đúng

```text
ĐÚNG:
  freeze plan
  → viết matrix M (state × event × outcome) đóng kín
  → mọi external action có linearization transaction
  → sync code/guide MỘT LẦN theo M
  → implement / align
  → MỘT audit: code/guide == M  (finding = cell miss hoặc code≠M)

SAI:
  find edge_i → patch → review → find edge_{i+1} → … (vòng 20+)
```

### Matrix tối thiểu

| Trục             | Nội dung                                                                                                                         |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| States           | Các state durable forward-only của protocol                                                                                      |
| Events/faults    | send, cancel/Stop, unknown, terminal proof, attach, lease takeover, token callback, crash, outage, stale write, … (theo subject) |
| Mỗi cell         | next state + CAS/tx nào + ai giữ lease + clear intent? + được gọi provider/external?                                             |
| External actions | bảng 1-1: action → linearization transaction                                                                                     |

**Đóng** = không cell `TBD` / `depends`; không edge lạ ngoài bảng.

### Finding protocol hợp lệ / không

| Hợp lệ                                    | Không hợp lệ                        |
| ----------------------------------------- | ----------------------------------- |
| Cell thiếu / mơ hồ (`matrix-hole`)        | “Có race A→B” không map cell        |
| Code ≠ outcome cell đã freeze             | Vá một cạnh rồi đòi review vòng mới |
| Hai cell mâu thuẫn                        | Nới sang UI/transcript “tiện tay”   |
| Thiếu linearization cho action đã liệt kê | “Fix step N” khi M còn TBD          |

**Metric đúng:** cell TBD → 0.  
**Metric sai:** số finding đã fix trong khi đường kính protocol không giảm.

### Case study (generic)

Khi team vá từng race/Stop/crash/lease trên **cùng** protocol qua nhiều vòng review: thường **không** phải chỉ model yếu — lỗi chính là **xử lý finding cục bộ** thay vì đóng toàn bộ mô hình (matrix + linearization) ngay từ đầu. Reviewer sẽ luôn tìm cạnh kế của cùng state machine. Cách thoát: freeze → đóng M → sync guide một lần → một audit cuối.

---

## Luật chấm dứt & ingress

Pass **phải dừng** khi: claim freeze + probes freeze + inventory đóng (hoặc blocked) + có verdict + timebox/scan cuối xong.

**Hard cap mặc định:** 1 full pass + tối đa 1 delta cùng claim id. Thêm full pass → Claim-Revision (id mới hoặc revision+1 có chủ đích).

**Sau kill, bug mới:**

| Case                                 | Hành động                                     |
| ------------------------------------ | --------------------------------------------- |
| Ngoài claim                          | Work/KR mới; review cũ vẫn killed             |
| Trong claim, probe đã có mà vẫn fail | Lỗ claim → fix + cứng probe                   |
| Trong claim, không probe cover       | Lỗ claim + bắt buộc thêm probe trước re-green |
| Coverage boundary dối                | Sửa boundary + Claim-Revision                 |

**Cấm:** chuỗi “Vòng N” trên cùng id không revision.

---

## Output cho user

1. Path report (nếu ghi file) hoặc full report trong chat
2. Verdict + một dòng lý do stop
3. Đếm C / I / M open
4. Coverage boundary (2 gạch: nghĩa là / không)
5. Bảng finding tóm tắt
6. Protocol blocked → liệt kê **cell/tx còn thiếu**, không giả inventory đủ

**Không implement** trừ Fix policy ≠ `no`.

---

## Prompt mẫu

**Inventory only:**

> Kill-review subject `<tên>`. Mode `plan`. Fix policy `no`. Single pass. Freeze claim, probes, inventory; ghi report; stop.

**Protocol:**

> Kill-review `<protocol subject>`. Mode `protocol-matrix`. Nếu matrix chưa đóng → KILL_BLOCKED và liệt kê cell TBD; không vá từng race. Fix policy `no`.

**Delta sau fix:**

> Delta kill-review trên KR-NNN / report trước. Cùng claim. Không nới scope.

**Từ chối open set:**

> “Review until clean / hết bug” không claim đóng sẽ không terminate theo kill-review. Đề xuất 1–3 claim đóng, mỗi claim một pass.

---

## Force rules

- CẤM multi-round full re-inspect cùng claim không Claim-Revision.
- CẤM bán green như an toàn toàn hệ thống.
- CẤM protocol vá-từng-finding khi matrix chưa đóng.
- BẮT BUỘC OOS + residual risk mọi report.
- BẮT BUỘC tuân bảng BAD/GOOD ở trên.
- BẮT BUỘC skill này thắng skill khác về **termination semantics**.
- BẮT BUỘC tự chứa: không yêu cầu reader mở principle/spec ngoài skill để hiểu luật kill.
