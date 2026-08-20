# HLD Checklist (Phase 1)

Sử dụng checklist này trong các buổi Design Review cho High-Level Design.

## D0: Bounded Contexts
- [ ] Tính năng này phục vụ cho User Story nào?
- [ ] Tính năng này chạm vào những Bounded Context (Domain) nào?
- [ ] Có nguy cơ "rò rỉ" (leak) logic từ Domain này sang Domain khác không?
- [ ] Quyết định tạo Context mới hay mở rộng Context cũ?

## D1: Module Graph (Placement)
- [ ] Lớp Presentation sẽ nằm ở module nào? (Ví dụ: `:feature:search:ui`)
- [ ] Lớp Domain sẽ nằm ở module nào? (Ví dụ: `:feature:search:domain`)
- [ ] Lớp Data sẽ nằm ở module nào? (Ví dụ: `:feature:search:data`)
- [ ] Có kéo theo dependency lớn nào không mong muốn không?

## D2: Owner & Data Flow
- [ ] Ai là Single Source of Truth (SSOT) cho State này?
- [ ] Luồng dữ liệu (Data Flow) đã đảm bảo đi một chiều (UDF) chưa?
- [ ] Các tác vụ Mutation (Update/Delete/Create) được xử lý ra sao? Ai giữ quyền ghi?

## D3: Data Lifecycle
- [ ] Dữ liệu thuộc mức: View-bound, Screen-bound, Session-bound, hay Persistent?
- [ ] Giải pháp lưu trữ tương ứng đã phù hợp chưa (SavedState, ViewModel, Memory Cache, Room, DataStore)?
- [ ] Có chiến lược Invalidate Cache chưa?

## D4: Component Diagram
- [ ] Sơ đồ Component (C4 - Level 2/3) đã có chưa?
- [ ] Chiều mũi tên trong sơ đồ có tuân thủ Dependency Inversion không?
- [ ] Các thành phần bên ngoài (API Backend, OS System) đã được vẽ rõ chưa?

## D5: Cross-feature Boundary
- [ ] Tính năng này có cần public API cho các feature khác sử dụng không?
- [ ] Nếu có, module `:api` (Interface) và `:impl` (Implementation) đã được tách bạch chưa?
- [ ] Có rủi ro Circular Dependency ở cấp độ module không?
