# KMP Interop Bridge Patterns

## 1. Kotlin Side - Bridge Registry Pattern

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

## 2. Swift Side - AppDelegate Initialization

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

## 3. Firebase Bridge Implementation Template

**Example: Creating Firebase Bridge**

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
```

```swift
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
