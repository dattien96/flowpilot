# CP-64 Bản Ghi Chú Kiến Trúc: Reproduce-First TDD Gate (`r-reproduce`)

> **Dành cho Bạn (Human / Tech Lead):** Tài liệu này giải thích bản chất vấn đề, lý do học hỏi từ DeepSeek / SWE-bench, và cách cơ chế kiểm duyệt TDD Đỏ-Xanh (Red-Green TDD) tương thích với FlowPilot mà không làm gãy các flow tạo tính năng mới.

---

## 1. Issue Hiện Tại Của Chúng Ta Là Gì? (The Pain Point)

Hãy nhìn vào cách FlowPilot đang thực hiện TDD trong `task-harness`, `bug-harness` và `vibe-sprint` hiện nay:

### Quy trình hiện tại:
1. Node `test_signatures` chạy: Agent chỉ viết các **khung hàm rỗng (empty test signatures)**:
   ```go
   // Scenario: summing negative numbers still returns the correct total.
   func TestSum_NegativeNumbers(t *testing.T) {} // Thân hàm hoàn toàn rỗng!
   ```
2. Node `implement` chạy: Agent coder được dặn trong prompt: *"Hãy vừa sửa code production, vừa tự điền nội dung và assertion vào hàm test rỗng ở trên."*

### 3 "Lỗ hổng" chí mạng của quy trình này (Đặc biệt khi Fix Bug):
1. **Test không bao giờ ĐỎ (Never Fails):** Hàm test rỗng thì lúc nào chạy cũng XANH (PASS). Không có bất kỳ khoảnh khắc nào chứng minh được bug đang thực sự xảy ra.
2. **Coder "tự biên tự diễn" (Confirmation Bias):** Coder sửa code production xong, sau đó nó viết assertion trong test sao cho... vừa khít với đoạn code nó vừa viết. Nếu coder hiểu sai nghiệp vụ, nó sẽ viết test sai theo cách hiểu sai đó, và test vẫn Pass 100%!
3. **Không có gì bảo đảm bug đã được sửa (No Proof of Fix):** Nếu ngày mai ai đó vô tình xóa đoạn code fix bug đi, liệu test đó có ĐỎ không? Không ai biết, vì chưa bao giờ ai nhìn thấy test đó chạy ĐỎ lúc có bug!

---

## 2. Ta Apply Bài Học DeepSeek Để Làm Gì? (The Solution)

Trong các cuộc thi AI coding benchmark đỉnh cao thế giới (như SWE-bench và DeepSWE), **DeepSeek-Coder** đạt tỷ lệ giải quyết bug cực cao nhờ nguyên tắc: **Reproduce-First (Tái Hiện Lỗi Trước Bằng Test Thực Thi)**.

Chúng ta áp dụng bài học này để tạo ra một chốt chặn vật lý mới: **Cổng `r-reproduce`**.

