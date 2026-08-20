# [Feature Name] - Design Document

## 0. Metadata
- **Feature:** [Tên tính năng]
- **Author:** [Tên tác giả]
- **Date:** [Ngày]
- **Status:** [Draft / Review / Approved]
- **PRs:** [Links nếu có]

## 1. Yêu cầu (Requirements)
- **FR (Chức năng):**
  - ...
- **NFR (Phi chức năng):**
  - Performance: ...
  - Offline mode: ...

## 2. Phase 1: High-Level Design (HLD)

### D0: Bounded Contexts
- Contexts: ...

### D1: Module Graph
- Vị trí code: ...

### D2: Owner & Data Flow
- Nguồn dữ liệu (SSOT): ...
- Sơ đồ Flow: [Có thể dùng chữ hoặc Mermaid]

### D3: Data Lifecycle
- Chiến lược cache: ...

### D4: Component Diagram
- [Chèn ảnh hoặc link sơ đồ C4/UML vào đây]

### D5: Cross-feature Boundary
- Các API public cho module khác (nếu có): ...

## 3. Phase 2: Low-Level Design (LLD)

### Hợp đồng (Contracts) chính
```kotlin
// Đặt code interface cốt lõi ở đây
```

### Đánh giá SOLID Probe Gate
- **S (Trách nhiệm đơn lẻ):** [Giải thích tại sao pass]
- **O (Mở/Đóng):** [Giải thích khả năng mở rộng]
- **L (Liskov):** [Đảm bảo an toàn hợp đồng]
- **I (Chia nhỏ Interface):** [Xác nhận không ép dependency]
- **D (Đảo ngược phụ thuộc):** [Xác nhận Port nằm ở Consumer]

## 4. Rủi ro & Quyết định (Risks & Decisions)
- [Quyết định A]: Tại sao chọn X thay vì Y? Đánh đổi là gì?
