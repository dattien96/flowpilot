---
name: android-solid-scan
description: Mechanical Playbook for detecting SOLID violations in Android
version: 1.0.0
---

# Mechanical Playbook: Android SOLID Scan

**Tài liệu này KHÔNG dạy lý thuyết SOLID.**
Tài liệu này dạy bạn mở code lên thì nhìn vào đâu, đếm cái gì, grep cái gì để tự phát hiện lỗi một cách máy móc như một bác sĩ khám bệnh. Đây là các giao thức kiểm tra từng bước, không phải lý thuyết trừu tượng.

---

## Bước 0: Kiến trúc & Build (Mở `build.gradle.kts` TRƯỚC, mở source code SAU)

Không bao giờ nhìn vào code trước. Nhìn vào cách các module liên kết với nhau trước.

### 0.1 Đếm `api()`
Mỗi `api()` ép TẤT CẢ các module tiêu thụ (consumers) phải "nuốt" dependency đó.
- Lệnh kiểm tra:
  ```bash
  grep -r "api(" . --include="*.gradle.kts"
  ```
- 🔴 Cảnh báo: Nếu thấy `api(project(":feature:..."))`, đó là dấu hiệu rò rỉ kiến trúc. Nên dùng `implementation()`.

### 0.2 Hướng phụ thuộc module
Dependency chỉ được trỏ từ trên xuống (App -> Feature -> Domain -> Foundation).
- 🔴 Vi phạm: Core/Foundation phụ thuộc vào Feature.

### 0.3 Hilt/KSP Plugins
- Foundation (Core, Utilities) MUST NOT có Hilt.
- Lệnh kiểm tra:
  ```bash
  grep -r "dagger.hilt.android.plugin" . --include="*.gradle.kts"
  ```
- 🔴 Vi phạm: Nếu thấy Hilt trong thư mục `core/` hoặc `domain/`.

### 0.4 Convention Plugin auto-apply vs opt-in
- Kiểm tra xem các plugins convention có đang bị lạm dụng không. Nên thiết kế dạng opt-in (module nào cần thì apply).

### 0.5 Cấu hình Room Database
- Lệnh kiểm tra:
  ```bash
  grep -r "fallbackToDestructiveMigration" .
  ```
- 🔴 Cảnh báo: Dùng `fallbackToDestructiveMigration` ở production. Kiểm tra `exportSchema = false`.

---

## Bước 1: Single Responsibility Principle (SRP) Detection

Phát hiện các class làm quá nhiều việc.

### 1.1 Đếm số dòng code (LOC)
- Thresholds:
  - `< 200`: Tốt.
  - `200 - 400`: Có thể chấp nhận.
  - `> 400`: 🔴 Vi phạm (Cần tách).
- Lệnh:
  ```bash
  find . -name "*.kt" -exec wc -l {} + | sort -nr | head -n 20
  ```

### 1.2 Import Analysis
- Lệnh:
  ```bash
  grep -r "^import " . | sort | uniq -c | sort -nr
  ```
- 🔴 Vi phạm: ViewModel chứa import của `android.view.*` hoặc `androidx.compose.ui.*`.

### 1.3 Đếm tham số Constructor
- 🔴 Vi phạm: Nếu constructor có `> 5` dependencies (đặc biệt là các UseCase trong ViewModel).
- Lệnh:
  ```bash
  grep -r "class .*ViewModel" . -A 5
  ```

### 1.4 Mutable state leaks
- 🔴 Vi phạm: Lộ `MutableStateFlow` ra ngoài ViewModel.
- Lệnh:
  ```bash
  grep -r "val .*: MutableStateFlow" .
  ```

### 1.5 Business logic ở sai tầng
- 🔴 Vi phạm: Tính toán, điều kiện if/else phức tạp trong UI layer (Fragment, Composable).

---

## Bước 2: Open/Closed Principle (OCP) Detection

### 2.1 Sealed Interfaces trong Core
- 🔴 Cảnh báo: `sealed class` ở Core module bị thay đổi quá nhiều lần vì thêm tính năng. Nên dùng interface mở nếu cần mở rộng độc lập.

### 2.2 Đếm số lượng nhánh `when`
- Lệnh:
  ```bash
  grep -r "when (" . -A 10
  ```
- 🔴 Vi phạm: Nếu một `when` statement có chứa các logic phức tạp trong mỗi nhánh thay vì delegate.

### 2.3 Copy-paste Code
- Dùng CPD (Copy/Paste Detector) hoặc grep các đoạn mã giống nhau.