### Mô hình hoạt động mục tiêu cho BugFix:
```text
[ Nhận Bug Report / Stacktrace ]
               │
               ▼
┌─────────────────────────────────────────────────────────────┐
│ BƯỚC 1: NODE TESTER (Viết Test Tái Hiện)                    │
│ Agent chỉ viết 1 file test MỚI với assertion đầy đủ.       │
│ TUYỆT ĐỐI KHÔNG ĐƯỢC PHÉP SỬA CODE PRODUCTION!              │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ BƯỚC 2: CỔNG KIỂM DUYỆT CỨNG (Cổng `r-reproduce` tại Runner)│
│ Runner tự động chạy test vừa viết:                          │
│                                                             │
│   ❌ NẾU TEST CHẠY XANH (PASS):                              │
│      ──► CHẶN ĐẦU NGAY: "Test của bạn không tái hiện        │
│          được lỗi! Hãy viết lại test chứng minh bug có thật"│
│                                                             │
│   ✅ NẾU TEST CHẠY ĐỎ (FAIL / ASSERTION ERROR):             │
│      ──► PASS CỔNG REPRODUCE: Bug đã được chứng minh vật lý!│
│          Mở khóa quyền ghi file production cho Coder.       │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ BƯỚC 3: NODE CODER (Sửa Code Logic)                         │
│ Coder sửa file production. BỊ CẤM SỬA LẠI FILE TEST!        │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ BƯỚC 4: VALIDATE (Chuyển Màu Test)                          │
│ Runner chạy lại test suite:                                 │
│   - Test vừa viết phải chuyển từ ĐỎ ──► XANH (GREEN).       │
│   - Các test cũ không được gãy (Regression Safety).         │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. Nó Tương Thích Với Hệ Thống Hiện Tại Như Thế Nào?

Đây là băn khoăn lớn nhất của bạn: *"Liệu áp dụng cái này có làm hỏng các flow Task mới hay Vibe flow không?"*.

### Giải pháp phân tầng tương thích hoàn hảo (Selective Applicability):

Chúng ta **không áp dụng bừa bãi** mà chia làm 2 nhánh rõ ràng dựa vào bản chất công việc:

#### Nhánh 1: Áp dụng BẮT BUỘC 100% cho BugFix (`bug-harness`, `bug-plan-harness`)
- **Vì sao phù hợp?** Code production đã có sẵn. Bạn hoàn toàn có thể gọi hàm cũ và truyền tham số lỗi để bắt test phải ĐỎ.
- **Hành vi:** Kích hoạt rule `r-reproduce`. Nếu turn của Tester không tạo ra ít nhất 1 test ĐỎ (do assertion fail, không phải compile error), Runner sẽ reprompt bắt viết lại.

#### Nhánh 2: Áp dụng HYBRID (2 pha) cho Task mới (`task-harness`, `vibe-sprint`)
- **Vì sao không ép ĐỎ ngay từ đầu?** Nếu là một tính năng hoàn toàn mới (ví dụ viết hàm `ExportPdf()` chưa từng tồn tại), nếu bắt viết test gọi `ExportPdf()`, trình biên dịch sẽ báo lỗi cú pháp `undefined symbol` $\rightarrow$ code không compile được thì không thể chạy test để ra màu ĐỎ!
- **Hành vi:**
  - Với **Task mới tinh:** Vẫn dùng `test_signatures` rỗng như cũ để định hình Interface trước.
  - Với **Task mang tính chất Refactor / Sửa đổi logic cũ:** (được đánh dấu qua `change_type: behavior-change` trong Change Contract), Runner sẽ tự động bật rule `r-reproduce`.

---

## 4. Cấu Trúc Kỹ Thuật Khi Triển Khai Vào Runner

1. **Gate Rule Mới: `r-reproduce`** nằm trong `internal/runner/gate_hook.go`:
   - Kiểm tra `oracle.Failed` hoặc `oracle.Regressed`.
   - Đối với node `test_signatures` của bug flow: Bắt buộc số lượng test fail mới phải $\ge 1$.
2. **Post-Turn Assertion Check:**
   - Phân biệt rõ: **Compile Error** (Lỗi cú pháp - KHÔNG ĐƯỢC CHẤP NHẬN là reproduce) vs **Assertion Error** (Lỗi logic `expected X but got Y` - ĐÂY MỚI LÀ REPRODUCE THẬT).
3. **Prompt Template Nâng Cấp:**
   - Tạo `prompts/reproduce-failing-test.md` thay thế cho `test-signatures.md` riêng trong các flow fix bug.

---

## 5. Tóm Tắt Giá Trị Đem Lại Cho Bạn

- **Không còn "Fix Bug Ảo":** 100% mọi bug được Agent fix từ nay về sau đều có bằng chứng: đã từng chạy ĐỎ và sau khi fix thì chạy XANH.
- **Bảo vệ Regression tuyệt đối:** Test tái hiện bug này sẽ được lưu lại vĩnh viễn trong test suite của dự án, chống việc bug bị tái phát trong tương lai.
- **Tiết kiệm Token:** Nếu Agent không tái hiện được bug, nó bị chặn ngay từ phút đầu tiên, không tốn hàng chục turn code linh tinh vào production code.
