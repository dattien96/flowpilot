# 1. Visibility Modifiers (Access Control)

**1.1. `public` (default) - Screen Function**

```kotlin
// ✅ CORRECT - public Screen
@Composable
fun HomeScreen(
    viewModel: HomeViewModel = koinViewModel(),
    modifier: Modifier = Modifier
) {
    val state by viewModel.state.collectAsState()
    HomeContent(state, modifier, onIntent = viewModel::processIntent)
}
```

**1.2 . `internal` - Content/SuccessView Functions**
```kotlin
// ✅ CORRECT - internal Content (testable, previewable)
@Composable
internal fun HomeContent(
    state: BaseScreenState<HomeScreenSuccessDataState>,
    modifier: Modifier = Modifier,
    onIntent: (HomeIntent) -> Unit
) {
    when (state) {
        is BaseScreenState.Loading -> LoadingScreen(modifier)
        is BaseScreenState.Success -> HomeSuccessView(state.data, onIntent, modifier)
        is BaseScreenState.Error -> ErrorScreen(state.messageErrorResId, onRetry = {}, modifier)
    }
}

// ✅ CORRECT - internal SuccessView (testable, previewable)
@Composable
internal fun HomeSuccessView(
    data: HomeScreenSuccessDataState,
    onIntent: (HomeIntent) -> Unit,
    modifier: Modifier = Modifier
) {
    Box(modifier = modifier.fillMaxSize()) {
        // UI implementation
    }
}
```

**1.3. `private` - Helper Components**

```kotlin
// ✅ CORRECT - private helper (used only in this file)
@Composable
private fun HomeMetricCard(
    title: String,
    value: String,
    modifier: Modifier = Modifier
) {
    Card(modifier = modifier) {
        Text(title)
        Text(value)
    }
}
```

# 2. Why `internal` instead of `private` for Content/SuccessView?
## 2.1. **Testing**: Allows testing Content/SuccessView without a ViewModel.
```kotlin
// HomeScreenTest.kt (in commonTest)
@Test
fun `test HomeContent displays loading state`() {
    // ✅ OK - internal is accessible within the same module
    HomeContent(
        state = BaseScreenState.Loading,
        onIntent = {}
    )
}
```

## 2.2. **Preview**: Allows creating previews for each state.
```kotlin
// HomeScreenPreview.kt
@Preview
@Composable
fun HomeContentLoadingPreview() {
    // ✅ OK - internal is accessible
    HomeContent(
        state = BaseScreenState.Loading,
        onIntent = {}
    )
}
```

## 2.3. **Encapsulation**: External modules only see the public Screen function.
```
Module A → calls HomeScreen() ✅
Module A → CANNOT call HomeContent() ❌ (internal)
```

# 3. Distinguishing Screen vs. Component
```kotlin
✅ CORRECT - Screen:
@Composable
fun LoginScreen(
    viewModel: LoginViewModel = koinViewModel(),
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsState()
    LoginContent(uiState = uiState, onIntent = viewModel::handleIntent)
}

❌ INCORRECT - Named Screen but no ViewModel:
@Composable
fun MealRecommendScreen(
    meals: List<Meal>,  // Receives data directly
    onAccept: (String) -> Unit,
    modifier: Modifier = Modifier
) { /* ... */ }

✅ CORRECT - Renamed to Component:
@Composable
fun MealRecommendComponent(  // Renamed from Screen → Component
    meals: List<Meal>,
    onAccept: (String) -> Unit,
    modifier: Modifier = Modifier
) { /* ... */ }
// File moved to /components/
```