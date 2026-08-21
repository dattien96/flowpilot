# 06 — JNI Native GlobalRef Leak

## Context

Kotlin ↔ C/C++ bridge via JNI. A native background thread needs to callback into Kotlin (`onNativeEvent()`), so the native side stores the Kotlin `callbackObj` as a `GlobalRef`. `JNI Global References` are **GC Roots** — the Kotlin object cannot be swept until `DeleteGlobalRef()` is called, even after the `Activity` is destroyed.

## Before (Leaking)

```cpp
// native.cpp
static jobject gCallback = nullptr; // JNI GlobalRef — GC Root, lives beyond Activity

extern "C" JNIEXPORT void JNICALL
Java_com_example_Player_nativeInit(JNIEnv* env, jobject /*thiz*/, jobject callback) {
    gCallback = env->NewGlobalRef(callback); // ❌ LEAK PATH: Native Global (GC Root) ──strong──▶ Activity/Fragment/Listener
    std::thread([]{
        while (true) { // background native thread — also a GC Root (live thread)
            // callback into Kotlin via gCallback
        }
    }).detach();
}

// ❌ LEAK PATH: no matching DeleteGlobalRef — Kotlin onDestroy never triggers cleanup
```

```kotlin
// PlayerActivity.kt
class PlayerActivity : AppCompatActivity() {
    init { nativeInit(this) } // ❌ LEAK PATH: Activity (this) passed as callback → NewGlobalRef holds it

    external fun nativeInit(callback: Any)
    fun onNativeEvent() { updateUI() } // native thread calls back here
    // no nativeDestroy() call in onDestroy()
}
```

**Leak Trace:**
```
GC Root (JNI Global gCallback)
  ──strong──▶ PlayerActivity (leaked, Retained = full view hierarchy)
  └─ also anchored by Native Live Thread
```

Profiler shows `PlayerActivity` retained large; `JNI Global` count grows per launch in `adb shell dumpsys meminfo`.

## After (Fixed)

```cpp
// native.cpp
static jobject gCallback = nullptr;
static JavaVM* gJvm = nullptr;

extern "C" JNIEXPORT void JNICALL
Java_com_example_Player_nativeInit(JNIEnv* env, jobject, jobject callback) {
    gCallback = env->NewGlobalRef(callback); // create
}

extern "C" JNIEXPORT void JNICALL
Java_com_example_Player_nativeDestroy(JNIEnv* env, jobject) {
    if (gCallback) {
        env->DeleteGlobalRef(gCallback); // ✅ SAFE / CLEAN FIX: symmetric cleanup removes GC Root edge
        gCallback = nullptr;
    }
}

// Alternative: weak reference — does not prevent GC
// gCallbackWeak = env->NewWeakGlobalRef(callback);
// env->IsSameObject not needed; GC can sweep Kotlin peer even if native still holds weak ref
// ✅ SAFE: WeakGlobalRef does not anchor the Kotlin object
```

```kotlin
// PlayerActivity.kt
class PlayerActivity : AppCompatActivity() {
    init { nativeInit(this) }

    external fun nativeInit(callback: Any)
    external fun nativeDestroy() // ✅ SAFE / CLEAN FIX: symmetric native cleanup

    override fun onDestroy() {
        nativeDestroy() // ✅ SAFE: always pair NewGlobalRef with DeleteGlobalRef in matching destroy
        super.onDestroy()
    }
}
```

## Essence

`NewGlobalRef` elevates a Kotlin object to a **GC Root** on the native side. Without symmetric `DeleteGlobalRef`, the object (`Activity`/`Listener`) is `Large` (native process lifetime) holding `Small` (`Activity` screen lifetime) — textbook `Large → Small` leak across the Native ↔ ART boundary. Fix is **symmetric lifecycle**: every `NewGlobalRef` must have a paired `DeleteGlobalRef` in the matching Kotlin destroy/close, or use `NewWeakGlobalRef` when the native side does not need to keep the Kotlin object alive (check `IsSameObject` / `IsWeakGlobalRef` liveness before use).

**Heuristic signal:** any `NewGlobalRef` in `cpp/` without a `DeleteGlobalRef` in the same file, or any `external fun nativeInit(callback)` that passes `this` without a corresponding `nativeDestroy()` called in `onDestroy`/`onCleared`/`close()`.

**Note on Island of Isolation:** native `GlobalRef` is a true GC Root, so even an isolated `Activity ↔ Fragment ↔ View` island cannot be swept if `GlobalRef` holds any one of them — unlike a pure Kotlin cycle, the native anchor prevents collection.
