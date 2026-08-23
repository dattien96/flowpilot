# 02 — Wrong Context Scope in Singleton

## Context

A singleton manager/repository/helper outlives any single `Activity` (scope: `Application` / process). Passing an `Activity` Context into it promotes the Activity to process lifetime.

## Before (Leaking)

```kotlin
// AnalyticsManager.kt — singleton holds whatever Context is passed
object AnalyticsManager {
    private lateinit var context: Context // ❌ LEAK PATH: static field (GC Root) ──strong──▶ Activity

    fun init(context: Context) {
        this.context = context // ❌ LEAK PATH: caller passes Activity
    }
    fun track(event: String) { /* uses context */ }
}

// LoginActivity.kt
override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    AnalyticsManager.init(this) // ❌ LEAK PATH: Activity → Singleton (Large) holds Activity (Small)
}
```

**Leak Trace:**
```
GC Root (static AnalyticsManager.context)
  ──strong──▶ LoginActivity (leaked, Retained = whole view tree)
```

LeakCanary/Profiler shows `AnalyticsManager` as GC Root class; `Activity` retained size = entire hierarchy.

## After (Fixed)

```kotlin
// Option A — use Application context explicitly
// ✅ SAFE / CLEAN FIX: store Application-scoped context only
object AnalyticsManager {
    private lateinit var appContext: Context
    fun init(context: Context) {
        this.appContext = context.applicationContext // ✅ SAFE: Application outlives everyone, no Activity edge
    }
}

// Option B — Hilt / DI (preferred)
// ✅ SAFE / CLEAN FIX: DI provides correct scope, impossible to pass Activity
@Singleton
class AnalyticsManager @Inject constructor(
    @ApplicationContext private val appContext: Context
) {
    fun track(event: String) { /* use appContext */ }
}

// Call site — no manual init needed
// @AndroidEntryPoint class LoginActivity : AppCompatActivity() { @Inject lateinit var analytics: AnalyticsManager }
```

## Essence

Singletons are GC Roots via their `ClassLoader`. Never store an `Activity`/`Fragment`/`View` Context in them. The fix is a **scope discipline**: singletons only hold `applicationContext` (which is itself a GC Root's child, so `Large → Large` is safe). DI with `@ApplicationContext` makes the wrong scope a compile-time error.

**Heuristic signal:** any `Singleton.init(context)` or `Companion object` field typed `Context` without `.applicationContext` unwrapping.
