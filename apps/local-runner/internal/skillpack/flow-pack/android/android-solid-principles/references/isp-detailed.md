# ISP Chi Tiết — Phân tách Interface trong Android

> *"Không ép Client phải phụ thuộc vào những method/dependency mà nó không sử dụng."*

---

## Câu hỏi chẩn đoán

| Scope | Câu hỏi |
|---|---|
| Module | Module `-api` có bị rò rỉ (leak) thư viện bên thứ 3 (Retrofit, Compose, Room) ra ngoài không? |
| Interface | Client nào chỉ dùng 1-2 method nhưng bị ép implement/depend cả interface 10 method? |

---

## Vi phạm 1: @Composable trên interface trong module dùng chung (P1)

### ❌ BEFORE
```kotlin
// :core:navigation — interface ép leak Compose dependency cho tất cả consumer
interface AppFeatureNavGraph {
    @Composable // ❌ Ép :core:navigation phải dùng api(libs.androidx.compose.runtime)
    fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput)
}

// build.gradle.kts của :core:navigation
dependencies {
    api(libs.androidx.compose.runtime) // ❌ Mọi consumer nuốt Compose dù chỉ cần route interface
}
```

### ✅ AFTER
```kotlin
// Interface thuần Kotlin — tách Registration Phase (Plain) và Render Phase (Composable)
interface AppFeatureNavGraph {
    fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput)
    // ✅ Pure Kotlin function! Compose chỉ cần ở implementation level
}

// build.gradle.kts
dependencies {
    implementation(libs.androidx.compose.runtime) // ✅ Hạ từ api() xuống implementation()
}
```

**Hậu quả:** App POS/Smart TV dùng Android View bị ép đóng gói Compose (~15MB).
**Lợi ích:** Giảm APK size, giảm build time nhờ Compile Avoidance.

---

## Vi phạm 2: api() leak thư viện qua build.gradle.kts (IMG-01)

### ❌ BEFORE
```kotlin
// :modules:image:image-ui-port/build.gradle.kts
dependencies {
    api(libs.androidx.compose.ui) // ❌ image-ui-port ép consumer nuốt Compose
    api(libs.coil.compose)        // ❌ Leak implementation detail
}
```

### ✅ AFTER
```kotlin
dependencies {
    implementation(libs.androidx.compose.ui) // ✅ Chỉ dùng nội bộ
    implementation(libs.coil.compose)         // ✅ Consumer không biết dùng Coil
}

// Interface thuần Kotlin, không leak Compose type:
interface ImageLoader {
    suspend fun load(url: String): ImageResult
}
```

**Test nhanh:** Thử đổi `api(...)` → `implementation(...)`. Compile vẫn pass → `api` thừa, đổi luôn.

---

## Vi phạm 3: Fat interface > 5 methods (PSL-A-14)

### ❌ BEFORE
```kotlin
// Interface 8 method — client Analytics chỉ cần 2, client UI cần 3, client Export cần 1
interface DeviceManager {
    fun getDevice(id: String): Device
    fun getAllDevices(): List<Device>
    fun addDevice(device: Device)
    fun removeDevice(id: String)
    fun updateDeviceName(id: String, name: String)
    fun syncDeviceState(id: String)
    fun exportDeviceConfig(id: String): ByteArray
    fun importDeviceConfig(config: ByteArray)
}
```

### ✅ AFTER — Tách theo use case
```kotlin
// Client nào cần gì inject cái đó
interface DeviceReader {
    fun getDevice(id: String): Device
    fun getAllDevices(): List<Device>
}

interface DeviceWriter {
    fun addDevice(device: Device)
    fun removeDevice(id: String)
    fun updateDeviceName(id: String, name: String)
}

interface DeviceSync {
    fun syncDeviceState(id: String)
}

interface DeviceConfigManager {
    fun exportDeviceConfig(id: String): ByteArray
    fun importDeviceConfig(config: ByteArray)
}
```

---

