# Kotlin-Swift Type Mapping Examples

## 1. Pattern: Swift Conforms to Kotlin Interface

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

## 2. Pattern: Calling Kotlin Function from Swift

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
