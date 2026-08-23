# 01 — Static MovementMethod + Capturing ClickableSpan Leak

## Context

`LoginFragment` displays Terms/Privacy with clickable spans. `LinkMovementMethod.getInstance()` is a **static singleton (GC Root)**, and a custom `Context.getClickableTextSpan()` creates an anonymous `ClickableSpan` that captures the `Activity`.

## Before (Leaking)

```kotlin
// AppUtils.kt — extension captures Activity
// ❌ LEAK PATH: ClickableSpan.$this_getClickableTextSpan ──strong──▶ Activity
// ❌ LEAK PATH: ClickableSpan.action lambda captures Activity via browser(this)
fun Context.getClickableTextSpan(action: () -> Unit) = object : ClickableSpan() {
    override fun onClick(v: View) { action() } // ❌ LEAK PATH: holds lambda → Activity
    override fun updateDrawState(ds: TextPaint) {
        ds.color = this@getClickableTextSpan.resources.getColor(R.color.colorPrimary, null) // ❌ LEAK PATH: receiver = Activity
    }
}

// LoginFragment.kt
context?.run { // this = Activity
    val spannable = SpannableString("$desc $terms and $privacy")
    spannable.setSpan(this.getClickableTextSpan { // ❌ LEAK PATH: receiver + lambda both Activity
        DataConstant.URL_TERMS.browser(this, DataConstant.URL_TERMS_HTML)
    }, desc.length + 1, desc.length + terms.length + 1, 0)
    textDescription.movementMethod = LinkMovementMethod.getInstance() // static GC Root touches view
    textDescription.text = spannable // TextView.mText ──strong──▶ Spannable ──strong──▶ Span ──strong──▶ Activity
}
```

**Leak Trace (heap, Show nearest GC root only):**
```
GC Root (WindowManagerGlobal / InputMethodManager static)
  → TextView textDescription
    → SpannableString mSpans
      → ClickableSpan $this_getClickableTextSpan (Depth 23) ──strong──▶ LoginActivity (Depth 24, Retained 490KB)
      → ClickableSpan.action ──strong──▶ LoginActivity
```
Two identical `Depth 23` nodes (Terms + Privacy).

**Why not collected:** `GC Root → TextView` is still alive (window/cache), and `TextView → Span → Activity` keeps `Activity` alive after `finish()`.

## After (Fixed)

```kotlin
// AppUtils.kt — stateless, resolves context at click time
// ✅ SAFE / CLEAN FIX: no synthetic $this capture, no Activity in Span
fun getClickableTextSpan(onClick: (View) -> Unit) = object : ClickableSpan() {
    override fun onClick(widget: View) { onClick(widget) } // ✅ SAFE: caller resolves context from widget
    override fun updateDrawState(ds: TextPaint) {
        ds.isUnderlineText = true
        // ✅ SAFE: do not read Activity resources here; use ds.color statically or widget.resources
    }
}

// LoginFragment.kt
private fun ActivityLoginBinding.termAndPrivacyTextSetup() {
    val desc = root.resources.getString(R.string.desc)
    val terms = root.resources.getString(R.string.terms)
    val privacy = root.resources.getString(R.string.privacy)
    val text = "$desc $terms and $privacy"
    val spannable = SpannableString(text)

    spannable.setSpan(getClickableTextSpan { widget -> // ✅ SAFE: View param, not captured Activity
        // resolve context lazily from the clicked View
        DataConstant.URL_TERMS.browser(widget.context, DataConstant.URL_TERMS_HTML)
    }, desc.length + 1, desc.length + terms.length + 1, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)

    textDescription.movementMethod = LinkMovementMethod.getInstance()
    textDescription.text = spannable
}

// ✅ SAFE / CLEAN FIX: symmetric cleanup in matching destroy
override fun onDestroyView() {
    super.onDestroyView()
    // break GC Root → TextView → Spannable → Span chain
    binding?.textDescription?.apply {
        text = null
        movementMethod = null
    }
    _webView?.destroy()
    _webView = null
}
```

## Essence

The singleton `LinkMovementMethod` is a GC Root, but it is not the direct holder — the **Span capturing `Activity`** is. Making the `Span` stateless (context from `View` param) removes the `Large → Small` edge (`Span → Activity`). Nullifying `text`/`movementMethod` in `onDestroyView` cuts the remaining `GC Root → View → Span` anchor, so the whole island becomes unreachable and ART sweeps it.
