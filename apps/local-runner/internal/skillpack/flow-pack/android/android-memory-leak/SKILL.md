---
version: 6
name: android-memory-leak
description: Use this skill PROACTIVELY when diagnosing, fixing, or preventing Android memory leaks. Covers ART GC reachability, heap dump analysis, LeakCanary/Profiler heuristics, and lifecycle-safe architecture.
---

# Android Memory Leak Detection & Remediation

Senior/Lead reference for **tracing, diagnosing, and fixing memory leaks** in Android (ART). This file holds theory, GC mechanics, profiler guide, and architectural fixes only. **Do not add per-case code here** — see `examples/` for concrete before/after implementations.

## A. Core Nature of Memory Leaks (Tracing GC / ART)

### 1. Strong Reachability Path (Mathematical Definition)

An object leaks **if** there exists a **Strong Reachability Path** `GC Root → … → leaked instance` **after** its logical lifecycle death (`Activity.onDestroy()`, `Fragment.onDestroyView()`, scope cancellation). ART uses **Tracing Mark-and-Sweep**: starting from roots, every strongly reachable object is marked alive; the rest is swept.

```
GC Root (static / live thread / JNI Global) ──strong──▶ … ──strong──▶ Leaked Activity
     └─ if no such path exists, even a cyclic island is collected
```

Weak/Soft/Phantom references are ignored for reachability — only `Strong` edges matter.

### 2. Scope Rule (Large vs Small)

| Scope size | Examples | Lifetime |
|---|---|---|
| **Large** (long-lived) | `Application`, `Singleton`, `Companion object`, `Static field`, `Live Thread`, `Process` | app / process |
| **Small** (short-lived) | `Activity`, `Fragment`, `View`, `ViewBinding`, `Controller/Presenter` | screen / view |

**Leak condition:**
```
Large ──strong──▶ Small   AND   Large is not cleared when Small dies  =>  LEAK
Small ──strong──▶ Large   =>  OK (small dies with large, no external anchor)
```

Concrete anchor from real hprof: `Large: WindowManagerGlobal.mRoots → DecorView → TextView → Spannable → ClickableSpan` holding `Small: LoginActivity` via synthetic field `$this_getClickableTextSpan` + captured lambda. After `Activity.finish()`, Large is still anchored to GC Root, so Small cannot be swept.

### 3. GC Root & Island of Isolation

If a cluster `A ↔ B` (e.g., `Activity ↔ Fragment ↔ View ↔ Span ↔ Activity`) cross-references itself but has **no edge from any GC Root**, ART collects the whole island. **This is NOT a leak** — explains why many Fragments keep `lazy contentViewBinding` without `binding = null` and still do not leak when they contain no external capture.

Leak requires a **true GC Root anchor**:

* **Static Fields** — `ClassLoader` / `Companion object` / `object Singleton` / `WindowManagerGlobal.mRoots`, `InputMethodManager.mNextServedView`, `TextLine.sCached`.
* **Live Threads** — `Choreographer`, `Main Looper MessageQueue` (posted `Runnable`/`Handler`), running `Thread`/`AsyncTask`, uncancelled `Coroutine` (`lifecycleScope` without cancellation).
* **JNI Global References** — `env->NewGlobalRef()` bridge Native ↔ ART; survives until `DeleteGlobalRef()`.

### 4. Two Independent Paths Mental Model

Every view on screen has:

* **Path 1 (structural):** `GC Root → WindowManager → DecorView → View` — exists for every visible screen, created by `setContentView`/`addView`. Cut automatically By WindowManager at System level on `removeView` after `onDestroy()`.
* **Path 2 (captured):** `View → Span / Listener / Callback → Activity` — created only when code captures `Activity` (see Heuristics).

Leak = Path 1 still alive **and** Path 2 holds Activity. Passing an Activity Context only extends Path 2; it does not create Path 1.

---

## B. Diagnosis Guide — Android Profiler / LeakCanary

### 1. Heap Dump Setup

* Trigger `Force GC` before dump. Filter `Filter by: Activity/Fragment leaks` (Android Studio Heap) — only instances that have passed `onDestroy()` but remain strongly reachable are flagged. Count `Leaks` column, inspect `Shallow` vs `Retained Size`.
* Top bar: `Heap: App`, `Class: All classes`, `Arrange by: Class`. Depth = distance to GC Root (0 = root); smaller Depth = closer to root.

### 2. Reading a Leak Trace

* Start at the `Leaked instance` header (`LoginActivity@326906016 Depth 24, Retained 490KB`) — large retained confirms the whole view tree is retained.
* Follow `(F)` Field / `(C)` Class symbols. Kotlin synthetic fields `this$0` (inner class) and `$this_getClickableTextSpan` (extension receiver) are implicit strong edges holding the outer instance.
* Ignore `WeakReference` / `WeakHashMap$Entry` branches — they do not prevent collection.

