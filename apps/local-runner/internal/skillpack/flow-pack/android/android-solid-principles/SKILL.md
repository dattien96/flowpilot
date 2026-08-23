---
name: android-solid-principles
description: Comprehensive Android SOLID principles skill
---

# Android SOLID Principles

Nguyên tắc SOLID giúp tạo ra một kiến trúc phần mềm linh hoạt, dễ mở rộng và bảo trì. Trong Android, việc áp dụng đúng SOLID giúp hạn chế rủi ro crash, memory leak và tăng khả năng tái sử dụng.

## S - Single Responsibility Principle (SRP)
> "Một lớp chỉ nên có một lý do duy nhất để thay đổi."

❓ **Câu hỏi chẩn đoán:**
- Class này có đang quản lý quá nhiều trạng thái không?
- Thay đổi UI có ảnh hưởng đến logic mạng không?

❌ **BEFORE (Vi phạm SRP):**
```kotlin
// ViewModel đang phụ thuộc vào Context, R.drawable, Toast
// Issue: PSL-A-07, HOME-01
class UserViewModel(private val context: Context) : ViewModel() {
    val errorIcon = R.drawable.ic_error
    fun showError(msg: String) {
        Toast.makeText(context, msg, Toast.LENGTH_SHORT).show()
    }
}
```
*Lý do sai:* ViewModel không nên biết về Context hay UI components. Nó làm khó test và dễ gây memory leak.

✅ **AFTER (Tuân thủ SRP):**
```kotlin
class UserViewModel : ViewModel() {
    private val _uiState = MutableStateFlow<UserUiState>(UserUiState.Loading)
    val uiState: StateFlow<UserUiState> = _uiState.asStateFlow()
    
    fun handleError(msg: String) {
        _uiState.update { it.copy(error = msg) }
    }
}
```
*Lý do đúng:* ViewModel chỉ quản lý state. Formatting và hiển thị là trách nhiệm của Composable/View.

💥 **Hậu quả vi phạm:** Khó test, Memory leak.
✨ **Lợi ích sau khi sửa:** Dễ test, code sạch.

## O - Open/Closed Principle (OCP)
> "Thực thể phần mềm nên mở cho việc mở rộng, nhưng đóng cho việc sửa đổi."

❓ **Câu hỏi chẩn đoán:**
- Thêm tính năng mới có phải sửa core module không?

❌ **BEFORE (Vi phạm OCP):**
```kotlin
// sealed interface trong :core buộc mọi feature phải sửa đổi core
// Issue: NAV-01
sealed interface AppRoute {
    object Home : AppRoute
    object Profile : AppRoute // Thêm tính năng phải sửa file này
}
```

✅ **AFTER (Tuân thủ OCP):**
```kotlin
// open interface cho phép tính năng tự định nghĩa
interface AppRoute
// Hilt Multibindings cho pluggable NavGraph
```

💥 **Hậu quả vi phạm:** Conflict code, tăng build time.
✨ **Lợi ích sau khi sửa:** Độc lập module.

## L - Liskov Substitution Principle (LSP)
> "Đối tượng của lớp con phải thay thế được cho lớp cha mà không làm thay đổi tính đúng đắn của chương trình."

❓ **Câu hỏi chẩn đoán:**
- Hàm có trả về giá trị mặc định sai lệch hoặc nuốt lỗi không?

❌ **BEFORE (Vi phạm LSP):**
```kotlin
// fallbackToDestructiveMigration ngầm xóa dữ liệu
// Issue: DB-02
Room.databaseBuilder(context, AppDatabase::class.java, "db")
    .fallbackToDestructiveMigration()
    .build()
```

✅ **AFTER (Tuân thủ LSP):**
```kotlin
// Cung cấp Migration cụ thể
Room.databaseBuilder(context, AppDatabase::class.java, "db")
    .addMigrations(MIGRATION_1_2)
    .build()
```

💥 **Hậu quả vi phạm:** Mất dữ liệu người dùng.
✨ **Lợi ích sau khi sửa:** An toàn dữ liệu.

## I - Interface Segregation Principle (ISP)
> "Nhiều interface đặc thù cho client thì tốt hơn là một interface chung chung."

❓ **Câu hỏi chẩn đoán:**
- Client có phải implement những hàm không cần thiết không?

❌ **BEFORE (Vi phạm ISP):**
```kotlin
// @Composable trên interface trong core làm lộ dependency Compose
// Issue: P1, IMG-01
interface ImageLoader {
    @Composable fun LoadImage(url: String)
}
```

✅ **AFTER (Tuân thủ ISP):**
```kotlin
// Interface thuần Kotlin
interface ImageLoader {
    fun load(url: String, imageView: ImageView)
}
```

💥 **Hậu quả vi phạm:** Dependency coupling.
✨ **Lợi ích sau khi sửa:** Clean architecture.

## D - Dependency Inversion Principle (DIP)
> "Module cấp cao không nên phụ thuộc module cấp thấp. Cả hai nên phụ thuộc abstraction."

❓ **Câu hỏi chẩn đoán:**
- Core có phụ thuộc vào app-specific không?

❌ **BEFORE (Vi phạm DIP):**
```kotlin
// Foundation hardcode tên thư viện của App A
// Issue: SEC-01
System.loadLibrary("appA_crypto")
```

✅ **AFTER (Tuân thủ DIP):**
```kotlin
// Thông qua interface
interface NativeLibraryProvider {
    fun getLibraryName(): String
}
```

### Vế 2 của DIP: Contract Shape
**Port must not leak impl details.**

❌ **BEFORE (Port lộ chi tiết JNI):**
```kotlin
interface CryptoPort {
    fun jniEncrypt(data: ByteArray, key: ByteArray): ByteArray
}
```

✅ **AFTER (Clean contract):**
```kotlin
fun interface StaticKeyProvider {
    fun getKey(): String
}
```

## Tổng hợp 147+ Issues
| Principle | Issues |
| --- | --- |
| SRP | PSL-A-07, HOME-01 |
| OCP | NAV-01 |
| LSP | DB-02 |
| ISP | P1, IMG-01 |
| DIP | SEC-01 |

## FORCE Rules
1. KHÔNG truyền Context vào ViewModel.
2. KHÔNG dùng fallbackToDestructiveMigration.
3. KHÔNG đặt @Composable vào interface ở core.