## Vi phạm 4: Interface/sealed class rỗng 0 member (marker interface vô nghĩa)

### ❌ BEFORE
```kotlin
// Marker interface không có method nào — ép consumer implement form thừa
sealed interface AppEvent  // ❌ Rỗng, 0 member

interface Trackable // ❌ Rỗng, chỉ dùng để instanceOf check
```

### ✅ AFTER
```kotlin
// Nếu cần marker, dùng annotation thay interface:
@Target(AnnotationTarget.CLASS)
annotation class Trackable

// Nếu sealed cần shared behavior:
sealed interface AppEvent {
    val timestamp: Long // ✅ Có member có nghĩa
}
```

---

## Vi phạm 5: Fragment/Activity implement Domain interface (NET-03)

### ❌ BEFORE
```kotlin
// Fragment implement domain interface → Hilt phải khởi tạo cả God Fragment
interface NetworkStatusListener {
    fun onNetworkAvailable()
    fun onNetworkLost()
}

class HomeFragment : Fragment(), NetworkStatusListener { // ❌
    // Client inject NetworkStatusListener nhưng Hilt dựng cả Fragment nặng nề
    override fun onNetworkAvailable() { updateUI() }
    override fun onNetworkLost() { showOfflineBanner() }
}
```

### ✅ AFTER
```kotlin
// Tách listener thành class riêng
class HomeNetworkHandler @Inject constructor() : NetworkStatusListener {
    private val _networkState = MutableStateFlow(true)
    val networkState: StateFlow<Boolean> = _networkState.asStateFlow()
    
    override fun onNetworkAvailable() { _networkState.value = true }
    override fun onNetworkLost() { _networkState.value = false }
}

// Fragment chỉ observe state
class HomeFragment : Fragment() {
    @Inject lateinit var networkHandler: HomeNetworkHandler
    // observe networkHandler.networkState
}
```

---

## Vi phạm 6: @Composable function nhận > 10 tham số

### ❌ BEFORE
```kotlin
@Composable
fun ProductCard(
    title: String,
    price: Double,
    imageUrl: String,
    discount: Double?,
    rating: Float,
    reviewCount: Int,
    isWishlisted: Boolean,
    isInCart: Boolean,
    badgeText: String?,
    sellerName: String,
    deliveryDate: String,
    onAddToCart: () -> Unit,
    onWishlistToggle: () -> Unit,
    onClick: () -> Unit,
) { ... } // ❌ 14 tham số!
```

### ✅ AFTER
```kotlin
data class ProductCardState(
    val title: String,
    val price: Double,
    val imageUrl: String,
    val discount: Double? = null,
    val rating: Float = 0f,
    val reviewCount: Int = 0,
    val isWishlisted: Boolean = false,
    val isInCart: Boolean = false,
    val badgeText: String? = null,
    val sellerName: String = "",
    val deliveryDate: String = "",
)

@Composable
fun ProductCard(
    state: ProductCardState,
    onAddToCart: () -> Unit,
    onWishlistToggle: () -> Unit,
    onClick: () -> Unit,
) { ... } // ✅ 4 tham số
```

---

## Grep commands phát hiện

```bash
# Tìm api() trong build files
grep -rn "api(" --include="*.kts" . | grep -v "implementation" | grep -v "//.*api"

# @Composable trên interface/abstract ở core/foundation
grep -B1 "@Composable" --include="*.kt" -rn core/ foundation/ | grep "interface\|abstract\|open"

# Interface rỗng
grep -A 3 "^interface \|^sealed " --include="*.kt" -rn . | grep -B1 "^--$\|^}"

# Fragment/Activity implement interface
grep -rn "class.*Fragment.*:.*\|class.*Activity.*:" --include="*.kt" . | grep -v "AppCompat\|Base"

# Composable với nhiều tham số
grep -A 5 "@Composable" --include="*.kt" -rn . | grep "fun " | awk '{print NF}' | sort -rn | head -10
```
