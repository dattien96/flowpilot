# OCP Chi Tiết — Mở rộng / Đóng sửa đổi trong Android

> *"Mở rộng tính năng bằng cách THÊM CODE MỚI, tuyệt đối không SỬA CODE CŨ."*

---

## Câu hỏi chẩn đoán

| Scope | Câu hỏi |
|---|---|
| Module | Khi thêm 1 Feature Module mới (`:feature:settings`), ta có phải sửa 1 dòng code nào ở `:core` hay `:app` không? |
| Class | Khi thêm 1 biến thể mới, ta có phải mở file base class/sealed class ra sửa không? |

---

## Vi phạm 1: sealed interface AppRoute trong :core (NAV-01)

### ❌ BEFORE
```kotlin
// :core:navigation — Kotlin `sealed` bắt buộc tất cả subclass cùng module
sealed interface AppRoute : NavKey {
    data object HomeRoute : AppRoute
    data class ProductDetailRoute(val id: String) : AppRoute
    // Thêm SettingsRoute? → BẮT BUỘC mở file core ra sửa!
}

// AppNavHost.kt — mỗi màn hình mới = thêm branch when
@Composable
fun AppNavHost(route: AppRoute) {
    when (route) { // ❌ File này phình mãi
        is HomeRoute -> HomeScreen()
        is ProductDetailRoute -> ProductDetailScreen(route.id)
    }
}
```

### ✅ AFTER — Pluggable NavGraph & Hilt Multibindings
```kotlin
// :core:navigation — ĐÓNG. Interface mở, không sealed.
interface AppRoute : NavKey
interface AppFeatureNavGraph {
    fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput)
}

// :feature:home — tự đăng ký Route & Graph
data object HomeRoute : AppRoute
class HomeFeatureNavGraph @Inject constructor() : AppFeatureNavGraph {
    override fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput) {
        entry<HomeRoute> { HomeScreen() }
    }
}

// :feature:settings — THÊM module mới, 0% đụng core
data object SettingsRoute : AppRoute
class SettingsFeatureNavGraph @Inject constructor() : AppFeatureNavGraph {
    override fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput) {
        entry<SettingsRoute> { SettingsScreen() }
    }
}

// :core:navigation — nhận set injection, không biết feature nào tồn tại
@Composable
fun AppNavHost(featureGraphs: Set<AppFeatureNavGraph>) {
    NavDisplay(
        entryProvider = entryProvider {
            featureGraphs.forEach { graph -> with(graph) { registerEntries(input) } }
        }
    )
}
```

**Hậu quả:** Dev settings bị nghẽn tiến độ chờ sửa core. Sửa `when` rủi ro crash màn cũ.
**Lợi ích:** Thêm feature 100% bằng module mới, 0% đụng code cũ.

---

## Vi phạm 2: when-block explosion (SRCH-03)

### ❌ BEFORE
```kotlin
// File filter handler phình theo số loại filter
fun applyFilter(filter: SearchFilter, query: SearchQuery): SearchQuery {
    return when (filter) {
        is PriceFilter -> query.copy(minPrice = filter.min, maxPrice = filter.max)
        is CategoryFilter -> query.copy(categoryId = filter.id)
        is BrandFilter -> query.copy(brandId = filter.id)
        is RatingFilter -> query.copy(minRating = filter.rating)
        // ... thêm 10 loại filter nữa → file phình 200 LOC
        // else -> query // ❌ nuốt filter mới im lặng
    }
}
```

