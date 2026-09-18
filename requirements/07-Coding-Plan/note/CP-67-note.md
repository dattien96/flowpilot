# CP-67 Bản Ghi Chú Kiến Trúc: Contract-First Scaffold TDD & Signature Lock Protocol

> **Dành cho Tech Lead / Human Operator:** Tài liệu này ghi lại chi tiết toàn bộ quyết định thiết kế cuối cùng (Final Design) của tính năng **Contract-First Scaffold TDD & Signature Lock Gate** sau chuỗi thảo luận chuyên sâu về các luồng `task-harness`, `vibe-sprint` và bài học từ `CP-64`.

---

## 1. Bối Cảnh & Vấn Đề Cốt Lõi (The Dilemma)

### 1.1 Thành công của CP-64 ở BugFix
Gần đây, chúng ta đã triển khai thành công [CP-64](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md) học tập phương pháp **Reproduce-First** từ DeepSeek / SWE-bench:
- Trước khi Coder được sửa code production, Tester bắt buộc phải viết 1 test và test đó **phải chạy ĐỎ (Assertion Failure)**.
- Sau khi test ĐỎ được xác nhận, file test bị **khóa cứng Read-Only**.
- Coder vào sửa code production sao cho test từ **ĐỎ $\rightarrow$ XANH**. Coder không thể sửa test để né lỗi.

### 1.2 "Nghịch lý con gà & quả trứng" khi áp dụng vào Task mới và Vibe
Chúng ta từng nói: *Chỉ áp dụng Reproduce-First cho BugFix vì Bug thì code cũ đã có sẵn để test.*

Tuy nhiên, trong `task-harness` và `vibe-sprint`, việc giữ quy trình cũ dẫn tới một **tử huyệt kiến trúc**:
1. **Tại bước TDD (`test_signatures`)**: Vì code tính năng mới chưa tồn tại, nếu Tester viết assertion gọi hàm mới thì compiler sẽ báo lỗi cú pháp **Compile Error** (`undefined: NewService`). Mà hệ thống cấm Compile Error, nên TDD chỉ dám viết các khung hàm test rỗng (`func TestFoo(t *testing.T) {}`).
2. **Tại bước Coder (`implement`)**: Coder được giao vừa viết code production vừa tự điền nội dung test.
   - **Nếu Coder viết test trước**: Vẫn gặp đúng vấn đề là chưa có code cụ thể thì viết thân test kiểu gì. Nếu viết được thì tại sao không viết luôn ở TDD?
   - **Nếu Coder viết code trước rồi mới viết test**: Đây chính là **Confirmation Bias / Tautological Testing ("Test tự khen mình")**. Coder viết code xong, sau đó nó viết assertion khớp đúng với những gì nó vừa viết (dù có thể hiểu sai spec hoặc sót edge case). Kết quả: Test pass 100% (Xanh) nhưng test vô dụng, không bảo vệ được nghiệp vụ!
   - **Nếu thêm một agent điền test sau Coder**: Nếu agent này nhìn vào code của Coder, nó cũng sẽ bị "nhiễm" bias và sinh test theo implementation hiện tại.

---

## 2. Quyết Định Thiết Kế Cuối Cùng: "Contract-First Scaffold TDD"

Sau khi cân nhắc, chúng ta đã thống nhất triển khai **Phương án 1 (Contract-First Scaffold TDD)** với đầy đủ các tầng kiểm duyệt vật lý và giao thức điều phối chặt chẽ:

```text
                  ┌──────────────────────────────────────────────┐
                  │                MAIN AGENT                    │
                  │         (Trọng tài & Điều phối)              │
                  │   - Nắm Master Plan & Specs                  │
                  │   - Duyệt Batch Signature Changes            │
                  │   - Ngân sách vòng lặp: cap: 5               │
                  └───────┬──────────────────────────────▲───────┘
                          │                              │
             (1) Lệnh tạo │                  (4) Báo cáo │
                 Scaffold │                      kết quả │ (Báo hoàn thành
                 + Test Đỏ│                              │  hoặc nộp BATCH)
                          ▼                              │
              ┌───────────────────────┐                  │
              │       STEP: TDD       │                  │
              │   (Model Xịn / Tier 1)│                  │
              │ - Tạo Stub Signatures │                  │
              │ - Viết Red Test Suite │                  │
              └───────────┬───────────┘                  │
                          │                              │
                          │ (Compile OK & Test ĐỎ)       │
                          │ Khóa Test + Snapshot Hash    │
                          ▼                              │
              ┌───────────────────────┐                  │
              │      STEP: CODER      │                  │
              │ - Chỉ Fill Body Code  │──────────────────┘
              │ - Cấm sửa Signature   │
              │ - Gom BATCH cuối turn │
              └───────────────────────┘
```

