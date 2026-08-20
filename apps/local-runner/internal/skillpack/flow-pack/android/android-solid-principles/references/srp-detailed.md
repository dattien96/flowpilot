# SRP Chi Tiết — Đơn Trách Nhiệm trong Android

> *"Một thành phần chỉ nên có một lý do duy nhất để thay đổi."*

---

## Câu hỏi chẩn đoán

| Scope | Câu hỏi |
|---|---|
| Feature | Hàm/Class này có bị ôm vừa Logic nghiệp vụ vừa Logic UI / State / Formatting không? |
| Module | Module này sở hữu duy nhất 1 năng lực (capability) hay đang thành "bãi rác" gom nhiều thứ? |

---

## Vi phạm 1: God ViewModel dính Context (PSL-A-07, HOME-01)

### ❌ BEFORE
```kotlin
// ViewModel vừa fetch API, vừa format UI text, vừa dính Context/Resource ID
class ProductViewModel @Inject constructor(
    private val repository: ProductRepository,
    @ApplicationContext private val context: Context, // ❌ Dính Android Context
) : ViewModel() {
    val productPriceText = mutableStateOf("")
    val iconResId = mutableStateOf(0)

    fun loadProduct(id: String) {
        viewModelScope.launch {
            val product = repository.getProduct(id)
            // ❌ Format UI text trực tiếp
            productPriceText.value = "$${String.format("%.2f", product.price)}"
            // ❌ R.drawable trong ViewModel — không Unit Test trên JVM được
            iconResId.value = if (product.isVip) R.drawable.ic_vip else R.drawable.ic_normal
            Toast.makeText(context, "Loaded", Toast.LENGTH_SHORT).show()
        }
    }
}
```

### ✅ AFTER
```kotlin
data class ProductUiState(
    val price: Double = 0.0,
    val isVip: Boolean = false,
)

class ProductViewModel @Inject constructor(
    private val repository: ProductRepository,
) : ViewModel() {
    private val _uiState = MutableStateFlow(ProductUiState())
    val uiState: StateFlow<ProductUiState> = _uiState.asStateFlow()

    fun loadProduct(id: String) {
        viewModelScope.launch {
            val product = repository.getProduct(id)
            _uiState.update { ProductUiState(price = product.price, isVip = product.isVip) }
        }
    }
}

// Tầng UI:
@Composable
fun ProductScreen(viewModel: ProductViewModel) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val formattedPrice = remember(state.price) { "$${String.format("%.2f", state.price)}" }
    val iconRes = if (state.isVip) R.drawable.ic_vip else R.drawable.ic_normal
    // Render...
}
```

**Hậu quả:** Không thể Unit Test trên JVM thuần (phải Emulator, chậm 100x).
**Lợi ích:** ViewModel test JVM 5ms. Tách biệt hoàn toàn Data & UI Rendering.

---

## Vi phạm 2: God Fragment 472 LOC (PSL-A-05)

### ❌ BEFORE
```kotlin
// Fragment 472 dòng vừa inflate layout, vừa validate form, vừa gọi API, vừa navigate
class AddDeviceFragment : Fragment() {
    // ... 15 lateinit var views
    // ... onClick listeners gọi thẳng repository
    // ... validation logic nhúng trong UI
    // ... navigation logic nhúng trong onClick
    // Tổng: 472 LOC, 8 import tầng khác nhau
}
```

### ✅ AFTER
```kotlin
// Fragment chỉ còn ~80 LOC: subscribe state, dispatch event
class AddDeviceFragment : Fragment() {
    private val viewModel: AddDeviceViewModel by viewModels()

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        viewModel.uiState.collectWithLifecycle(viewLifecycleOwner) { state ->
            // render UI from state
        }
        binding.btnSubmit.setOnClickListener {
            viewModel.onSubmit() // dispatch event, không xử lý ở đây
        }
    }
}

// Validation nằm ở domain:
class ValidateDeviceInput @Inject constructor() {
    operator fun invoke(input: DeviceInput): ValidationResult { ... }
}

// Navigation nằm ở ViewModel:
class AddDeviceViewModel @Inject constructor(
    private val validate: ValidateDeviceInput,
    private val addDevice: AddDeviceUseCase,
) : ViewModel() { ... }
```

