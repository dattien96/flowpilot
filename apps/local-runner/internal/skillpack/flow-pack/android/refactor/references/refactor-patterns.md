# Refactor Examples

## Example 1: Extract Component Pattern

### Before
```kotlin
// File: features/presentation/profile/screen/ProfileScreen.kt (400 lines)
@Composable
fun ProfileScreen(
    viewModel: ProfileViewModel = koinViewModel(),
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsState()
    
    Column(modifier = modifier) {
        // Header section (lines 50-120)
        Row {
            Avatar(url = uiState.avatarUrl)
            Column {
                Text(uiState.name)
                Text(uiState.email)
            }
        }
        
        // Info section (lines 130-220)
        Card {
            Row {
                Icon(Icons.Default.Phone)
                Text(uiState.phone)
            }
            Row {
                Icon(Icons.Default.LocationOn)
                Text(uiState.address)
            }
        }
        
        // Actions section (lines 230-320)
        Row {
            Button(onClick = { viewModel.handleIntent(EditProfile) }) {
                Text("Edit Profile")
            }
            Button(onClick = { viewModel.handleIntent(Logout) }) {
                Text("Logout")
            }
        }
    }
}
```

### After
```kotlin
// File: features/presentation/profile/screen/ProfileScreen.kt (80 lines)
@Composable
fun ProfileScreen(
    viewModel: ProfileViewModel = koinViewModel(),
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsState()
    
    Column(modifier = modifier) {
        ProfileHeader(avatarUrl = uiState.avatarUrl, name = uiState.name, email = uiState.email)
        ProfileInfoSection(phone = uiState.phone, address = uiState.address)
        ProfileActionsSection(onEdit = { viewModel.handleIntent(EditProfile) }, onLogout = { viewModel.handleIntent(Logout) })
    }
}
```

```kotlin
// File: features/presentation/profile/components/ProfileHeader.kt
@Composable
internal fun ProfileHeader(
    avatarUrl: String,
    name: String,
    email: String,
    modifier: Modifier = Modifier
) {
    Row(modifier = modifier) {
        Avatar(url = avatarUrl)
        Column {
            Text(name)
            Text(email)
        }
    }
}
```

```kotlin
// File: features/presentation/profile/components/ProfileInfoSection.kt
@Composable
internal fun ProfileInfoSection(
    phone: String,
    address: String,
    modifier: Modifier = Modifier
) {
    Card(modifier = modifier) {
        Row {
            Icon(Icons.Default.Phone)
            Text(phone)
        }
        Row {
            Icon(Icons.Default.LocationOn)
            Text(address)
        }
    }
}
```

```kotlin
// File: features/presentation/profile/components/ProfileActionsSection.kt
@Composable
internal fun ProfileActionsSection(
    onEdit: () -> Unit,
    onLogout: () -> Unit,
    modifier: Modifier = Modifier
) {
    Row(modifier = modifier) {
        Button(onClick = onEdit) {
            Text("Edit Profile")
        }
        Button(onClick = onLogout) {
            Text("Logout")
        }
    }
}
```

---

## Example 2: Extract Use Case Pattern

### Before
```kotlin
// File: features/presentation/login/LoginViewModel.kt
class LoginViewModel(
    private val authRepository: AuthRepository
) : ViewModel() {
    
    private val _uiState = MutableStateFlow<LoginState>(LoginState.Idle)
    val uiState: StateFlow<LoginState> = _uiState.asStateFlow()
    
    fun login(email: String, password: String) {
        viewModelScope.launch {
            // Validation logic in ViewModel ❌
            if (!isValidEmail(email)) {
                _uiState.value = LoginState.Error("Invalid email")
                return@launch
            }
            
            if (password.length < 8) {
                _uiState.value = LoginState.Error("Password too short")
                return@launch
            }
            
            // Business logic in ViewModel ❌
            _uiState.value = LoginState.Loading
            val result = authRepository.login(email, password)
            
            when (result) {
                is Result.Success -> _uiState.value = LoginState.Success(result.data)
                is Result.Error -> _uiState.value = LoginState.Error(result.message)
            }
        }
    }
    
    private fun isValidEmail(email: String): Boolean {
        // Complex email validation logic
        return Patterns.EMAIL_ADDRESS.matcher(email).matches()
    }
}
```

