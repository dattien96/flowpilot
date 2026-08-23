# 05 — Fragment ViewBinding Leak

## Context

`Fragment` views are destroyed in `onDestroyView()` but the `Fragment` instance itself survives in the backstack (`FragmentManager`). Holding the binding (which holds the entire view tree + `Activity` context via `mContext`) past `onDestroyView()` promotes the destroyed view tree to `Fragment` lifetime.

## Before (Leaking)

```kotlin
class DetailFragment : Fragment(R.layout.fragment_detail) {
    // ❌ LEAK PATH: Fragment (Large, in backstack) ──strong──▶ binding ──strong──▶ View tree ──strong──▶ Activity context
    private var _binding: FragmentDetailBinding? = null
    private val binding get() = _binding!!

    // Also common with lazy ViewBinding in ContentStateViewFragment:
    // private val contentViewBinding: ViewBinding by lazy { getCustomContentView() } — never cleared

    override fun onCreateView(inflater: LayoutInflater, c: ViewGroup?, s: Bundle?): View {
        _binding = FragmentDetailBinding.inflate(inflater, c, false)
        binding.recycler.adapter = adapter // adapter may also capture Fragment/Activity
        return binding.root
    }

    // ❌ LEAK PATH: missing cleanup — Fragment retains destroyed view hierarchy
    // no onDestroyView() override
}
```

**Leak Trace:**
```
GC Root (FragmentManager.mActive → DetailFragment)
  ──strong──▶ DetailFragment._binding
    ──strong──▶ FragmentDetailBinding.root
      ──strong──▶ ViewGroup → TextView.mContext ──strong──▶ Activity (leaked after onDestroyView)
```

LeakCanary flags `DetailFragment` and `Activity` as leaked; Profiler shows `Fragment` retained size = entire `DecorView` subtree.

## After (Fixed)

```kotlin
class DetailFragment : Fragment(R.layout.fragment_detail) {
    private var _binding: FragmentDetailBinding? = null
    private val binding get() = _binding!!

    override fun onCreateView(inflater: LayoutInflater, c: ViewGroup?, s: Bundle?): View {
        _binding = FragmentDetailBinding.inflate(inflater, c, false)
        return binding.root
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        binding.recycler.adapter = adapter
    }

    // ✅ SAFE / CLEAN FIX: symmetric nullify in matching destroy callback
    override fun onDestroyView() {
        super.onDestroyView()
        // break Fragment → View edge; also clear span/movementMethod if present
        binding.recycler.adapter = null
        _binding = null // ✅ SAFE: Fragment no longer holds destroyed view tree
    }
}

// Alternative delegate for concise syntax
// ✅ SAFE: auto-cleared delegate enforces the same rule
class AutoClearedValue<T : Any>(val fragment: Fragment) : ReadWriteProperty<Fragment, T> {
    private var _value: T? = null
    init { fragment.lifecycle.addObserver(object : DefaultLifecycleObserver {
        override fun onDestroy(owner: LifecycleOwner) {
            if (owner === fragment.viewLifecycleOwner) _value = null
        }
    }) }
    override fun getValue(t: Fragment, p: KProperty<*>) = _value!!
    override fun setValue(t: Fragment, p: KProperty<*>, v: T) { _value = v }
}
```

## Essence

`Fragment` lifetime (`Large`: survives `onDestroyView` in backstack) vs `View` lifetime (`Small`: dies at `onDestroyView`). Holding `_binding` past `onDestroyView` is `Large → Small` with no symmetric clear — the classic definition of a leak. The fix is **lifecycle symmetry**: `onCreateView` allocates, `onDestroyView` nullifies. This does not change the `Span` capture rule; it cuts the `Fragment → View` anchor so that even if a view internally captures context, the fragment no longer prolongs the view tree.

**Heuristic signal:** `Fragment` with `_binding` / `lateinit binding` / `by lazy ViewBinding` and no `onDestroyView { _binding = null }`.

**Note on lazy contentViewBinding:** `ContentStateViewFragment.contentViewBinding by lazy { getCustomContentView() }` exhibits the same pattern. It is safe only when `getCustomContentView()` returns views that do not capture `Activity` (no `ClickableSpan` with `Activity` capture). Once a span/worker captures `Activity`, the lazy holder must be cleared or the span must be stateless (see `01_static_movement_method_leak`).
