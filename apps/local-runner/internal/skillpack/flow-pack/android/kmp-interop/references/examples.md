# KMP Interop Examples

## 1. Registry Pattern Implementation

### 1.1 Kotlin Side - Bridge Registry Pattern
**❌ WRONG - Top-level lateinit var (Anti-pattern):**
```kotlin
// File: AdMobBanner_ios.kt
lateinit var createIOSBannerAd: (String, () -> Unit, (String) -> Unit) -> UIViewController

// File: AdMobInterstitial_ios.kt
lateinit var createIOSInterstitialAdHelper: () -> IOSInterstitialAdBridge
```

**✅ CORRECT - Centralized Registry:**
```kotlin
// File: AdMobBridgeRegistry.kt (in commonMain or iosMain)
object AdMobBridgeRegistry {
    var bannerAdFactoryFromXcode: ((String, () -> Unit, (String) -> Unit) -> UIViewController)? = null
    var interstitialAdHelperFromXcode: (() -> IOSInterstitialAdBridge)? = null

    // Usage in actual implementation
    fun createBannerAd(adUnitID: String, onLoaded: () -> Unit, onFailed: (String) -> Unit): UIViewController {
        return bannerAdFactoryFromXcode?.invoke(adUnitID, onLoaded, onFailed)
            ?: error("Banner ad factory not initialized. Call AdMobBridgeRegistry.setup() in AppDelegate.")
    }
}
```

**Benefits:**
- ✅ Single point of registration.
- ✅ Null-safe access with clear error messages.
- ✅ Scalable: Adding a new bridge only requires adding a property to the Registry.
- ✅ Testable: Easily mock the Registry.

### 1.2 Swift Side - AppDelegate Initialization
**❌ WRONG - Init in SwiftUI View:**
```swift
// ContentView.swift or ComposeView
struct ComposeView: UIViewControllerRepresentable {
    init() {
        // ❌ WRONG - SwiftUI can re-init this View multiple times!
        AdMobBanner_iosKt.createIOSBannerAd = { ... }
        AdMobInterstitial_iosKt.createIOSInterstitialAdHelper = { ... }
    }
}
```

**✅ CORRECT - Init in AppDelegate (One-time setup):**
```swift
// AppDelegate.swift
@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        // ✅ CORRECT - Called only once when the app launches
        setupAdMobBridge()
        return true
    }

    private func setupAdMobBridge() {
        AdMobBridgeRegistry.shared.bannerAdFactoryFromXcode = { adUnitID, onLoaded, onFailed in
            let bannerView = BannerAdView(
                adUnitID: adUnitID,
                onAdLoaded: { _ = onLoaded() },
                onAdFailed: { error in _ = onFailed(error) }
            )
            return UIHostingController(rootView: bannerView)
        }

        AdMobBridgeRegistry.shared.interstitialAdHelper = {
            return InterstitialAdBridge()
        }
    }
}
```

**Benefits:**
- ✅ Guaranteed one-time execution.
- ✅ Proper app lifecycle management.
- ✅ No risk of re-initialization.
- ✅ Clear separation of concerns.

### 1.3 Decision Tree: Where to Initialize Bridge?
```
Setting up an iOS bridge?
│
├─ Does the bridge need initialization at app launch?
│  └─ YES → AppDelegate.application(didFinishLaunching)
│     Example: AdMob, Firebase, Analytics
│
├─ Does the bridge depend on user state/navigation?
│  └─ YES → Lazy init in ViewModel or when needed
│     Example: Payment processor (init only at checkout)
│
└─ Is the bridge used only in a specific View?
   └─ YES → Init in that View's ViewModel (NOT View.init())
      Example: Camera bridge (only used on the camera screen)
```

### 1.4 File Structure Example
```
core/
└── ads/
    ├── commonMain/
    │   └── AdMobBridgeRegistry.kt         # Registry object
    └── iosMain/
        ├── AdMobBanner_ios.kt             # Banner implementation
        └── AdMobInterstitial_ios.kt       # Interstitial implementation

iosApp/
├── AppDelegate.swift                      # Bridge setup here ✅
└── ContentView.swift                      # No bridge setup ❌
```

### 1.5 Full Example: Firebase Bridge
```swift
// Step 1: Create FirebaseBridgeRegistry.kt in :core:appfirebase/iosMain
object FirebaseBridgeRegistry {
    var authFactory: (() -> IOSFirebaseAuthBridge)? = null
    var analyticsFactory: (() -> IOSFirebaseAnalyticsBridge)? = null

    fun createAuthBridge(): IOSFirebaseAuthBridge {
        return authFactory?.invoke()
            ?: error("FirebaseBridgeRegistry.authFactory not initialized. Call setupFirebaseBridge() in AppDelegate.")
    }
}

// Step 2: Update implementation files
class IOSFirebaseAuth : FirebaseAuthBridge {
    private val bridge = FirebaseBridgeRegistry.createAuthBridge()
    // ...
}

// Step 3: Add to AppDelegate.swift
private func setupFirebaseBridge() {
    FirebaseBridgeRegistry.shared.authFactory = {
        return SwiftFirebaseAuthImpl()
    }
}

func application(didFinishLaunching) -> Bool {
    setupAdMobBridge()     // Existing
    setupFirebaseBridge()  // New
    return true
}
```

