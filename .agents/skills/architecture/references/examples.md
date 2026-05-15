# 1. SOLID Principles Examples

### Liskov Substitution Principle (LSP)
**Whenever creating a subclass, ask yourself:**
- _Am I forced to use all of the parent's functions?_ (If not ➔ **Red Flag 1** ➔ Separate into interfaces).
- _Does the caller need to know who I am?_ (If `is MyClass` checks are required ➔ **Red Flag 2** ➔ Push logic into Polymorphism).
- _Does my overridden function cause the app to crash in situations where its parent runs normally?_ (If so ➔ **Red Flag 3** ➔ Logic correction required).

# 2. Screen Architecture Pattern
## 2.1 XxxScreen - Entry Point

```kotlin
@Composable
fun SettingScreen(
    viewModel: SettingViewModel = koinViewModel(),
    modifier: Modifier = Modifier
) {
    val uiState by viewModel.uiState.collectAsState()

    SettingContent(
        uiState = uiState,
        onIntent = viewModel::handleIntent,
        modifier = modifier
    )
}
```

## 2.2 XxxContent - State Handler
**Case A: NOT observing BaseScreenState**
```kotlin
@Composable
internal fun SettingContent(
    userData: UserData,
    onIntent: (SettingIntent) -> Unit,
    modifier: Modifier = Modifier
) {
    // Render UI directly
    Column(modifier = modifier) {
        // UI implementation
    }
}
```

**Case B: Observing BaseScreenState**
```kotlin
@Composable
internal fun SettingContent(
    uiState: BaseScreenState<SettingData>,
    onIntent: (SettingIntent) -> Unit,
    modifier: Modifier = Modifier
) {
    when (uiState) {
        is BaseScreenState.Loading -> {
            LoadingScreen(modifier = modifier)
        }
        is BaseScreenState.Error -> {
            ErrorScreen(
                errorStringResId = uiState.messageErrorResId,
                onRetry = { onIntent(XxxIntent.Retry) },
                modifier = modifier
            )
        }
        is BaseScreenState.Success -> {
            SettingSuccessView(
                data = uiState.data,
                onIntent = onIntent,
                modifier = modifier
            )
        }
    }
}
```

## 2.3 XxxSuccessView - Final UI (Only if BaseScreenState)

```kotlin
@Composable
internal fun SettingSuccessView(
    data: SettingData,
    onIntent: (SettingIntent) -> Unit,
    modifier: Modifier = Modifier
) {
    Column(modifier = modifier) {
        // Actual UI implementation
        Text(data.userName)
        Button(onClick = { onIntent(SettingIntent.Logout) }) {
            Text("Logout")
        }
    }
}
```

# 3. Screen Intent & State → `/screen/[screen-name]/`
- **RULES**:
  - ✅ `MealPreferencesIntent.kt` - ALLOWED in `/screen/preferences/`
  - ✅ `MealPreferencesState.kt` (contains MealPreferencesSuccessDataState) - ALLOWED in `/screen/preferences/`
  - ❌ `PreferenceTab.kt` - NOT allowed in `/screen/`, must be in `/model/`
  - ❌ `IntroData.kt` - NOT allowed in `/screen/`, must be in `/model/`

  Structure Example:
```
features/presentation/setting/
├── model/
│   ├── SettingSectionItem.kt        // UI Model
│   └── PreferenceTab.kt              // Enum → MUST be in /model/
└── screens/
    └── preferences/
        ├── MealPreferencesScreen.kt
        ├── MealPreferencesViewModel.kt
        ├── MealPreferencesIntent.kt      // Intent → ALLOWED here
        └── MealPreferencesState.kt       // ScreenState → ALLOWED here
```

# 4. Distinguishing based on naming:
```
MUST be in /model/:
- SettingSectionItem, IntroData, AgeMetricGroupUiItem
- PreferenceTab, ViewMode (ALL enums that are not xxxIntent/xxxScreenState)

ALLOWED in /screen/xxx/:
- XxxIntent (e.g., MealPreferencesIntent, MetricSyncIntent)
- XxxScreenState (e.g., CountryScreenState, MetricSyncStateScreen)
- XxxSuccessDataState (e.g., MealPreferencesSuccessDataState)
```

**❌ WRONG:**
```kotlin
// File: SettingSection.kt (in /components/)
data class SettingSectionItem(...)  // INCORRECT - data class in component file

@Composable
fun SettingSection(items: List<SettingSectionItem>) { }
```

**✅ CORRECT:**
```kotlin
// File: SettingSectionItem.kt (in /model/)
data class SettingSectionItem(...)

// File: SettingSection.kt (in /components/)
@Composable
fun SettingSection(items: List<SettingSectionItem>) { }
```

# 5. Decision Tree for UI Components
❌ WRONG:
```
MetricScreenHeader used in 4 screens (Age, Gender, Health, Hobbies)
→ Put into appdesign ✗ INCORRECT
→ Because all 4 screens belong to the preferences feature
```

✅ CORRECT:
```
MetricScreenHeader used in 4 screens within the preferences feature
→ Place in features/presentation/preferences/components/ ✓ CORRECT
```

❌ WRONG:
```
AppSelectableCard used in GenderCard (preferences) and LoginForm (auth)
→ Leave in preferences/components ✗ INCORRECT
→ Because it is used in 2 different features
```

✅ CORRECT:
```
AppSelectableCard used in preferences + auth features
→ Place in core-foundation:appdesign ✓ CORRECT
```