### After
```kotlin
// File: features/domain/usecase/ValidateLoginCredentialsUseCase.kt
class ValidateLoginCredentialsUseCase {
    operator fun invoke(email: String, password: String): ValidationResult {
        if (!isValidEmail(email)) {
            return ValidationResult.Error("Invalid email")
        }
        
        if (password.length < 8) {
            return ValidationResult.Error("Password must be at least 8 characters")
        }
        
        return ValidationResult.Success
    }
    
    private fun isValidEmail(email: String): Boolean {
        return Patterns.EMAIL_ADDRESS.matcher(email).matches()
    }
}

// File: features/domain/usecase/LoginUseCase.kt
class LoginUseCase(
    private val authRepository: AuthRepository,
    private val validateCredentials: ValidateLoginCredentialsUseCase
) {
    suspend operator fun invoke(email: String, password: String): LoginResult {
        val validation = validateCredentials(email, password)
        if (validation is ValidationResult.Error) {
            return LoginResult.Error(validation.message)
        }
        
        return try {
            val user = authRepository.login(email, password)
            LoginResult.Success(user)
        } catch (e: Exception) {
            LoginResult.Error(e.message ?: "Login failed")
        }
    }
}

// File: features/presentation/login/LoginViewModel.kt
class LoginViewModel(
    private val loginUseCase: LoginUseCase
) : ViewModel() {
    
    private val _uiState = MutableStateFlow<LoginState>(LoginState.Idle)
    val uiState: StateFlow<LoginState> = _uiState.asStateFlow()
    
    fun login(email: String, password: String) {
        viewModelScope.launch {
            _uiState.value = LoginState.Loading
            val result = loginUseCase(email, password)
            
            when (result) {
                is LoginResult.Success -> _uiState.value = LoginState.Success(result.user)
                is LoginResult.Error -> _uiState.value = LoginState.Error(result.message)
            }
        }
    }
}
```

---

## Example 3: Migrate to StateFlow Pattern

### Before (LiveData)
```kotlin
// File: features/presentation/home/HomeViewModel.kt
class HomeViewModel(
    private val repository: HomeRepository
) : ViewModel() {
    
    private val _items = MutableLiveData<List<Item>>()
    val items: LiveData<List<Item>> = _items
    
    private val _loading = MutableLiveData<Boolean>()
    val loading: LiveData<Boolean> = _loading
    
    fun loadItems() {
        viewModelScope.launch {
            _loading.value = true
            val result = repository.getItems()
            _items.value = result
            _loading.value = false
        }
    }
}

// File: features/presentation/home/HomeScreen.kt
@Composable
fun HomeScreen(
    viewModel: HomeViewModel = koinViewModel()
) {
    val items by viewModel.items.observeAsState(emptyList())
    val loading by viewModel.loading.observeAsState(false)
    
    if (loading) {
        LoadingScreen()
    } else {
        ItemList(items = items)
    }
}
```

### After (StateFlow)
```kotlin
// File: features/presentation/home/HomeViewModel.kt
class HomeViewModel(
    private val repository: HomeRepository
) : ViewModel() {
    
    sealed class HomeState {
        object Loading : HomeState()
        data class Success(val items: List<Item>) : HomeState()
        data class Error(val message: String) : HomeState()
    }
    
    private val _state = MutableStateFlow<HomeState>(HomeState.Loading)
    val state: StateFlow<HomeState> = _state.asStateFlow()
    
    fun loadItems() {
        viewModelScope.launch {
            _state.value = HomeState.Loading
            try {
                val items = repository.getItems()
                _state.value = HomeState.Success(items)
            } catch (e: Exception) {
                _state.value = HomeState.Error(e.message ?: "Failed to load")
            }
        }
    }
}

// File: features/presentation/home/HomeScreen.kt
@Composable
fun HomeScreen(
    viewModel: HomeViewModel = koinViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    
    when (state) {
        is HomeState.Loading -> LoadingScreen()
        is HomeState.Success -> ItemList(items = (state as HomeState.Success).items)
        is HomeState.Error -> ErrorScreen(message = (state as HomeState.Error).message)
    }
}
```

---

## Example 4: Simplify Conditional Logic Pattern

### Before (Complex Nested If)
```kotlin
// File: features/domain/usecase/CalculateDiscountUseCase.kt
class CalculateDiscountUseCase {
    fun calculate(user: User, cartTotal: Double): Double {
        if (user.isPremium) {
            if (user.hasCoupon) {
                if (cartTotal > 100) {
                    return 0.3 // 30% discount
                } else {
                    return 0.2 // 20% discount
                }
            } else {
                return 0.1 // 10% discount
            }
        } else {
            if (cartTotal > 200) {
                return 0.05 // 5% discount for non-premium
            } else {
                return 0.0 // No discount
            }
        }
    }
}
```

