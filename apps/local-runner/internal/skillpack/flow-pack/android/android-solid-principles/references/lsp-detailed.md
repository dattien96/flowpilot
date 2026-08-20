# LSP Chi Tiết — Thay thế Liskov trong Android

> *"Class con/triển khai phải thay thế hoàn toàn class cha/interface mà không làm nổ vỡ bất biến."*

---

## Câu hỏi chẩn đoán

| Scope | Câu hỏi |
|---|---|
| Feature | Các implementation có giữ đúng bất biến không, hay có class nổ `UnsupportedOperationException` / xóa dữ liệu ngầm? |
| Contract | Override trả về giá trị sentinel/dummy vô nghĩa? |

---

## Vi phạm 1: fallbackToDestructiveMigration — Xóa sạch DB user (DB-02)

### ❌ BEFORE
```kotlin
// Phá vỡ bất biến lưu trữ: "DB giữ dữ liệu của user"
val db = Room.databaseBuilder(context, AppDatabase::class.java, "app.db")
    .fallbackToDestructiveMigration() // ❌ XÓA SẠCH khi schema lệch!
    .build()
```

### ✅ AFTER
```kotlin
val MIGRATION_1_2 = object : Migration(1, 2) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE products ADD COLUMN discount REAL NOT NULL DEFAULT 0.0")
    }
}

val db = Room.databaseBuilder(context, AppDatabase::class.java, "app.db")
    .addMigrations(MIGRATION_1_2) // ✅ Bảo toàn 100% dữ liệu
    .build()
```

**Hậu quả:** User update app → mất sạch toàn bộ database.
**Lưu ý:** `fallbackToDestructiveMigration` HỢP LỆ cho DB **cache thuần** (§database-strategy).

---

## Vi phạm 2: TODO() / NotImplementedError trong implementation (MIG-01)

### ❌ BEFORE
```kotlin
interface FeatureInstaller {
    suspend fun install(featureId: String): InstallResult
    suspend fun uninstall(featureId: String): UninstallResult
}

class PlayStoreFeatureInstaller @Inject constructor() : FeatureInstaller {
    override suspend fun install(featureId: String) = ...  // OK
    override suspend fun uninstall(featureId: String): UninstallResult {
        TODO("Not implemented yet") // ❌ Crash lúc runtime!
    }
}
```

### ✅ AFTER
```kotlin
// Nếu chưa cần uninstall, tách interface:
interface FeatureInstaller {
    suspend fun install(featureId: String): InstallResult
}

// Hoặc nếu cần contract đầy đủ, implement hành vi no-op an toàn:
override suspend fun uninstall(featureId: String): UninstallResult {
    return UninstallResult.NotSupported // ✅ Giá trị có nghĩa, caller xử lý được
}
```

---

## Vi phạm 3: Override trả sentinel/dummy value vô nghĩa

### ❌ BEFORE
```kotlin
interface UserProfileProvider {
    fun getDisplayName(): String
    fun getAvatarUrl(): String
}

class GuestProfileProvider : UserProfileProvider {
    override fun getDisplayName(): String = "" // ❌ Dummy! UI hiện rỗng
    override fun getAvatarUrl(): String = ""   // ❌ Dummy! Image load fail im lặng
}
```

### ✅ AFTER
```kotlin
interface UserProfileProvider {
    fun getDisplayName(): String?  // ✅ Nullable — caller biết phải xử lý
    fun getAvatarUrl(): String?
}

class GuestProfileProvider : UserProfileProvider {
    override fun getDisplayName(): String? = null // ✅ Caller hiện "Guest"
    override fun getAvatarUrl(): String? = null   // ✅ Caller hiện default avatar
}
```

---

## Vi phạm 4: @Composable default body rỗng (IMG-04)

### ❌ BEFORE
```kotlin
// Interface với default rỗng — implementation quên override = UI trắng
interface FeatureScreen {
    @Composable
    fun Content() {} // ❌ Default rỗng — nuốt lỗi im lặng
}

class SettingsScreen : FeatureScreen {
    // Quên override Content() → UI hiện TRẮNG, không ai biết
}
```

### ✅ AFTER
```kotlin
interface FeatureScreen {
    @Composable
    fun Content() // ✅ Không default → compile error nếu quên implement
}
```

---

## Vi phạm 5: Race condition ở Search (SRCH-02)

### ❌ BEFORE
```kotlin
// User gõ "abc" rồi "abcd" nhanh. Request "abc" trả về muộn hơn → ghi đè "abcd"
class SearchViewModel : ViewModel() {
    fun search(query: String) {
        viewModelScope.launch { // ❌ launch mới mỗi lần, không cancel cũ
            val results = repository.search(query)
            _results.value = results // ❌ kết quả cũ ghi đè kết quả mới
        }
    }
}
```

### ✅ AFTER
```kotlin
class SearchViewModel : ViewModel() {
    private val _query = MutableStateFlow("")
    
    val results = _query
        .debounce(300)
        .flatMapLatest { query -> // ✅ Cancel request cũ tự động
            if (query.isBlank()) flowOf(emptyList())
            else repository.searchFlow(query)
        }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), emptyList())

    fun search(query: String) {
        _query.value = query
    }
}
```

---

## Vi phạm 6: Silent error swallowing

### ❌ BEFORE
```kotlin
// catch nuốt hết kể cả CancellationException → phá vỡ coroutine
suspend fun fetchData(): List<Item> {
    return try {
        api.getItems()
    } catch (e: Exception) { // ❌ Nuốt hết!
        emptyList() // ❌ Caller tưởng "không có data", thật ra bị lỗi mạng
    }
}

// ?.let trên dữ liệu bắt buộc — im lặng bỏ qua khi null
fun handleDeepLink(uri: Uri?) {
    uri?.let { // ❌ Nếu uri null (deeplink corrupt) → im lặng bỏ qua
        navigator.navigate(parseRoute(it))
    }
}
```

### ✅ AFTER
```kotlin
suspend fun fetchData(): AppResult<List<Item>> {
    return try {
        AppResult.Success(api.getItems())
    } catch (e: CancellationException) {
        throw e // ✅ Không nuốt CancellationException
    } catch (e: Exception) {
        AppResult.Error(e) // ✅ Caller biết lỗi, hiện retry UI
    }
}

fun handleDeepLink(uri: Uri?) {
    val route = uri?.let { parseRoute(it) }
    if (route != null) {
        navigator.navigate(route)
    } else {
        analytics.logInvalidDeepLink(uri) // ✅ Track lỗi
        navigator.navigateToHome()
    }
}
```

---

## Grep commands phát hiện

```bash
# Bom hẹn giờ
grep -rn "TODO()\|NotImplementedError\|UnsupportedOperationException" --include="*.kt" .

# Override trả sentinel
grep -rn "override.*fun" --include="*.kt" . -A3 | grep 'return ""\|return -1\|return null\|return 0'

# Default body rỗng
grep -B2 -A2 "= {}" --include="*.kt" -rn . | grep -i "composable\|abstract\|open\|override"

# Race condition
grep -rn "launch\|async" --include="*.kt" . | grep -i "search\|query\|fetch\|load"

# Silent error swallowing
grep -rn "catch.*{" --include="*.kt" . -A3 | grep "return null\|null\|emptyList"

# fallbackToDestructiveMigration
grep -rn "fallbackToDestructive" --include="*.kt" .
```
