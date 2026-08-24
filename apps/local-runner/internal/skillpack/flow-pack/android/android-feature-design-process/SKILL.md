---
version: 6
name: android-feature-design-process
description: Quy trình thiết kế tính năng 2 pha (HLD & LLD), cổng kiểm duyệt SOLID (SOLID Gate) và tư duy Lead Design (Lead Design Mindset) cho kỹ sư Android.
---

# Android Feature Design Process — Skill Guide

> **Dùng khi nào:** Khi bắt đầu nhận một Task/Feature mới, cần lên kế hoạch kiến trúc,
> viết tài liệu thiết kế (System Specs, Tech Design) trước khi code.
>
> **Không dùng khi:** Fix bug nhỏ, hoặc code refactor nội bộ không đổi kiến trúc.

---

## 1. Tư duy Thiết kế (Lead Design Mindset)

Trước khi vào quy trình, kỹ sư cần giữ 4 tư duy sau (Mindset A-B-C-D):

1.  **Mindset A: Tìm kiếm Biên giới (Boundary)**:
    Mọi feature đều có ranh giới. Đâu là UI? Đâu là Domain? Đâu là Data? Nếu UI gọi thẳng DB, biên giới bị phá vỡ.
2.  **Mindset B: Đặt Port đúng chỗ**:
    Dependency Inversion (DIP) yêu cầu Port (interface) phải do **Consumer** sở hữu, không phải **Provider**. Giao diện `ProductRepository` phải nằm ở `:domain`, còn implementation nằm ở `:data`.
3.  **Mindset C: Port nói lên Năng lực (Capability), không phải Cơ chế (Mechanism)**:
    Interface không được rò rỉ chi tiết implement. `interface Storage { fun save(bytes: ByteArray) }` là năng lực. `interface Storage { fun saveToSqlite(blob: ByteArray) }` là cơ chế (thủng OCP/DIP).
4.  **Mindset D: Bài test 3 dòng**:
    Bất kỳ Port nào cũng phải có thể làm giả (Fake) trong tối đa 3 dòng code để viết Unit Test. Nếu phải fake thêm Context, Lifecycle, hay cấu trúc dữ liệu phức tạp của bên thứ 3 -> Port thiết kế sai.

---

## 2. Quy trình Thiết kế 2 Pha (2-Phase Design Process)

Thiết kế kiến trúc Android được chia làm 2 giai đoạn họp/review:

### Phase 1: High-Level Design (HLD)
Mục tiêu: Đạt sự đồng thuận về luồng dữ liệu, các module liên quan, và hợp đồng giao tiếp (API/Port). **Chưa viết code chi tiết.**

*   **Đầu ra:** Diagram luồng dữ liệu, danh sách các Module, định nghĩa Interface (Port).
*   **Thành phần tham gia:** Tech Lead, Android Dev, (có thể Backend Dev nếu cần API mới).

### Phase 2: Low-Level Design (LLD) & SOLID Gate
Mục tiêu: Duyệt qua chi tiết các Class, ViewModel, UseCase, và vượt qua cổng kiểm duyệt SOLID (SOLID Gate). **Bảo vệ kiến trúc trước khi gõ phím.**

*   **Đầu ra:** Danh sách các class chi tiết, Data classes, giải pháp cho các edge-cases (error handling, offline, caching).
*   **Thành phần tham gia:** Android Dev team.

---

## 3. Quy trình 13 Bước Thiết kế (D0 - D12)

Đây là 13 bước tư duy kỹ sư cần đi qua khi thiết kế một feature:

*   **D0. Hiểu rõ Yêu cầu (Requirements)**: Tính năng làm gì? Edge cases (mất mạng, lỗi API) ra sao?
*   **D1. Xác định Bounded Context**: Tính năng này thuộc `:feature` nào? Có cần tạo mới không?
*   **D2. Bóc tách Domain Models**: Dữ liệu nghiệp vụ thuần túy là gì? (Không dính `@Entity`, `@SerializedName`).
*   **D3. Định nghĩa Ports (Interfaces)**: Consumer cần gì? (Mindset B & C).
*   **D4. Thiết kế UseCases (Interactors)**: Hành động của user/hệ thống là gì? (Mỗi UseCase 1 việc - SRP).
*   **D5. Thiết kế Presentation (State & Event)**: Màn hình có những State nào? (Loading, Success, Error). User có những Event gì?
*   **D6. Định nghĩa ViewModel**: Đóng gói State (Flow/StateFlow) và tiếp nhận Event từ UI. Không dính Android Context.
*   **D7. Thiết kế Data Layer**: Repository implement các Ports (D3). Mappers chuyển đổi Data <-> Domain.
*   **D8. Quyết định Data Source (Local/Remote)**: Có cần cache DB (Room) không? API Retrofit là gì?
*   **D9. Cấu hình DI (Hilt/Dagger)**: Làm sao để nối Ports (D3) với Implementation (D7)? Nằm ở `:di`.
*   **D10. Thiết kế UI (Compose/XML)**: Phác thảo cấu trúc Component (Dumb/Smart components).
*   **D11. Kế hoạch Unit Test**: Các UseCase, ViewModel sẽ được test với Fake Ports như thế nào? (Mindset D).
*   **D12. Đánh giá Rủi ro & Migration**: Có phá vỡ dữ liệu cũ không? (LSP). Cần `@AutoMigration`?

---

## 4. SOLID Gate - 8 Bước Kiểm Duyệt (P1 - P8)

Trước khi chốt LLD, phải vượt qua 8 câu hỏi kiểm duyệt này (dựa trên 5 nguyên lý SOLID):

*   **P1 (SRP)**: ViewModel có đang làm thay việc của Domain (gọi logic phức tạp) hay Data (gọi thẳng DB) không?
*   **P2 (SRP)**: Có class nào > 300 dòng hoặc ôm > 5 dependencies (inject) không?
*   **P3 (OCP)**: Nếu thêm 1 loại dữ liệu/hành vi mới, có phải sửa lại code cũ hay chỉ cần thêm class mới?
*   **P4 (LSP)**: Các implementation của Port có ném lỗi `NotImplementedError` hay nuốt lỗi im lặng không?
*   **P5 (LSP)**: Thay đổi Data Base có cần fallbackToDestructiveMigration (xóa sạch DB) không?
*   **P6 (ISP)**: Có Interface nào ép client implement những method không cần thiết không?
*   **P7 (DIP)**: Core/Foundation có chứa danh từ riêng (tên Feature/App) không?
*   **P8 (DIP)**: Data layer có import UI class không? Domain layer có import Android/Retrofit/Room không?

*(Tham khảo `android-solid-principles` và `android-solid-scan` để xem cách sửa nếu vi phạm).*

---

## Tài liệu tham khảo

| File | Nội dung |
|---|---|
| `references/hld-checklist.md` | Checklist chi tiết cho Phase 1 (HLD). |
| `references/lld-solid-gate.md` | Bộ câu hỏi kiểm duyệt LLD và SOLID Gate. |
| `references/lead-design-mindset.md` | Deep dive vào 4 Mindset (A-B-C-D) của Lead Design. |
| `examples/feature-search-design.md` | Mẫu tài liệu thiết kế tính năng Tìm kiếm. |
| `examples/download-manager-design.md` | Mẫu tài liệu thiết kế Download Manager (background task). |
| `examples/design-artifact-template.md` | Template chuẩn để viết Markdown Design Document. |