### After (Polymorphism / Sealed Class)
```kotlin
// File: features/domain/model/DiscountStrategy.kt
sealed class DiscountStrategy {
    abstract fun calculate(cartTotal: Double): Double
}

class PremiumWithCoupon : DiscountStrategy() {
    override fun calculate(cartTotal: Double): Double {
        return if (cartTotal > 100) 0.3 else 0.2
    }
}

class PremiumWithoutCoupon : DiscountStrategy() {
    override fun calculate(cartTotal: Double): Double = 0.1
}

class NonPremium : DiscountStrategy() {
    override fun calculate(cartTotal: Double): Double {
        return if (cartTotal > 200) 0.05 else 0.0
    }
}

// File: features/domain/usecase/CalculateDiscountUseCase.kt
class CalculateDiscountUseCase {
    fun calculate(user: User, cartTotal: Double): Double {
        val strategy = getStrategy(user)
        return strategy.calculate(cartTotal)
    }
    
    private fun getStrategy(user: User): DiscountStrategy {
        return when {
            user.isPremium && user.hasCoupon -> PremiumWithCoupon()
            user.isPremium -> PremiumWithoutCoupon()
            else -> NonPremium()
        }
    }
}
```

---

## Example 5: Rename for Clarity Pattern

### Before
```kotlin
// ❌ Bad naming
fun data() { }  // What does this return?
class Manager { }  // What does it manage?
val usr = getUser()  // Abbreviation
fun proc() { }  // Unclear abbreviation
```

### After
```kotlin
// ✅ Clear naming
fun calculateUserData(): UserData { }
class UserProfileManager { }
val user = getUser()
fun processPayment() { }
```

---

## Example 6: Remove Duplication (DRY) Pattern

### Before (Duplication)
```kotlin
// File: features/presentation/login/LoginForm.kt
@Composable
fun LoginForm(onLogin: (String, String) -> Unit) {
    Column {
        // Email field (duplicated)
        Column {
            Text("Email", style = MaterialTheme.typography.labelLarge)
            OutlinedTextField(
                value = email,
                onValueChange = { email = it },
                isError = emailError != null
            )
            if (emailError != null) {
                Text(emailError!!, color = MaterialTheme.colorScheme.error)
            }
        }
        
        // Password field (duplicated)
        Column {
            Text("Password", style = MaterialTheme.typography.labelLarge)
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                isError = passwordError != null
            )
            if (passwordError != null) {
                Text(passwordError!!, color = MaterialTheme.colorScheme.error)
            }
        }
    }
}

// File: features/presentation/signup/SignupForm.kt
@Composable
fun SignupForm(onSignup: (String, String, String) -> Unit) {
    Column {
        // Email field (same as Login)
        Column {
            Text("Email", style = MaterialTheme.typography.labelLarge)
            OutlinedTextField(
                value = email,
                onValueChange = { email = it },
                isError = emailError != null
            )
            if (emailError != null) {
                Text(emailError!!, color = MaterialTheme.colorScheme.error)
            }
        }
        
        // Password field (same as Login)
        Column {
            Text("Password", style = MaterialTheme.typography.labelLarge)
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                isError = passwordError != null
            )
            if (passwordError != null) {
                Text(passwordError!!, color = MaterialTheme.colorScheme.error)
            }
        }
    }
}
```

### After (Extracted Common Component)
```kotlin
// File: core-foundation/modules/appdesign/widgets/CommonInputField.kt
@Composable
fun CommonInputField(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    isError: Boolean = false,
    errorMessage: String? = null,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default
) {
    Column(modifier = modifier) {
        Text(label, style = MaterialTheme.typography.labelLarge)
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            isError = isError,
            keyboardOptions = keyboardOptions
        )
        if (isError && errorMessage != null) {
            Text(errorMessage, color = MaterialTheme.colorScheme.error)
        }
    }
}

// File: features/presentation/login/LoginForm.kt
@Composable
fun LoginForm(onLogin: (String, String) -> Unit) {
    Column {
        CommonInputField(
            label = "Email",
            value = email,
            onValueChange = { email = it },
            isError = emailError != null,
            errorMessage = emailError
        )
        CommonInputField(
            label = "Password",
            value = password,
            onValueChange = { password = it },
            isError = passwordError != null,
            errorMessage = passwordError,
            keyboardOptions = KeyboardOptions.Password
        )
    }
}
```