### ✅ AFTER — Strategy Pattern
```kotlin
// Mỗi filter tự biết cách apply
fun interface FilterApplier {
    fun apply(filter: SearchFilter, query: SearchQuery): SearchQuery
}

// Đăng ký qua Hilt Multibindings hoặc Map
class PriceFilterApplier @Inject constructor() : FilterApplier {
    override fun apply(filter: SearchFilter, query: SearchQuery): SearchQuery {
        val priceFilter = filter as PriceFilter
        return query.copy(minPrice = priceFilter.min, maxPrice = priceFilter.max)
    }
}

// Orchestrator không biết filter nào tồn tại
class FilterEngine @Inject constructor(
    private val appliers: Map<String, @JvmSuppressWildcards FilterApplier>
) {
    fun applyAll(filters: List<SearchFilter>, query: SearchQuery): SearchQuery {
        return filters.fold(query) { acc, filter ->
            appliers[filter.type]?.apply(filter, acc) ?: acc
        }
    }
}
```

---

## Vi phạm 3: Copy-paste giữa feature (PSL-A-21)

### ❌ BEFORE
```kotlin
// AddDeviceStepChain.kt — 200 LOC
class AddDeviceStepChain {
    fun execute(steps: List<Step>) {
        // validate → collect data → submit → show result
        // logic gần giống hệt MirroringDeviceStepChain
    }
}

// MirroringDeviceStepChain.kt — 180 LOC (copy-paste 80%)
class MirroringDeviceStepChain {
    fun execute(steps: List<Step>) {
        // validate → collect data → submit → show result
        // Chỉ khác ở bước submit
    }
}
```

### ✅ AFTER — Template Method / Composition
```kotlin
// Base chain xử lý flow chung
abstract class DeviceStepChain {
    fun execute(steps: List<Step>) {
        validate(steps)
        val data = collectData(steps)
        submit(data) // abstract — mỗi loại tự implement
        showResult()
    }
    
    protected abstract fun submit(data: DeviceData)
    // ... validate, collectData, showResult là chung
}

class AddDeviceStepChain @Inject constructor() : DeviceStepChain() {
    override fun submit(data: DeviceData) { /* add logic */ }
}

class MirroringDeviceStepChain @Inject constructor() : DeviceStepChain() {
    override fun submit(data: DeviceData) { /* mirroring logic */ }
}
```

---

## Vi phạm 4: Boolean flag trên base class → Class Explosion

### ❌ BEFORE
```kotlin
abstract class BasePaymentProcessor {
    abstract val shouldValidateCard: Boolean    // ❌ toggle behavior
    abstract val shouldSendReceipt: Boolean     // ❌ toggle behavior
    abstract val shouldNotifyMerchant: Boolean  // ❌ toggle behavior
    
    fun process(payment: Payment) {
        if (shouldValidateCard) validateCard(payment)
        // ... sử dụng flag để rẽ nhánh
        if (shouldSendReceipt) sendReceipt(payment)
        if (shouldNotifyMerchant) notifyMerchant(payment)
    }
}
// 3 boolean = 8 tổ hợp → 8 subclass tiềm năng = Class Explosion
```

### ✅ AFTER — Composition over Inheritance
```kotlin
fun interface PaymentStep {
    suspend fun execute(payment: Payment)
}

class CardValidator @Inject constructor() : PaymentStep { ... }
class ReceiptSender @Inject constructor() : PaymentStep { ... }
class MerchantNotifier @Inject constructor() : PaymentStep { ... }

class PaymentProcessor @Inject constructor(
    private val steps: List<@JvmSuppressWildcards PaymentStep>
) {
    suspend fun process(payment: Payment) {
        steps.forEach { it.execute(payment) }
    }
}

// DI config chọn steps cho từng loại thanh toán — không cần subclass
```

---

## Grep commands phát hiện

```bash
# sealed trong core/foundation
grep -rn "sealed " --include="*.kt" core/ foundation/

# when blocks lớn (> 10 branches)
grep -c "is \|-> " --include="*.kt" -rn . | awk -F: '$NF > 10 {print}'

# Copy-paste: class tên giống nhau ở 2 feature
find . -name "*.kt" | xargs basename -a | sort | uniq -d

# Boolean flag trên base class
grep -rn "abstract.*val.*:.*Boolean\|abstract.*fun.*:.*Boolean" --include="*.kt" . | grep -i "base\|abstract"
```