### 3. "Show nearest GC root only" — Why Mandatory

A leaked instance typically has 30+ incoming references (layouts, arrays, `mContext` in every child). Without filtering, the dominator edge is hidden in noise.

Enabling **Show nearest GC root only** runs the **Shortest Path** algorithm from any GC Root to the leaked object and collapses all non-minimal branches. In the real case:

* OFF: ~30 refs including `mAppCompatCallback`, `PhoneLayoutInflater`, `WebView`, `FitWindowsLinearLayout`.
* ON: 5 refs — `LoginActivity:24 ← $this_getClickableTextSpan:23 (×2 Terms/Privacy) ← Spannable ← TextView ← GC Root`. The two `Depth 23` entries are the exact `ClickableSpan` instances created in `LoginFragment`.

Workflow: enable toggle → expand the `Depth 23` node → verify `SpannableString.mSpans → TextView.mText` chain → confirm GC Root at the top (`WindowManager`/`InputMethodManager` or static cache).

---

## C. Detection Heuristics for AI (Automated Scan Rules)

Use these as a checklist / lint rules when reviewing Kotlin code.

### Heuristic 1 — Context Escaping

> Flag any `Activity` / `Fragment.requireContext()` flowing into `Singleton`, `Companion object`, `object`, `static field`, `Repository`, `Helper`, `Manager`.

Signals: `fun foo(context: Context)` stored in `companion object`, `@Singleton` constructor parameter typed `Context` without `@ApplicationContext`, custom `Cache.put(context)`.

Fix principle: inject `@ApplicationContext` or `context.applicationContext`.

### Heuristic 2 — Implicit Capture (Anonymous / Lambda)

> Flag `object : ClickableSpan()`, `object : Handler()`, `Runnable`, `Callback`, `WebViewClient`, `MovementMethod`, `addTextChangedListener { this }` where the anonymous class or lambda references `this` / `this@Activity` / outer `Context`.

Signals: extension `fun Context.getClickableTextSpan(action: ()->Unit)` returning anonymous `ClickableSpan` whose `updateDrawState` reads `this@getClickableTextSpan.resources` and whose `action` lambda captures `Activity`; non-static `AsyncTask`/`Thread` with implicit `this$0`.

Fix principle: `companion object` / top-level function, `WeakReference`, or `(View) -> Unit` handler that resolves context from the `View` param at click time.

### Heuristic 3 — Dangling Listeners / Observers

> Flag `register()` / `observe()` / `addListener()` / `set MovementMethod` / `text = spannable` without symmetric `unregister()` / `removeObserver()` / `nullify` in `onDestroy()` / `onDestroyView()`.

Signals: `LinkMovementMethod.getInstance()` + `textView.text = spannableWithSpans` without `onDestroyView { text = null; movementMethod = null }`, `LiveData.observe`, `Flow.collect`, `BroadcastReceiver.register`, `LocationManager.requestLocationUpdates`.

Fix principle: enforce `onDestroyView` cleanup; prefer `lifecycleScope` / `repeatOnLifecycle` / `viewLifecycleOwner`.

### Heuristic 4 — Native Bridge Boundaries

> Flag `env->NewGlobalRef(obj)` in C/C++ without a symmetric `env->DeleteGlobalRef()` triggered by Kotlin `onDestroy` / `close()`.

Signals: JNI callback object stored as `jobject gCallback;` set via `NewGlobalRef`, background native thread calling back into Kotlin after Activity death.

Fix principle: symmetric `nativeDestroy()` or use `NewWeakGlobalRef` so ART can collect the Kotlin peer.

---

## D. Architectural Remedies (Generic)

* **Context discipline:** never store `Activity` in objects whose lifetime exceeds the screen; DI graph provides `@ApplicationContext` for singletons.
* **Span / Callback hygiene:** make `ClickableSpan` stateless; resolve `Context` from `View.getContext()` at `onClick(View)` time, not at construction time.
* **Lifecycle symmetry:** every `register / add / set` has a paired `unregister / remove / null` in the matching destroy callback.
* **Thread / Coroutine scoping:** prefer `lifecycleScope`/`viewModelScope`; bare `Thread`/`Handler` must be `static + WeakReference` and explicitly quit.
* **Tooling:** run LeakCanary in `debug` builds; gate CI with heap-dump retained-size threshold and fail on `Activity/Fragment leaks > 0`.

> Detailed before/after code for each pattern lives in `examples/`. Keep this file free of per-case implementations to preserve token budget.

## References

* `SKILL.md` template derived from `code-style` / `architecture` skills in this pack.
* Real heap case: `LoginActivity@326906016 Retained 490KB`, 2× `AppUtilsKt$getClickableTextSpan` at `Depth 23` (this document's motivating example).
