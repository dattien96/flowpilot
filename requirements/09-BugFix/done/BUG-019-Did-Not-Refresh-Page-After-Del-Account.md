# BUG-019 Did Not Refresh Page After Del Account

Symptom:
Deleting a provider account from the accounts page shows a "done notice" (toast/status message), but the deleted account continues to appear in the UI list. The user had to reload/refresh the page manually, but the deleted account would still persist or get re-added under a new ID.

Root cause:
When the frontend invokes the DELETE endpoint, the backend deletes the account's state metadata from `provider-accounts.json`, but leaves the actual account's home configuration directory (e.g. `~/.codexHome1`) on disk. Immediately after, the frontend invalidates and refetches the list. When fetching, the backend scans the home directory, finds the directory still exists with valid auth files, and automatically syncs/re-adds it back to the active state under a new ID. This made the deletion appear to have no effect.

What we changed:
- Updated the local runner backend's `DeleteProviderAccount` function in `apps/local-runner/internal/runner/provider_accounts.go`.
- Added logic to completely remove the managed account configuration home directory (e.g. `~/.codexHome1`) from disk using `os.RemoveAll` when the account is a managed slot (`SlotIndex > 0`).
- Implemented a safety check to ensure we only remove directories whose base name matches the expected provider prefix (e.g. `.codexHome`, `.claudeHome`, `.geminiHome`), protecting user files.
- Added comprehensive unit test coverage (`TestDeleteProviderAccountRemovesManagedHomeDirectory`) in `apps/local-runner/internal/runner/provider_accounts_test.go` verifying directory cleanup and list update behaviors.
- Fixed existing test isolation in `provider_accounts_test.go` on Windows by setting `APPDATA` and `USERPROFILE` environment variables in tests and using path filtering.

Verification:
- Added `TestDeleteProviderAccountRemovesManagedHomeDirectory` in `provider_accounts_test.go` and verified it passes cleanly.
- Verified that all unit tests in `provider_accounts_test.go` pass successfully on the target machine.