---

## 2. Type Mapping Examples

### 2.1 Pattern: Swift Conforms to Kotlin Interface
**❌ WRONG - Using KotlinUnit in Swift class:**
```swift
// Kotlin interface
interface IOSBridge {
    fun load(onSuccess: () -> Unit)
}

// Swift class - WRONG
class Bridge: IOSBridge {
    func load(onSuccess: @escaping () -> KotlinUnit) {  // ❌ WRONG
        onSuccess()
    }
}
```

**✅ CORRECT - Using Void:**
```swift
// Swift class - CORRECT
class Bridge: IOSBridge {
    func load(onSuccess: @escaping () -> Void) {  // ✅ CORRECT
        onSuccess()
    }
}
```

### 2.2 Pattern: Calling Kotlin Function from Swift
**When calling Kotlin functions that return `Unit`, you must explicitly discard `KotlinUnit`:**
```swift
// Kotlin
lateinit var kotlinCallback: () -> Unit

// Swift calls it
AdMobKt.kotlinCallback = { onLoaded, onFailed in
    BannerView(
        onAdLoaded: {
            _ = onLoaded()  // ✅ Discard KotlinUnit
        },
        onAdFailed: { error in
            _ = onFailed(error)  // ✅ Discard KotlinUnit
        }
    )
}
```

### 2.3 Gradle framework export
**Check export in `shared/build.gradle.kts`:**
```kotlin
cocoapods {
    framework {
        baseName = "shared"
        isStatic = true
        export(projects.core.ads)  // ✅ Export ads module
    }
}
```

### 2.4 Decision Tree: Type Conversion
```
Writing Swift code?
│
├─ Swift class conforms to a Kotlin interface?
│  └─ YES → Use () -> Void, (String) -> Void
│     Example: class Bridge: IOSBridge { func load(onSuccess: () -> Void) }
│
├─ Swift calling a Kotlin function returning Unit?
│  └─ YES → Discard KotlinUnit using _ =
│     Example: _ = onLoaded()
│
└─ Swift function being called from Kotlin?
   └─ YES → Swift receives () -> Void, no conversion needed
      Example: helper.load(onLoaded: kotlinCallback)
```

### 2.5 Common Errors and Fixes
**Error 1: Type does not conform to protocol**
```
Type 'InterstitialAdBridge' does not conform to protocol 'IOSInterstitialAdBridge'
```
**Cause**: Function signature mismatch.
**Fix**: Change `() -> KotlinUnit` to `() -> Void`.

**Error 2: Cannot convert value of type**
```
Cannot convert value of type '() -> KotlinUnit' to expected argument type '() -> Void'
```
**Cause**: Directly passing a Kotlin callback to a Swift function.
**Fix**: Wrap in a closure:
```swift
onLoaded: {
    _ = kotlinOnLoaded()
}
```

---

## 3. UI Bridge Pattern Examples (Google Sign-In)

**CRITICAL**: When creating a feature with native iOS functionality (Google Sign-In, Apple Pay, Camera, etc.), you MUST follow this pattern to:
- ✅ Reuse design system (colors, typography, shapes) from Compose.
- ✅ Reuse i18n strings from Compose resources.
- ✅ Avoid duplicate code between platforms.
- ✅ Keep Swift code minimal - only handle native logic.

### 3.1 Pattern Overview
```
┌─────────────────────────────────────────────────────────────┐
│                    COMPOSE (KMP)                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  PrimaryButton (design system)                       │   │
│  │  Text(uiLayerString(Res.string.xxx)) (i18n)         │   │
│  │  onClick → generate data → call bridge               │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                 │
│                           ▼                                 │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Bridge Interface (Kotlin)                           │   │
│  │  fun doAction(data: DataFromKotlin, callback)        │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    SWIFT (Native)                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Bridge Implementation                               │   │
│  │  - Receive data from Kotlin                          │   │
│  │  - Execute native SDK (Google, Apple, etc.)          │   │
│  │  - Return result via callback                        │   │
│  │  - NO UI rendering - Compose handles that            │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

#### 3.2. When to Use This Pattern

| Scenario | Use This Pattern? |
|----------|-------------------|
| Native SDK requires presenting UI (OAuth, Payment) | ✅ YES |
| Need platform-specific crypto/security | ✅ YES |
| Camera, Biometrics, HealthKit access | ✅ YES |
| Pure UI component (buttons, cards) | ❌ NO - Use Compose |
| Data processing, networking | ❌ NO - Use KMP shared code |

### 3.3 Implementation Steps
**Kotlin Interface (iosMain)**
```kotlin
// core/xxx/src/iosMain/kotlin/.../IOSXxxBridge.kt
interface IOSXxxBridge {
    /**
     * @param dataFromKotlin Data generated in Kotlin (e.g., nonce, config)
     * @param onResult Callback with result from native SDK
     */
    fun doAction(dataFromKotlin: String, onResult: (Result?) -> Unit)
}
```

**Compose UI (iosMain)**
```kotlin
// core/xxx/src/iosMain/kotlin/.../XxxView.ios.kt
@Composable
actual fun XxxView(onComplete: (Result) -> Unit) {
    val bridge: XxxIosNativeBridge = koinInject()
    val xxxBridge = remember { bridge.createXxxBridge() }

    // Use Compose UI - reuses design system and i18n
    Column(modifier = Modifier.wrapContentSize()) {
        PrimaryButton(
            onClick = {
                // Generate data in Kotlin (shared logic)
                val data = DataGenerator.generate()

                // Pass to Swift, keep processing data in Kotlin
                xxxBridge.doAction(data.hashed) { result ->
                    onComplete(Result(result, data.raw))
                }
            },
            buttonContent = {
                Text(uiLayerString(Res.string.xxx_button)) // i18n
            }
        )
    }
}
```

**Swift Bridge Implementation**
```swift
// iosApp/iosApp/Xxx/XxxHelper.swift
final class XxxHelper {
    static let shared = XxxHelper()

