# Clean Code Examples

## 1. Naming
- **Function names must be clear verbs:**
  - ✅ `calculateTotal()`
  - ❌ `total()`
- **Rule 1: Name must reflect meaning.**
  - ✅ `UserSessionRepository`, `InvoiceValidator`
  - ❌ `data`, `manager`, `helper`
- **Rule 2: Avoid confusing abbreviations.**
  - ✅ `userRepository`
  - ❌ `usrRepo`
- **Rule 3: Avoid unnecessary prefixes.**
  - ✅ `user`, `name`
  - ❌ `mUser`, `strName`

## 2. Null Safety
**Rule: Avoid `!!` as much as possible.**
```kotlin
// ❌ BAD
val name = user!!.name

// ✅ GOOD
val name = user?.name ?: return
```
