# DIP Chi Tiết — Đảo ngược Phụ thuộc trong Android

> **Vế 1:** *"High-level module không phụ thuộc Low-level module. Cả hai phụ thuộc vào Abstraction."*
> **Vế 2:** *"Abstraction không phụ thuộc Chi tiết; Chi tiết phụ thuộc Abstraction."*

**High-level = policy (đổi vì nghiệp vụ). Low-level = mechanism (đổi vì công nghệ).**

---

## Câu hỏi chẩn đoán

| Scope | Câu hỏi |
|---|---|
| Super App (vế 1) | Module dùng chung (`:foundation` / `:core`) có chứa danh từ riêng (tên app, vendor) không? |
| Contract Shape (vế 2) | Đọc mỗi file interface, tôi có đoán được impl dùng công nghệ gì không? → đoán được = thủng |
| Test 3 dòng (vế 2) | Viết fake impl trong 3 dòng được không? → phải bịa tham số vô nghĩa = thủng |

---

## Vi phạm 1: Danh từ riêng trong Foundation (SEC-01) — Vế 1

### ❌ BEFORE
```kotlin
// Foundation module — dùng chung cross-app
object NativeSecurityDefaults {
    const val LIBRARY_NAME = "appstart_security" // ❌ Tên app-specific!
}

// App B dùng lại Foundation → crash ngay lập tức:
// UnsatisfiedLinkError: dlopen failed: library "libappstart_security.so" not found
```

### ✅ AFTER
```kotlin
// Foundation chỉ giữ Abstraction (Port)
interface NativeLibraryProvider {
    val libraryName: String
}

// :core:security của App A
class AppASecurityLibraryProvider @Inject constructor() : NativeLibraryProvider {
    override val libraryName = "appstart_security"
}

// :core:security của App B
class AppBSecurityLibraryProvider @Inject constructor() : NativeLibraryProvider {
    override val libraryName = "appb_security"
}
```

**Hậu quả:** App B crash `UnsatisfiedLinkError` ngay khi mở.
**Lợi ích:** Tái sử dụng 100% Foundation Security cho App B.

---

## Vi phạm 2: Port là bản dịch 1-1 của JNI (SEC-02) — Vế 2

### ❌ BEFORE
```kotlin
// Port nằm ở security-api → vế 1 ĐÚNG (không import :core:security)
// NHƯNG signature là bản dịch 1-1 của JNI → vế 2 THỦNG
interface StaticKeyProvider {
    fun loadLibraryIfNeeded()                       // ❌ vòng đời impl rò ra
    @Throws(UnsatisfiedLinkError::class)            // ❌ exception cơ chế
    fun getStaticKeyAt(index: Int): String          // ❌ index = layout mảng char C++
}

// Fake cho test phải bịa 3 hàm, 1 hàm vô nghĩa (loadLibraryIfNeeded):
class FakeStaticKeyProvider : StaticKeyProvider {
    override fun loadLibraryIfNeeded() {} // TODO: vô nghĩa
    override fun getStaticKeyAt(index: Int) = "fake"
}
```

### ✅ AFTER
```kotlin
// security-api dùng plugin kotlin.library → không import nổi Android/JNI
fun interface StaticKeyProvider {
    fun get(keyName: String): String?
}

// Impl tự uốn theo hợp đồng:
class NativeStaticKeyProvider(
    private val lookup: (String) -> String?
) : StaticKeyProvider {
    override fun get(keyName: String): String? =
        try { lookup(keyName) }
        catch (_: LinkageError) { null } // Nuốt lỗi JNI → trả null
}

// Fake cho test = 1 dòng:
val fake = StaticKeyProvider { "test-value" }  // ✅ Fun interface, 1 dòng
```

**Quy tắc vế 2:** Build xanh nhiều tháng. Chỉ lộ khi cần impl thứ hai (đọc BuildConfig / remote config)
→ phải sửa cả Port lẫn **toàn bộ** consumer.

---

## Vi phạm 3: Data layer import UI class (EXT-01) — Vế 1

### ❌ BEFORE
```kotlin
// :feature:home:data — Data layer import UI class!
import com.app.feature.home.ui.HomeScreenState  // ❌

class HomeRepositoryImpl : HomeRepository {
    override fun getHome(): Flow<HomeScreenState> { // ❌ Trả UI type
        return api.getHomeFeed().map { response ->
            HomeScreenState(items = response.items) // ❌ Data biết UI
        }
    }
}
```

### ✅ AFTER
```kotlin
// Domain model thuần — không biết UI
data class HomeFeed(val items: List<HomeFeedItem>)

// :feature:home:data — trả domain model
class HomeRepositoryImpl : HomeRepository {
    override fun getHomeFeed(): Flow<HomeFeed> {
        return api.getHomeFeed().map { response ->
            HomeFeed(items = response.items.map { it.toDomain() })
        }
    }
}

// :feature:home:presentation — map domain → UI
class HomeViewModel @Inject constructor(private val repo: HomeRepository) : ViewModel() {
    val uiState = repo.getHomeFeed().map { feed ->
        HomeUiState(items = feed.items.map { it.toUiModel() })
    }
}
```