    func doAction(dataFromKotlin: String, onResult: @escaping (String?) -> Void) {
        // Use native SDK
        NativeSDK.shared.execute(data: dataFromKotlin) { result in
            _ = onResult(result)  // Discard KotlinUnit
        }
    }
}

// iosApp/iosApp/XcodeBridges.swift
class SwiftXxxBridge: NSObject, IOSXxxBridge {
    func doAction(dataFromKotlin: String, onResult: @escaping (String?) -> Void) {
        XxxHelper.shared.doAction(dataFromKotlin: dataFromKotlin, onResult: onResult)
    }
}
```

### 3.3 Google Sign-In Flow Example
**Problems Solved:**
- ❌ OLD: Swift generates nonce, renders button → duplicate code, mismatched styling.
- ✅ NEW: Kotlin generates nonce, renders button → shared code, consistent UI.

**Files:**

| Layer | File | Purpose |
|-------|------|---------|
| Kotlin Interface | `IOSGoogleSignInBridge.kt` | Define `signIn(hashedNonce, callback)` |
| Kotlin UI | `LoginGoogleView.ios.kt` | Compose UI + nonce generation |
| Shared Logic | `NonceGenerator.kt` | SHA-256 nonce (used by both Android/iOS) |
| Swift Bridge | `XcodeBridges.swift` | Conform to Kotlin interface |
| Swift Helper | `GoogleSignInHelper.swift` | Call GIDSignIn SDK |

**Data Flow:**
```
1. User taps PrimaryButton (Compose)
   ↓
2. Kotlin: val nonce = NonceGenerator.generate()
   ↓
3. Kotlin: bridge.signIn(nonce.hashed) { idToken -> ... }
   ↓
4. Swift: GIDSignIn.signIn(nonce: hashedNonce)
   ↓
5. Swift: callback(idToken)
   ↓
6. Kotlin: onLoginGoogle(idToken, nonce.raw)  // raw nonce kept in Kotlin
   ↓
7. Kotlin: Supabase.signInWithIdToken(idToken, nonce.raw)
```

### 3.4. Key Principles

**A. Data Generation in Kotlin**
```kotlin
// ✅ CORRECT - Generate in Kotlin, pass to Swift
val nonce = NonceGenerator.generate()  // Shared code
bridge.signIn(nonce.hashed) { idToken ->
    // nonce.raw stays in Kotlin for further processing
}

// ❌ WRONG - Generate in Swift
// Causes duplicate code and potential mismatch
```

**B. UI in Compose**
```kotlin
// ✅ CORRECT - Use design system
PrimaryButton(onClick = { bridge.signIn() }) {
    Text(uiLayerString(Res.string.action_button))
}

// ❌ WRONG - Embed Swift UI
UIKitView(factory = { swiftButton.view })
```

**C. Swift Only for Native SDK Calls**
```swift
// ✅ CORRECT - Minimal Swift code
func signIn(hashedNonce: String, onResult: @escaping (String?) -> Void) {
    GIDSignIn.signIn(nonce: hashedNonce) { result in
        _ = onResult(result?.idToken)
    }
}

// ❌ WRONG - Generate data in Swift
func signIn(onResult: @escaping (String?, String) -> Void) {
    let nonce = generateNonce()  // Duplicate logic!
    // ...
}
```

**Implementation Comparison:**
```kotlin
// ✅ CORRECT - Generate in Kotlin, pass to Swift
val nonce = NonceGenerator.generate()  // Shared code
bridge.signIn(nonce.hashed) { idToken ->
    // nonce.raw stays in Kotlin for further processing
}

// ❌ WRONG - Generate in Swift
// Causes duplicate code and potential mismatch
```
