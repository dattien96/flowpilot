# Walkthrough: Quét tính năng Home

Ví dụ này cho thấy cách áp dụng Playbook để quét module tính năng `feature:home`.

## Bước 1: Build & Architect
Tôi chạy: `grep -r "api(" ./feature-home`
- **Kết quả**: Không tìm thấy `api()`. Tốt.

## Bước 2: SRP
Chạy: `find ./feature-home -name "*.kt" -exec wc -l {} + | sort -nr | head -n 5`
- **Kết quả**: `HomeViewModel.kt` dài 650 dòng.
- **Phân tích**: Lớn hơn 400 dòng -> 🔴 Vi phạm SRP.
- Tôi mở `HomeViewModel.kt` ra xem và thấy nó chứa logic parse JSON nội bộ thay vì gọi UseCase.

Chạy: `grep -r "val .*: MutableStateFlow" ./feature-home`
- **Kết quả**: `val uiState = MutableStateFlow(...)` (không private).
- **Phân tích**: 🔴 Lỗi rò rỉ trạng thái.

## Bước 3: DIP
Chạy: `grep -r "import retrofit2" ./feature-home/domain`
- **Kết quả**: Tìm thấy `import retrofit2.Response` trong `HomeUseCase.kt`.
- **Phân tích**: 🔴 Vi phạm DIP nghiêm trọng. Domain layer đang biết về framework mạng (Retrofit).

## Báo cáo (Tóm tắt)
- **SRP**: Tách `HomeViewModel` ra thành các class nhỏ hơn, chuyển parsing logic xuống Data layer. Đóng gói `MutableStateFlow`.
- **DIP**: Tạo lại interface trong Domain trả về model thuần tuý. Cấu hình mapper trong Data layer.
