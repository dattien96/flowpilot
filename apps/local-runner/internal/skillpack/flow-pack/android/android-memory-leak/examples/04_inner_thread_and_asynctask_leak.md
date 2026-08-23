# 04 — Inner Thread / AsyncTask / Handler Leak

## Context

Non-static inner classes hold an implicit synthetic field `this$0` to the outer `Activity`. If the thread/handler outlives the screen (rotation, back press, slow network), that `this$0` keeps the entire `Activity` + view tree alive.

## Before (Leaking)

```kotlin
class LoginActivity : AppCompatActivity() {
    // ❌ LEAK PATH: non-static inner class → synthetic this$0 ──strong──▶ LoginActivity
    inner class LoadTask : AsyncTask<Void, Void, String>() {
        override fun doInBackground(vararg p: Void): String {
            Thread.sleep(10_000) // ❌ LEAK PATH: Live Thread (GC Root) ──strong──▶ LoadTask ──strong──▶ Activity
            return "done"
        }
        override fun onPostExecute(r: String) { findViewById<TextView>(R.id.tv).text = r }
    }

    // ❌ LEAK PATH: anonymous Runnable captures outer this
    private val handler = Handler(Looper.getMainLooper())
    fun start() {
        handler.postDelayed(object : Runnable {
            override fun run() { updateUI() } // implicit this@LoginActivity
        }, 30_000) // GC Root: MessageQueue → Message → Runnable → Activity
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        LoadTask().execute() // launched, Activity rotates → old instance leaked
    }
}
```

**Leak Trace:**
```
GC Root (Live Thread "AsyncTask #1" / Main Looper MessageQueue)
  ──strong──▶ LoadTask / Runnable
    ──strong──▶ LoginActivity.this$0 (leaked, Retained = decorView + bindings)
```

## After (Fixed)

```kotlin
// ✅ SAFE / CLEAN FIX: static + WeakReference, or structured concurrency

// Option A — static class + WeakReference (classic fix)
class LoadTask(activity: LoginActivity) : AsyncTask<Void, Void, String>() {
    private val ref = WeakReference(activity) // ✅ SAFE: weak edge, GC can sweep Activity
    override fun doInBackground(vararg p: Void): String { Thread.sleep(5_000); return "done" }
    override fun onPostExecute(r: String) { ref.get()?.findViewById<TextView>(R.id.tv)?.text = r }
}

// Option B — lifecycleScope / viewModelScope (preferred, modern)
// ✅ SAFE / CLEAN FIX: coroutine scope is Small, cancelled with lifecycle — no Large → Small edge
class LoginViewModel : ViewModel() {
    fun load() = viewModelScope.launch {
        delay(10_000)
        // update StateFlow; Activity observes via repeatOnLifecycle
    }
}
class LoginActivity : AppCompatActivity() {
    private val vm: LoginViewModel by viewModels()
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                vm.uiState.collect { render(it) } // ✅ SAFE: auto-cancelled on destroy
            }
        }
    }
    // No Handler.postDelayed without removal; if needed:
    override fun onDestroy() {
        super.onDestroy()
        handler.removeCallbacksAndMessages(null) // ✅ SAFE: break MessageQueue → Runnable edge
    }
}
```

## Essence

`Thread` / `Message` in `Looper` queue are GC Roots (live threads). A non-static inner `Runnable`/`AsyncTask` is `Large` holding `Small` (`Activity`) via `this$0`. Even if `Activity` is `finish()`ed, the root keeps the task → keeps the activity. Fix is **scope-bound concurrency**: `static` + `WeakReference` breaks the strong edge, and `lifecycleScope`/`viewModelScope` makes the task's lifetime `≤ Activity`'s lifetime so `Large → Small` never forms.

**Heuristic signal:** `inner class` / `object : Runnable/Handler/AsyncTask` inside `Activity`/`Fragment` without `WeakReference` or without `removeCallbacks` / scope cancellation.
