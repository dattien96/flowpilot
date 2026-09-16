# CP-65 Bản Ghi Chú Kiến Trúc: Multi-Candidate Tournament & PDR Escalation Harness

> **Dành cho Bạn (Human / Tech Lead):** Tài liệu này giải thích chi tiết lý do tại sao phương pháp thử lại tuần tự (Sequential Retry) thường thất bại trên các bài toán khó, cách thức hoạt động của "Đấu trường giải thuật" (Tournament Voting / PDR) từ DeepSeek, và cách cắm cơ chế này vào FlowPilot như một chiếc "phao cứu sinh" tự động.

---

## 1. Issue Hiện Tại Của Chúng Ta Là Gì? (The Pain Point)

Trong các flow hiện tại của FlowPilot (`task-harness`, `bug-plan-harness`, `vibe-sprint`), cơ chế sửa lỗi khi Review bị Reject hoặc Test bị Fail là **Thử Lại Tuần Tự (Sequential Retry Loop)**:

```text
Lượt 1 (Model A viết) ──► Review Reject!
  └──► Lượt 2 (Model A đọc lại lịch sử cũ, viết đè lên) ──► Review Reject!
        └──► Lượt 3 (Model A bối rối, cố sửa chắp vá) ──► Chạm trần Cap (Fail/Dừng)!
```

### 3 Điểm nghẽn chí mạng của việc Thử lại tuần tự:
1. **Bẫy định kiến (Cognitive Lock-in):** Khi một model đã nghĩ sai cách tiếp cận ở Lượt 1, việc bắt nó tự sửa ở Lượt 2 và 3 thường dẫn đến hiện tượng **"vá víu chắp vá" (patch-on-patch)**. Càng sửa thì code càng nát.
2. **Context bị ô nhiễm (Context Dilution):** Lịch sử của 3 lượt sửa thất bại trước đó chất đầy vào context window. Model bị "ngợp" bởi các lỗi sai trong quá khứ và bắt đầu mất phương hướng.
3. **Bế tắc khi chạm trần (Cap Outage):** Khi chạm giới hạn lặp (ví dụ `review_cap: 3` hoặc `vibe-owner-debate` tranh luận 5 hiệp không ngã ngũ), hệ thống chỉ có 2 lựa chọn tồi: hoặc dừng lại bắt người dùng tự làm bằng tay, hoặc nhắm mắt cho qua (leak bug).

---

## 2. Ta Apply Bài Học DeepSeek / PDR Để Làm Gì? (The Solution)

Các nghiên cứu mới nhất của DeepSeek và các top agent trên SWE-bench chỉ ra rằng:
> **"Thay vì cho 1 model nghĩ và sửa 5 lần liên tiếp, hãy cho 3 nhánh chạy song song (Parallel Rollouts) với các góc nhìn độc lập, rồi dùng Trọng tài tự động chọn ra phương án tốt nhất."**
> Cơ chế này gọi là **Parallel-Distill-Refine (PDR)**.

Chúng ta xây dựng **CP-65: Đấu Trường Giải Thuật (Multi-Candidate Tournament)**:

```text
                           [ BÀI TOÁN HÓC BÚA / BUG KHÓ ]
                                          │
                     ┌────────────────────┼────────────────────┐
                     ▼ (Nhánh 1 - Clean)  ▼ (Nhánh 2 - Clean)  ▼ (Nhánh 3 - Clean)
                   Claude                Codex                Grok
                (Chiến lược A)       (Chiến lược B)       (Chiến lược C)
                     │                    │                    │
                     ▼                    ▼                    ▼
                Ứng viên 1           Ứng viên 2           Ứng viên 3
                     │                    │                    │
                     └────────────────────┼────────────────────┘
                                          ▼
                         [ HỘI ĐỒNG TRỌNG TÀI TỰ ĐỘNG ]
                           (Runner Scoring Matrix)
                                          │
                     ├── 1. Test Suite Pass Rate (Trọng số 50%)
                     ├── 2. LSP Compiler Diagnostics Clean (Trọng số 30%)
                     ├── 3. GitNexus Blast Radius Nhỏ Nhất (Trọng số 20%)
                                          │
                                          ▼
                            [ CHỌN PHƯƠNG ÁN CHIẾN THẮNG ]
                                  (Auto-Pick Winner)
```

---

## 3. Nó Tương Thích Với Hệ Thống Hiện Tại Như Thế Nào?

Để không làm lãng phí token cho 80% các task bình thường, chúng ta cắm cơ chế Tournament vào hệ thống theo **2 mũi nhọn**:

### Mũi nhọn 1: Làm "Phao Cứu Sinh" Tự Động (Escalation Fallback Leg)
Áp dụng cho các flow sẵn có (`task-harness`, `vibe-sprint`):
- Bình thường, hệ thống vẫn chạy 1 model đơn lẻ để tiết kiệm token tối đa.
- **KHI VÀ CHỈ KHI:**
  - Review Loop lặp quá trần (`review_cap_exceeded`).
  - Hoặc `vibe-owner-debate` 2 bên tranh cãi 5 hiệp vẫn không thống nhất được verdict.
  - Hoặc Coder fix 2 lần liên tiếp vẫn làm gãy Regression test.
- $\rightarrow$ **Thay vì fail hoặc đơ máy**, Runner tự động kích hoạt **Tournament Escalation Node**: Bật 2 nhánh song song (Claude vs Codex) độc lập để giải cứu task đó!

### Mũi nhọn 2: Tạo một Flow Chuyên Dụng (`tournament-harness.yaml`)
Dành cho người dùng chủ động chọn khi biết trước đây là bài toán thuật toán khó, bug chập chờn (race condition) hoặc refactor diện rộng:
- User gõ: `flowpilot run tournament-harness`
- Flow này tự động chia nhánh đối kháng ngay từ đầu để tìm ra lời giải tối ưu nhất.

---

## 4. Bộ Trọng Tài Tự Động (Tournament Arbiter) Chấm Điểm Như Thế Nào?

Trọng tài **không dùng cảm tính của AI**, mà dùng **3 chỉ số vật lý cứng của Runner**:

$$Score = (TestPassRate \times 0.5) + (LSPScore \times 0.3) + (BlastRadiusScore \times 0.2)$$

1. **Test Suite Pass Rate (50%):** Ứng viên nào pass 100% test (cả test cũ lẫn test mới) thì đạt điểm tối đa. Ứng viên nào làm gãy test cũ bị trừ điểm nặng.
2. **LSP Diagnostics (30% - Từ CP-63):** Ứng viên nào có 0 lỗi compiler và 0 warning type mismatch sẽ ăn trọn điểm này.
3. **GitNexus Blast Radius (20%):** Ứng viên nào giải quyết được bài toán mà ít làm xáo trộn các file/module khác nhất (Blast Radius `Low` thay vì `High`) sẽ chiến thắng.

---

## 5. Tóm Tắt Giá Trị Đem Lại Cho Bạn

- **Tỷ lệ tự động hoàn thành vượt trội:** Loại bỏ triệt để tình trạng Agent bị "kẹt vô tận" (stuck loop) hoặc phải bỏ dở giữa chừng vì review reject quá 3 lần.
- **Tận dụng tối đa sức mạnh Multi-Provider:** Thay vì chỉ dùng 1 model, FlowPilot phát huy tối đa lợi thế có sẵn cả Claude, Codex, Grok cùng lúc.
- **Bảo đảm an toàn tuyệt đối:** Dù chọn model nào, phương án chiến thắng bắt buộc phải vượt qua được Test Suite và LSP mới được merge vào nhánh chính.
