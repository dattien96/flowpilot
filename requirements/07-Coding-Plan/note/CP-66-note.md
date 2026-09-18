# CP-66 Bản Ghi Chú Kiến Trúc: Living Knowledge Base & `knowledge.flow` Context Source

> **Dành cho Bạn (Human / Tech Lead):** Tài liệu này giải thích chi tiết vấn đề "mù mờ kiến trúc" và lãng phí hàng chục ngàn token khi Agent phải tự mò mẫm codebase, cách học tập mô hình "DeepWiki" của Devin AI kết hợp cùng sức mạnh có sẵn của GitNexus + LSP, và cách cắm nguồn ngữ cảnh này vào FlowPilot như một Context Source chuẩn mực.

---

## 1. Issue Hiện Tại Của Chúng Ta Là Gì? (The Pain Point)

Trong một dự án thực tế quy mô trung bình đến lớn (từ 20.000 đến 100.000 dòng code):

### Vòng luẩn quẩn của Agent ở Turn 1:
Khi người dùng giao một task hoặc báo một bug:
1. **Agent bị "mù đường":** Nó không biết file nào gọi file nào, luồng dữ liệu đi từ Controller xuống Service qua Repository như thế nào.
2. **Đốt token điên cuồng:** Agent buộc phải dùng các tool `ls`, `grep_search`, `view_file` để đọc lướt 20 đến 50 file mã nguồn thô.
   - Mỗi file 500 dòng $\rightarrow$ Tốn **40.000 đến 60.000 token** ngay ở lượt đầu tiên chỉ để... "ngó nghiêng" xem code nằm ở đâu!
3. **Ảo giác và quên yêu cầu (Context Dilution):**
   - Đến lượt thứ 2 hoặc thứ 3, Context Window của Agent đã ngập tràn mã nguồn rác từ các file đọc dở.
   - Model bắt đầu "lú lẫn", sinh ra các đoạn code vi phạm kiến trúc phân tầng của dự án (ví dụ: Controller gọi thẳng vào DB mà không qua Service).

---

## 2. Ta Apply Bài Học Devin "DeepWiki" Để Làm Gì? (The Solution)

Devin AI giải quyết bài toán này bằng cách xây dựng một **"Bách khoa toàn thư sống" (Living DeepWiki)** được chưng cất sẵn từ mã nguồn. 

Trong FlowPilot, chúng ta có một lợi thế khổng lồ mà Devin không có:
- **GitNexus:** Đã phân tích sẵn toàn bộ repository thành 300 luồng nghiệp vụ thực thi (`Execution Flows`) và bản đồ phụ thuộc liên module.
- **LSP (từ CP-63):** Đã có sẵn cây cú pháp (AST) và bảng danh bạ Interfaces/Models chính xác 100%.

Chúng ta tận dụng 2 "mỏ vàng" này để biên dịch thành một **Bộ Tri Thức Sống (Living Knowledge Base)** nằm tại `.flowpilot/knowledge/`:

```text
.flowpilot/knowledge/
├── system-overview.md      ◄── Kiến trúc tổng thể, phân tầng, thư viện chung (~500 tokens)
├── execution-flows.md       ◄── Top các luồng nghiệp vụ cốt lõi (Login, Checkout, Sync...) (~800 tokens)
└── data-models.md           ◄── Danh bạ các bảng DB, Entities, DTOs quan trọng (~400 tokens)
```

Thay vì bắt AI đọc 50 file code thô (50.000 tokens), Runner chỉ cần bốc đúng **1 trang tóm tắt luồng liên quan (~500 tokens)** ném vào prompt:
$\rightarrow$ **AI hiểu ngay toàn bộ kiến trúc trong 0.2 giây, tiết kiệm 99% token mò đường!**

---

## 3. Nó Tương Thích Với Hệ Thống Hiện Tại Như Thế Nào?

Đây là câu hỏi rất quan trọng bạn đã đặt ra: *"Cái này có xung đột với các context source cũ không? Và nó cắm vào đâu?"*

