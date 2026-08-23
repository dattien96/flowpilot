# 03 — Nested FragmentManager Leak (parent vs child)

## Context

A parent `Fragment` hosts a `ViewPager`/`ViewPager2`/`BottomSheet` with child fragments. Using the Activity's `supportFragmentManager` instead of the parent's `childFragmentManager` decouples child fragment lifecycle from the parent — children outlive the parent and keep its `Context` alive.

## Before (Leaking)

```kotlin
class ParentFragment : Fragment() {
    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val adapter = ChildPagerAdapter(
            requireActivity().supportFragmentManager, // ❌ LEAK PATH: Activity-scoped FM ──strong──▶ ChildFragment
            lifecycle
        )
        // or
        ChildFragment().show(requireActivity().supportFragmentManager, "tag") // ❌ LEAK PATH: same
        viewPager.adapter = adapter
    }
}

class ChildPagerAdapter(fm: FragmentManager, lc: Lifecycle) : FragmentStateAdapter(fm, lc)
```

**Leak Trace:**
```
GC Root (Activity.mFragments → FragmentManager.mActive)
  ──strong──▶ ChildFragment (retained via Activity FM)
    ──strong──▶ ParentFragment.mView (via requireActivity() context / shared ViewModel)
      ──strong──▶ ParentFragment (already destroyView but still referenced by child)
```

When `ParentFragment` is popped, `Activity`'s `FragmentManager` still retains `ChildFragment` instances (they are `Large` scope now), which in turn hold `ParentFragment`'s `Context`/`View` via captured references.

## After (Fixed)

```kotlin
class ParentFragment : Fragment() {
    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        // ✅ SAFE / CLEAN FIX: child lifecycle bound to parent, not Activity
        val adapter = ChildPagerAdapter(childFragmentManager, viewLifecycleOwner.lifecycle)
        viewPager.adapter = adapter
    }

    private fun showChildSheet() {
        // ✅ SAFE: same
        ChildBottomSheet().show(childFragmentManager, "tag")
    }
}

class ChildPagerAdapter(fm: FragmentManager, lc: Lifecycle) : FragmentStateAdapter(fm, lc)
```

## Essence

`FragmentManager` is the owner scope. `requireActivity().supportFragmentManager` is `Large` (Activity lifetime); `childFragmentManager` is `Small` (parent Fragment lifetime). Hosting children in the wrong manager violates `Large → Small` rule: Activity-scoped manager (`Large`) holds child fragments (`Small`) that hold parent context, so parent cannot be swept when popped. Fix is **scope alignment** — children of a Fragment must be managed by `childFragmentManager` / `viewLifecycleOwner.lifecycle`.

**Heuristic signal:** any `requireActivity().supportFragmentManager` or `parentFragmentManager` inside `Fragment.onViewCreated` / `DialogFragment.show()` where the caller is itself a Fragment.