---

## Vi phạm 4: ViewModel bypass Gateway (PSL-P-01) — Vế 1

### ❌ BEFORE
```kotlin
// ViewModel gọi thẳng Retrofit Service — coupling cứng
class ProductViewModel @Inject constructor(
    private val productService: ProductRetrofitService, // ❌ Import trực tiếp!
) : ViewModel() {
    fun load(id: String) {
        viewModelScope.launch {
            val response = productService.getProduct(id) // ❌ Retrofit Response<T>
            // parse response...
        }
    }
}
```

### ✅ AFTER
```kotlin
class ProductViewModel @Inject constructor(
    private val getProduct: GetProductUseCase, // ✅ Domain abstraction
) : ViewModel() {
    fun load(id: String) {
        viewModelScope.launch {
            val result = getProduct(id) // ✅ Trả AppResult<Product>
            // handle result...
        }
    }
}
```

---

## Vi phạm 5: Port nằm ở module provider (AppNavModule) — Vế 1

### ❌ BEFORE
```kotlin
// Interface nằm ở :datasource:retrofit — provider sở hữu Port (SAI)
// :feature:home:domain phải depend :datasource:retrofit chỉ để biết interface
package com.app.datasource.retrofit

interface ProductRepository { // ❌ Nằm ở provider module
    suspend fun getProduct(id: String): Product
}
```

### ✅ AFTER
```kotlin
// Interface nằm ở :feature:home:domain — consumer sở hữu Port (ĐÚNG)
package com.app.feature.home.domain

interface ProductRepository { // ✅ Consumer sở hữu
    suspend fun getProduct(id: String): Product
}

// Implementation ở :feature:home:data
class ProductRepositoryImpl @Inject constructor(
    private val api: ProductApi
) : ProductRepository { ... }
```

---

## Vi phạm 6: Port mang từ vựng vendor — Vế 2

### ❌ BEFORE
```kotlin
// Port nằm đúng chỗ, nhưng từ vựng lộ Play Core
interface DynamicModuleInstaller {
    fun install(moduleName: String): Flow<SplitInstallState> // ❌ SplitInstall = Play Core
    fun getInstalledModules(): Set<String>
    fun cancel(sessionId: Int) // ❌ sessionId = Play Core concept
}

// Trạng thái mang từ vựng vendor
sealed interface SplitInstallState {
    object Downloading : SplitInstallState    // ❌ Đọc là đoán ra Play Core ngay
    object Installed : SplitInstallState
    data class Failed(val errorCode: Int) : SplitInstallState
}
```

### ✅ AFTER
```kotlin
// Port nói năng lực, không nói cơ chế
interface DynamicModuleInstaller {
    fun install(moduleName: String): Flow<InstallProgress>
    fun getAvailableModules(): Set<String>
}

sealed interface InstallProgress {
    data class Downloading(val percent: Float) : InstallProgress
    object Completed : InstallProgress
    data class Failed(val reason: InstallFailure) : InstallProgress
}

enum class InstallFailure { NETWORK, STORAGE, UNKNOWN }
```

---

## Hai bài test nhanh (không cần grep)

1. **"Đọc mỗi file interface — đoán được impl dùng công nghệ gì không?"** → đoán được = thủng vế 2.
2. **"Viết fake impl trong 3 dòng."** → phải bịa tham số vô nghĩa / phải `TODO()` = thủng vế 2.

> ⚠️ Vế 1 hỏng → Gradle/compiler la ngay. **Vế 2 hỏng → build vẫn xanh nhiều tháng** — chỉ lộ khi cần impl thứ hai.

---

## Grep commands phát hiện

```bash
# Danh từ riêng trong core/foundation
grep -rni "appstart\|app_start\|\"server\"\|\"api_key\"" --include="*.kt" core/ foundation/

# System.loadLibrary hardcode
grep -rn "System.loadLibrary\|System.load(" --include="*.kt" core/ foundation/

# Data/domain import UI
grep -rn "import.*\.ui\.\|import.*\.view\.\|import.*\.screen\." --include="*.kt" . | grep -i "data\|domain\|repository"

# Core/Foundation import Feature
grep -rn "import.*\.feature\." --include="*.kt" core/ foundation/

# ViewModel import Retrofit Service
grep -rn "import.*Service\|import.*Api\|import.*Endpoint" --include="*.kt" . | grep -i "viewmodel\|presenter"

# Kiểu vendor lọt vào interface
grep -rn "Cursor\|SQLiteDatabase\|Call<\|Response<\|okhttp3\|retrofit2\|Bitmap\|Drawable" \
  --include="*.kt" . | grep -i "interface\|-api/\|/api/"

# Exception cơ chế trong Port
grep -rn "@Throws" --include="*.kt" . | grep -i "-api/\|/api/\|/domain/"

# Port nằm ở provider module (SAI)
grep -rn "^interface \|^abstract class " --include="*.kt" . | grep -v "test\|Test"
# → Kiểm tra: interface nằm ở :datasource hay :domain?
```