### 2.4 Cờ Boolean (Boolean Flags)
- Lệnh:
  ```bash
  grep -r "is.*: Boolean" .
  ```
- 🔴 Cảnh báo: Lạm dụng cờ boolean (`isVip`, `isPremium`, v.v.) trong data layer thay vì đa hình.

---

## Bước 3: Liskov Substitution Principle (LSP) Detection

### 3.1 Tìm `TODO()` hoặc `NotImplementedError`
- Lệnh:
  ```bash
  grep -r "TODO(" .
  grep -r "NotImplementedError" .
  ```
- 🔴 Vi phạm: Implement interface nhưng quăng lỗi do không hỗ trợ chức năng.

### 3.2 Sentinel returns
- 🔴 Cảnh báo: Trả về `-1`, `""`, hoặc null vô nghĩa thay vì dùng kiểu dữ liệu thể hiện lỗi (như Result).

### 3.3 `@Composable` default `{}`
- 🔴 Vi phạm: Hàm Composable nhận tham số function nhưng default `{}` làm mất dấu hiệu bắt buộc truyền callback.

### 3.4 Nuốt lỗi im lặng (Silent error swallowing)
- Lệnh:
  ```bash
  grep -r "catch (e: Exception) {}" .
  ```
- 🔴 Vi phạm: Khối `catch` rỗng.

---

## Bước 4: Interface Segregation Principle (ISP) Detection

### 4.1 `api()` vs `implementation()`
- (Đã check ở Bước 0)

### 4.2 Đếm method trong Interface
- Lệnh:
  ```bash
  grep -r "interface " . -A 20
  ```
- 🔴 Vi phạm: Interface có `> 5` methods. Khách hàng thường không dùng hết.

### 4.3 `@Composable` on Interface
- 🔴 Vi phạm: Interface chứa các hàm bị đánh dấu `@Composable`, làm giới hạn nơi sử dụng.

### 4.4 Fragment implementing Domain
- 🔴 Vi phạm: Fragment hoặc Activity trực tiếp implement interface của domain.

---

## Bước 5: Dependency Inversion Principle (DIP) Detection

### 5.1 Danh từ riêng (Proper Nouns) trong Domain
- 🔴 Vi phạm: Domain module chứa các chữ như `Firebase`, `Retrofit`, `Room`.
- Lệnh:
  ```bash
  grep -r "Firebase" ./domain
  grep -r "Retrofit" ./domain
  ```

### 5.2 Hướng Import
- 🔴 Vi phạm: Domain module import từ Data hoặc App module.

### 5.3 Port Ownership
- Giao diện (Port) cho UseCase phải được định nghĩa ở lớp Domain. Lớp Data thực thi giao diện đó.

### 5.4 Vị trí Hilt Module
- 🔴 Cảnh báo: Định nghĩa Hilt Module ở tầng Domain. (Nên ở Data hoặc App).

---

## Bước 6: Các lỗi cắt ngang (Cross-cutting)

- Lệnh kiểm tra `!!` hoặc `checkNotNull`:
  ```bash
  grep -r "!!" .
  grep -r "checkNotNull" .
  ```
- Lệnh kiểm tra `by lazy` trên biến `var`:
  ```bash
  grep -r "var .* by lazy" .
  ```

---

## Bước 7: Data Modeling

- Lệnh kiểm tra cấu trúc Map lạ:
  ```bash
  grep -r "Map<String, Any>" .
  ```
- Lệnh kiểm tra bỏ qua kết quả trả về:
  ```bash
  grep -r "@file:Suppress(\"UnusedReturnValue\")" .
  ```

---

## Quick Reference Table (Nhìn Thấy Gì → Nghi Ngờ Gì)

| What you see (Nhìn thấy gì) | Suspect (Nghi ngờ gì) | Next action (Hành động tiếp) |
|---|---|---|
| `api(project(":core"))` | Vi phạm ISP/Encapsulation | Đổi thành `implementation` |
| `val state = MutableStateFlow()` public | Trạng thái có thể bị sửa từ bên ngoài | Thay bằng `asStateFlow()` |
| `interface Repository` có 15 methods | Vi phạm ISP | Tách ra các Query/Command interface nhỏ |
| Domain import `android.content.Context` | Vi phạm DIP (Domain phụ thuộc framework) | Truyền Data qua model thuần Kotlin |
| Bắt exception rỗng `catch(e) {}` | Vi phạm LSP (Nuốt lỗi) | Log lỗi, ném ra ngoài hoặc bọc trong Result |
| > 5 tham số ở constructor ViewModel | Vi phạm SRP | Xem xét gom nhóm hoặc Extract Class |
