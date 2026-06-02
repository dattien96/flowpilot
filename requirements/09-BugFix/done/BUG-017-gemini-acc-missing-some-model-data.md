# BUG-017 Gemini account card missing Gemini 3.x data

Symptom:
The Gemini account card only showed `gemini-2.5-*` usage lines after connecting an account.

Root cause:
The account metadata formatter sorted Gemini quota buckets alphabetically and then truncated the list to the first three items. That dropped the newer `gemini-3*` buckets whenever more than three entries were returned.

Fix:
Removed the `slice(0, 3)` limit from the Gemini quota usage mapping so the account card can render all returned model buckets.

Verification:
- Added unit coverage for the Gemini quota mapping helper.
- Confirmed the focused admin-web test file passes.