---

## 3. Bảy Trụ Cột Kỹ Thuật Chi Tiết

### Trụ cột 1: Step TDD dùng Model Xịn ("Scaffold & Red Test")
- Không dùng model rẻ/nhẹ cho TDD. TDD lúc này đóng vai trò là **API Contract Architect** (sử dụng Claude Sonnet 3.7 / Opus, Grok 4.6 Reasoning, Codex).
- **Nhiệm vụ của TDD**:
  1. **Sinh bộ khung Production Stubs**: Tạo/bổ sung các file production với đầy đủ Struct, Interface, Function signatures (tên, tham số, kiểu trả về). Phần thân hàm bắt buộc là Stub rỗng:
     - *Kotlin / Android*: `fun doAction(param: String): Result = TODO("not implemented")`
     - *Go*: `func DoAction(param string) (*Result, error) { return nil, errors.New("not implemented") }`
     - *TypeScript*: `export function doAction(param: string): Result { throw new Error("not implemented"); }`
  2. **Sinh Test Suite Hoàn Chỉnh (Executable Red Tests)**: Viết các test cases với assertions đầy đủ gọi vào các hàm stub vừa tạo.
- **Kết quả khi chạy test**:
  - **Compile Check**: PASS 100% (Vì các symbol, method đã tồn tại trên đĩa, không bao giờ bị lỗi `undefined symbol`).
  - **Runtime Test**: Phải CHẠY ĐỎ (RED) 100% do `TODO()` hoặc assertion fail.

---

### Trụ cột 2: Cổng Khóa Chữ Ký (`r-signature-lock`) bằng Canonical AST Hash
Làm sao ngăn chặn Coder tự tiện đổi signature?
- **Khái niệm**: Không băm nguyên cả file (vì Coder phải sửa thân hàm). Thay vào đó, băm **Chữ ký chuẩn hóa (Canonical AST Signatures)**.
- **Cơ chế**:
  1. Runner dùng AST Parser (hoặc LSP Document Symbols từ CP-63) bóc tách toàn bộ khai báo: Function name, Receiver, Param names & types, Return types, Struct/Interface fields.
  2. Loại bỏ hoàn toàn khối ruột `{ ... }`.
  3. Sắp xếp alphabet và tính `SHA256(canonical_signatures)` $\rightarrow$ lưu vào `FrozenContractRecord.SignatureHash`.
- **Kiểm duyệt**: Sau turn của Coder, nếu `Hash_Coder != Hash_TDD` mà Coder không nộp kèm yêu cầu đàm phán hợp lệ $\rightarrow$ Gate `r-signature-lock` chặn đứng ngay lập tức!

---

### Trụ cột 3: Nhiệm vụ Coder — "Fill Body Only"
- Coder bước vào một môi trường đã có sẵn:
  - Khung hàm đã định nghĩa rõ ràng.
  - Test suite đã viết sẵn và đang chạy ĐỎ.
  - File test đã bị khóa `ReadOnlyPaths` (không thể sửa hay weaken).
  - Chữ ký hàm đã bị khóa bởi `r-signature-lock`.
- Nhiệm vụ duy nhất của Coder: Viết logic nghiệp vụ vào ruột các hàm `{ ... }` sao cho test từ **ĐỎ $\rightarrow$ XANH**.

---

### Trụ cột 4: Nguyên Tắc "Accumulate & Batch" (Không Làm Lẻ Tẻ)
Khi đang code, nếu Coder phát hiện 1 signature bị thiếu tham số hoặc sai kiểu dữ liệu:
- **Tuyệt đối KHÔNG dừng lại ngay lập tức** để bắn về (tránh bẫy Ping-Pong Thrashing).
- **Quy tắc**:
  1. Ghi nhận lỗi/nhu cầu đổi signature đó vào danh sách tạm.
  2. Tiếp tục implement tối đa các phần khác của task (dù biết hiện tại test có thể chưa pass).
  3. Trong quá trình đó, nếu phát hiện thêm các lỗi signature khác thì tiếp tục gom vào.
  4. Đến **cuối turn**, đóng gói toàn bộ thành **1 BATCH REQUEST DUY NHẤT** gửi về.

---

