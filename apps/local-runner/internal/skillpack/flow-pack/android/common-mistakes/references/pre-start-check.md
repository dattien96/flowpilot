# Common Mistakes Self-Check Example

## Scenario: Rushed Implementation

**User**: "Add a button to the profile screen to reset data."

**❌ MISTAKE (Draft 1)**:
Agent immediately starts writing code in `ProfileScreen.kt` and adds a `resetData()` function to `ProfileViewModel`.

**✅ CORRECT (Self-Correction using Checklist)**:
1. **Mistake 1 (Missing Context)**: "Wait, which 'reset data'? Just local or also remote Supabase data?"
2. **Mistake 2 (Cramming)**: "Should I also add the confirmation dialog in the same task?"
3. **Mistake 6 (Assumptions)**: "I should ask if this needs an extra permission or if it's available to all users."

**Action**: Agent stops and asks: "Before implementation, I have 2 questions: 1. Does the reset affect Supabase or just local cache? 2. Should I include a 'Are you sure?' confirmation dialog?"
