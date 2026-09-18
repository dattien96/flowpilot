---
name: react-native-design-process
description: Quy trình thiết kế tính năng 2 pha (HLD & LLD) chuẩn SOLID cho React Native, chuyển giao từ quy trình AppStart Design Process (FR/NFR -> HLD -> LLD -> Scaffold code).
version: 6
---

# React Native 2-Phase Feature Design Process

Quy trình thiết kế hệ thống và tính năng mới trước khi code, đảm bảo kiểm soát chặt chẽ biên kiến trúc và tuân thủ SOLID cơ học.

---

## 1. TỔNG QUAN LUỒNG THIẾT KẾ (THE 4-STEP AGENDA)

```text
┌──────────────────────────────────────────────────────────────────────────┐
│  0. REQUIREMENTS DISCOVERY                                               │
│     Thu thập bài toán bằng ngôn ngữ nghiệp vụ (0 tên component / lib)    │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  1. FR / NFR                                                             │
│     Chốt Functional Requirements + Non-Functional Requirements           │
│     → Input bắt buộc trước khi vẽ kiến trúc                              │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  2. HIGH-LEVEL DESIGN (HLD)          ← PHASE 1                           │
│     Ngồi package nào? Port gì? Đổi lib đụng đâu? DB/API chạm module nào? │
│     Đơn vị: Package Monorepo + Public Interface + Dependency Matrix      │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  │  HLD GATE PASS
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  3. LOW-LEVEL DESIGN (LLD)           ← PHASE 2                           │
│     Design theo SOLID: Component Map (I1–I9) + Worksheet S–O–L–I–D      │
│     Hooks, services, entities, invariants, race conditions               │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  │  LLD SOLID GATE PASS
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  4. SCAFFOLD CODE & TEST (Chạy Playbook verify)                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 2. PHASE 1: HIGH-LEVEL DESIGN (HLD)

### Bảng câu hỏi xác định Public API Surface (Q1 – Q11)
Trả lời từng câu hỏi để tìm ra các artifact đưa vào file `index.ts` của Feature hoặc Core Package:

| # | Keyword | Câu hỏi từ Requirement | Có ➔ Thêm vào Public API |
| :--- | :--- | :--- | :--- |
| **Q1** | **ACTOR** | Ai gọi capability này? (Màn hình Home, Background Task, App khác) | Ghi nhận Consumer; chưa tạo class |
| **Q2** | **ACTION** | Họ muốn *làm* động từ gì? (save, compress, calculate, export) | Method trên **Facade / Service Interface** |
| **Q3** | **INPUT** | Họ đưa vào những gì? | **Request / DTO** interface thuần túy |
| **Q4** | **ID** | Có cần chỉ đúng 1 task đang chạy để cancel/observe không? | Thuộc tính `id: string` trên Request |
| **Q5** | **PROGRESS**| Có cần theo dõi tiến trình (0-100%) khi đang chạy không? | **Progress Callback** hoặc **Stream/Observable** |
| **Q6** | **STATUS** | Trạng thái có đặt tên được không? | **Union Type** (`'idle' \| 'loading' \| 'success' \| 'error'`) |
| **Q7** | **RESULT** | Kết quả cuối cùng là gì? | **Result<T, Error>** Discriminated Union |
| **Q8** | **CONTROL** | Consumer có cần Pause / Resume / Cancel không? | Method `abort()` qua `AbortController` |
| **Q9** | **OPTIONS** | Có tùy chọn cấu hình không? (priority, timeout, size) | Options interface |
| **Q10**| **UI-BIND** | Consumer có vẽ lên màn hình không? | Component UI xuất khẩu (Ví dụ: `<QuoteEditorView />`) |
| **Q11**| **FILTER** | Tên có dính chữ `SQLite`, `Axios`, `FFmpeg`, `MMKV` không? | **CẤM** đưa vào Public API ➔ Đẩy xuống LLD/Internal |

---

## 3. PHASE 2: LOW-LEVEL DESIGN (LLD)

### Bảng phân rã trách nhiệm nội bộ (I1 – I9)
Trả lời câu hỏi để tìm các component bên trong thư mục `internal/`:

| # | Keyword | Câu hỏi phân rã | Có ➔ Component nội bộ trong Impl |
| :--- | :--- | :--- | :--- |
| **I1** | **ORCHESTRATE**| Có nhiều bước cần xếp hàng / điều phối không? | `XxxEngine` / `XxxCoordinator` |
| **I2** | **FETCH** | Có bước lấy dữ liệu/stream từ bên ngoài không? | `XxxFetcher` / `RemoteDataSource` |
| **I3** | **DECODE** | Có bước đổi định dạng dữ liệu (JSON ➔ Model, Bytes ➔ Image) không? | `XxxMapper` / `XxxDecoder` |
| **I4** | **STORE** | Có bước ghi đĩa / cache tách biệt khỏi fetch không? | `LocalDataSource` / `StorageCache` |
| **I5** | **POLICY** | Có luật nghiệp vụ / Retry / Tiêu chuẩn giảm giá không? | `XxxPolicy` / `DomainService` |
| **I6** | **EMIT** | Có phát sự kiện hoặc thông báo tiến trình nội bộ không? | `EventEmitter` / `Subject` |
| **I7** | **BIND** | Có gắn kết quả vào state của UI/Hook không? | Custom Hook Controller (`useXxxController`) |
| **I8** | **PLUG** | Mai sau muốn thêm loại nguồn mới mà không sửa Core không? | `Strategy Registry` |

---

## 4. BẢNG KIỂM SOÁT SOLID GATE TRƯỚC KHI CODE

Trước khi sinh code, bản thiết kế bắt buộc phải vượt qua bảng checklist này:

* [ ] **S Gate:** Mỗi component/file chỉ có đúng 1 lý do để thay đổi. Tách biệt hoàn toàn Pure Domain (0% UI), Custom Hook (State/Event), và Dumb UI Component.
* [ ] **O Gate:** Các biến thể mở rộng (loại file nén, loại ca trực, gói thuê bao) được cắm vào qua Strategy/Registry, không dùng `switch(type)` cứng ở Core.
* [ ] **L Gate:** Không có hàm ném lỗi `throw new Error('TODO')` hoặc trả về giá trị giả rỗng (`""`, `null`) để nuốt lỗi. Mọi luồng async dùng `Result<T, E>` fail-closed.
* [ ] **I Gate:** Public interface có ≤ 5 methods. Component nhận ≤ 7 props rời rạc (hoặc dùng UiState). Không leak thư viện SQLite/Axios ra public API.
* [ ] **D Gate:** Consumer sở hữu Interface (Port). Tầng Presentation và Domain không import bất kỳ class Database hay Network cụ thể nào.
