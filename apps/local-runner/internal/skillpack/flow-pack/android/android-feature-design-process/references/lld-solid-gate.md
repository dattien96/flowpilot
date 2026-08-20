# LLD SOLID Gate (Phase 2)

Chi tiết 5 câu hỏi "chặn cổng" (Probe Gate) trước khi bắt đầu implementation. Trả lời chi tiết cho từng interface / component cốt lõi.

## 1. Probe S (Single Responsibility Principle)
- **Câu hỏi:** Class / ViewModel / UseCase này có đang gánh nhiều hơn 1 lý do để thay đổi không?
- **Cách kiểm tra:** Liệt kê các lý do có thể khiến bạn phải sửa file này trong tương lai. Nếu có > 1 nhóm lý do không liên quan (ví dụ: đổi UI state VÀ đổi mapping network), thì vi phạm SRP.
- **Giải pháp:** Tách class thành các component nhỏ hơn.

## 2. Probe O (Open/Closed Principle)
- **Câu hỏi:** Khi có thêm một biến thể (variant) mới của rule/nghiệp vụ, tôi có phải sửa lại code cốt lõi của class hiện tại không?
- **Cách kiểm tra:** Giả sử ngày mai có thêm yêu cầu "Hỗ trợ login bằng Apple" (hiện tại mới có Google, Facebook). Class `LoginManager` có phải thêm `if/else` hoặc `when` không? Nếu có -> Vi phạm.
- **Giải pháp:** Dùng Polymorphism (Interface/Abstract class). Inject các strategy vào class cốt lõi.

## 3. Probe L (Liskov Substitution Principle)
- **Câu hỏi:** Mọi Implementation (Fake, Real, Mock) của Interface này có thể thay thế cho nhau mà không làm Crash/Thay đổi hành vi đúng của Consumer không?
- **Cách kiểm tra:** 
  - Implementation A có ném Exception nào mà Interface không quy định không?
  - Implementation B có trả về Null trong khi hợp đồng mong đợi Non-Null không?
  - Implementation C có side-effect ẩn không?
- **Giải pháp:** Làm rõ Hợp đồng (Contract) trong Interface (bằng comment/doc hoặc signature rõ ràng), đảm bảo mọi Impl phải tuân thủ nghiêm ngặt.

## 4. Probe I (Interface Segregation Principle)
- **Câu hỏi:** Interface này có quá mập không? Consumer có bị ép phải implement (hoặc nhận) những thứ nó không hề cần đến không?
- **Cách kiểm tra:** Có class nào `implement` interface nhưng để các hàm là `throw NotImplementedError()` hoặc thân hàm rỗng không?
- **Giải pháp:** Chia nhỏ interface mập thành nhiều role interface nhỏ hơn.

## 5. Probe D (Dependency Inversion Principle)
- **Câu hỏi:** Interface có nằm ĐÚNG CHỖ (ở package/module của Consumer) và có HÌNH DẠNG ĐÚNG (nói ngôn ngữ của Consumer) không?
- **Cách kiểm tra:** 
  - Package chứa Interface có nằm chung với package của implementation không? (Lỗi!)
  - Tên hàm của interface có chứa chi tiết cơ chế (VD: `getFromRoom`, `fetchHttp`) không? (Lỗi!)
- **Giải pháp:** Chuyển Interface về module/package của lớp gọi (Consumer). Sửa tên hàm phản ánh "Năng lực" thay vì "Cơ chế".