### Trụ cột 5: Trung Gian Bắt Buộc — Main Agent (Hub-and-Spoke)
- **Tuyệt đối cấm giao tiếp ngang hàng (Peer-to-Peer)** giữa Coder và TDD.
- Mọi báo cáo của Coder đều gửi về **Main Agent (Orchestrator Hub)**:
  - **Main Agent thẩm định**: Nếu Coder lười biếng đòi xóa tham số validate $\rightarrow$ Main Agent gạt bỏ ngay, bắt Coder tự xử lý bên trong thân hàm.
  - **Nếu đề xuất hợp lý**: Main Agent chắt lọc danh sách batch, tạo chỉ thị chuẩn xác chuyển giao cho TDD cập nhật lại stub và test.
  - **Sau khi TDD sửa xong**: Main Agent kiểm tra test ĐỎ, cập nhật lại snapshot hash, rồi mới bàn giao lại cho Coder.

---

### Trụ cột 6: 100% Kết Quả Phải Là Typed Schema Result (SP-06 Tier 1)
Không bao giờ dùng regex parse văn bản tự do của AI để ra quyết định trạng thái. Mọi kết quả chuyển giao đều dùng Tool Schemas (Declared Faces):
1. **TDD Step**: Bắt buộc gọi `submit_scaffold_outcome`:
   ```yaml
   status: scaffold_ready | blocked
   stubs: [{ file, symbols: [{ name, kind, signature }] }]
   test_suite: { test_file, red_tests: [], failure_type }
   ```
2. **Coder Step**: Bắt buộc gọi `submit_coder_outcome`:
   ```yaml
   status: completed | renegotiate_signatures | blocked
   summary: "..."
   batch_signature_requests:
     - symbol: "fetchUser"
       file: "user_service.go"
       current_signature: "fun fetchUser(id: String): User"
       proposed_signature: "fun fetchUser(id: String, forceRefresh: Boolean = false): User"
       rationale: "Cần cờ bypass cache khi refresh"
   implementation_progress: "Đã xong 85% logic"
   ```

---

### Trụ cột 7: Ngân Sách An Toàn — `cap: 5`
- Trong cấu hình policy của flow: `cap: 5` (và `onCap: escalate`).
- Nhờ cơ chế Gom Batch (Trụ cột 4), thông thường chỉ cần 1 đến 2 vòng đàm phán là giải quyết xong toàn bộ thay đổi kiến trúc.
- `cap: 5` cung cấp dư địa rộng rãi cho các task phức tạp, đồng thời là chốt chặn chống tốn token nếu hai agent rơi vào vòng lặp bất đồng ý kiến.

---

## 4. Bảng So Sánh Trước và Sau Khi Có CP-67

| Tiêu chí | Trước CP-67 (Legacy) | Sau CP-67 (Contract-First TDD) |
| :--- | :--- | :--- |
| **Model ở TDD** | Model nhẹ, chỉ viết signature rỗng | **Model xịn (High-Reasoning)**, thiết kế API & Test ĐỎ |
| **Trạng thái test tại TDD** | Test rỗng $\rightarrow$ luôn XANH giả tạo | **Test đầy đủ $\rightarrow$ Compile OK nhưng chạy ĐỎ thật** |
| **Nguy cơ ở Coder** | **Confirmation Bias cực nặng** (tự code, tự viết test khen mình) | **Triệt tiêu 100% Bias** (test đã bị khóa trước khi code) |
| **Quyền sửa chữ ký** | Coder tự do đổi signature tùy tiện | **Khóa cứng qua `r-signature-lock` (AST Hash)** |
| **Xử lý khi cần đổi chữ ký** | Coder tự sửa lén trong file | **Gom BATCH cuối turn gửi Main Agent duyệt** |
| **Giao tiếp Agent** | Không rõ ràng hoặc text tự do | **100% Typed Tool Schemas qua Main Agent** |
| **Ngân sách đàm phán** | Không kiểm soát | **Khóa cứng `cap: 5`** |

---

## 5. Kết Luận & Lộ Trình Triển Khai
Tài liệu kế hoạch chi tiết đã được lập tại:
`requirements/07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md`

Các slice công việc tiếp theo:
- **P-1**: Xây dựng 2 tool faces `submit_scaffold_outcome` và `submit_coder_outcome`.
- **P-2**: Viết bộ bóc tách chữ ký chuẩn hóa AST và gate `r-signature-lock`.
- **P-3**: Viết Prompts và Agent Persona cho Scaffold Architect (TDD).
- **P-4**: Cập nhật Coder Prompts với hợp đồng "Accumulate & Batch".
- **P-5**: Cập nhật Topology `task-harness.yaml` và `vibe-sprint.yaml` với loop qua Main Agent (`cap: 5`).
