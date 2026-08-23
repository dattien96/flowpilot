# UI Extraction Examples

## 1. Component Extraction Decision Tree

```
Can the component be extracted?
│
├─ Is it used in ≥2 DIFFERENT features?
│  └─ YES → Move to :core-foundation:appdesign
│
├─ Is it used multiple times WITHIN a single feature?
│  └─ YES → Move to /components/ of that feature
│
└─ Is it used only once and as a private function?
   └─ YES → Keep it in the Screen file (OK)
```

## 2. Scanning Checklist Example

1. ✅ Read all Screen files in the package.
2. ✅ List all `@Composable` functions (both public and private).
3. ✅ Identify duplicate UI patterns (Logo+Title, Loading Overlay, etc.).
4. ✅ Check if the component has been extracted; if not, extract it.
5. ✅ Verify the component is placed correctly (appdesign vs feature/components).

## 3. Patterns to Extract

- Logo + Title + Subtitle header (used in Login + Register).
- Loading overlay with CircularProgressIndicator (used in multiple screens).
- Error message display pattern.
- Form field groups (Email + Password fields).
- Divider with text ("OR" divider).