---

## Vi phạm 3: Repository ôm quá nhiều nguồn data (DB-01)

### ❌ BEFORE
```kotlin
// Repository vừa gọi Retrofit, vừa đọc Room, vừa SharedPreferences
class HomeRepository @Inject constructor(
    private val api: HomeApi,
    private val dao: HomeDao,
    private val prefs: SharedPreferences,  // ❌ 3 nguồn data trong 1 class
    private val cache: MemoryCache,
    private val analytics: AnalyticsTracker, // ❌ cross-cutting concern
) {
    // 15 hàm, 300 LOC
}
```

### ✅ AFTER
```kotlin
// Tách thành datasource interfaces do domain định nghĩa
interface HomeRemoteDataSource {
    suspend fun fetchFeed(): List<HomeFeedItem>
}

interface HomeLocalDataSource {
    fun observeFeed(): Flow<List<HomeFeedItem>>
    suspend fun saveFeed(items: List<HomeFeedItem>)
}

// Repository chỉ orchestrate:
class HomeRepositoryImpl @Inject constructor(
    private val remote: HomeRemoteDataSource,
    private val local: HomeLocalDataSource,
) : HomeRepository {
    override fun getFeed(): Flow<List<HomeFeedItem>> = ...
}
```

---

## Vi phạm 4: CQS Violation — Hàm query mà mutate state

### ❌ BEFORE
```kotlin
// Hàm tên "check" nhưng thân sửa state
fun checkDeviceStatus(): Boolean {
    val status = repository.getStatus()
    _currentStatus.value = status   // ❌ Side effect! Query function đang mutate
    _lastChecked.value = System.currentTimeMillis()
    return status.isActive
}
```

### ✅ AFTER
```kotlin
// Tách command và query
fun refreshDeviceStatus() {             // Command — mutate state
    val status = repository.getStatus()
    _currentStatus.value = status
    _lastChecked.value = System.currentTimeMillis()
}

fun isDeviceActive(): Boolean {         // Query — thuần túy
    return _currentStatus.value.isActive
}
```

---

## Vi phạm 5: Mutable state rò rỉ ra ngoài

### ❌ BEFORE
```kotlin
class HomeViewModel : ViewModel() {
    val items = MutableStateFlow<List<Product>>(emptyList()) // ❌ Public Mutable!
    var isLoading = true    // ❌ var public
    var errorMessage = ""   // ❌ var public
}
// Bất kỳ class nào cũng: viewModel.items.value = emptyList() — phá encapsulation
```

### ✅ AFTER
```kotlin
class HomeViewModel : ViewModel() {
    private val _items = MutableStateFlow<List<Product>>(emptyList())
    val items: StateFlow<List<Product>> = _items.asStateFlow() // ✅ Read-only

    private val _uiState = MutableStateFlow(HomeUiState())
    val uiState: StateFlow<HomeUiState> = _uiState.asStateFlow()
}
```

---

## Grep commands phát hiện

```bash
# Đếm LOC — file > 400 LOC gần như chắc chắn vi phạm SRP
find . -name "*.kt" -exec wc -l {} + | sort -rn | head -20

# ViewModel import Context/R.*
grep -rn "import android.content\|import.*\.R$\|import.*\.R\." --include="*.kt" . | grep -i "viewmodel\|vm"

# Constructor quá nhiều tham số (> 7 = red flag)
grep -A 10 "@Inject" --include="*.kt" -rn . | grep "constructor"

# MutableStateFlow/LiveData bị expose public
grep -rn "val.*MutableStateFlow\|val.*MutableLiveData" --include="*.kt" . | grep -v "private\|internal\|_"

# Data layer import R.string/R.drawable
grep -rn "import.*\.R\." --include="*.kt" . | grep -i "data\|repository\|datasource\|domain"
```