### 3.1 Không xung đột, mà bổ trợ hoàn hảo:
- `conventions`: Dạy AI **Cách viết code** (Quy tắc, TDD, code style).
- `canonical.head`: Dạy AI **Spec nghiệp vụ cụ thể** của feature này.
- `change.contract`: Dạy AI **Phạm vi file được phép sửa** trong turn này.
- `source.dependence`: Cho AI thấy **Quan hệ vi mô (Micro)** giữa 2 hàm cạnh nhau.
- ⭐ **`knowledge.flow` (Nguồn mới):** Cho AI thấy **Hành trình vĩ mô (Macro)** của luồng dữ liệu xuyên suốt toàn hệ thống (`Controller ──► Service ──► DB`).

### 3.2 Cắm vào hệ thống như một Context Source chuẩn CP-44 / SD-22:
- Chúng ta tạo struct `KnowledgeFlowSource` implement interface `ContextSource`.
- Đăng ký source key: `knowledge.flow`.
- Cắm vào `candidateSources` của các node cần tư duy kiến trúc trong file YAML:
  ```yaml
  contextProfiles:
    plan_writer: # Node lập kế hoạch
      candidateSources: [conventions, knowledge.flow, source.excerpt]
    investigate: # Node điều tra bug
      candidateSources: [conventions, knowledge.flow, source.excerpt]
    coder: # Node viết code: KHÔNG NẠP để tiết kiệm token
      candidateSources: [conventions, change.contract, source.excerpt]
  ```

---

## 4. Vòng Đời Vận Hành: Khi Nào Gen? Khi Nào Update? Ai Làm Gì?

### A. Khi nào Gen (Khởi tạo lần đầu)?
- **Thời điểm:** Khi người dùng vừa tạo project trong FlowPilot (`/project` wizard) hoặc chạy lệnh `flowpilot init`.
- **Cách làm:** Runner kích hoạt 1 lần chạy phân tích ngầm của GitNexus và LSP, chưng cất thành các file Markdown trong `.flowpilot/knowledge/`.

### B. Khi nào Update (Cập nhật sống)?
> **TUYỆT ĐỐI KHÔNG cập nhật sau mỗi lần gõ phím!**
- **Thời điểm:** Nằm tại node **`audit`** (bước cuối cùng của mỗi flow, khi code đã được test, review và gate duyệt pass 100%).
- **Cách làm:** Runner lấy danh sách file vừa sửa, cập nhật vi sai (incremental patch) vào file markdown tương ứng (ví dụ thêm 1 field vào `data-models.md`).

### C. Ai là người bốc file? (Runner hay AI?)
- **100% Code Go của Runner làm việc Tra cứu & Bốc tài liệu:**
  - Runner đọc target file từ prompt/locus (ví dụ: `PaymentService.kt`).
  - Runner tra đồ thị GitNexus: `PaymentService` thuộc flow `Checkout_Flow`.
  - Runner đọc đúng đoạn Markdown của `Checkout_Flow` nhét vào context slot.
- **AI chỉ việc đọc và suy luận:** AI mở phiên lên là thấy ngay tài liệu luồng nghiệp vụ đã được mở sẵn trên bàn.

---

## 5. Tóm Tắt Giá Trị Đem Lại Cho Bạn

- **Tiết kiệm từ 30.000 đến 50.000 token mỗi session:** Triệt tiêu hoàn toàn các turn "mò mẫm tìm file" vô nghĩa.
- **Chất lượng code chuẩn kiến trúc:** Agent luôn nhìn thấy bản đồ phân tầng của hệ thống, không bao giờ viết code "bắc cầu bậy" vi phạm kiến trúc.
- **Càng dùng càng thông minh:** Knowledge Base được cập nhật liên tục sau mỗi task hoàn thành ở node `audit`, biến FlowPilot thành một trợ lý ngày càng am hiểu codebase của riêng bạn.
