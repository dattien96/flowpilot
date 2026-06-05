### Real issues found during live test

#### Issue A: Picker API key rejected when restricted incorrectly

Observed behavior:

- Google Picker opened an error page:

```text
The API developer key is invalid.
```

Root cause found during testing:

- this was not an OAuth problem
- it was caused by Google Picker API key restriction settings
- changing the Picker key restriction from `Websites` to `None` allowed the folder browser to open

Follow-up note:

- after the flow is fully stable, the Picker key should be tightened again with correct website restrictions for the runner host

#### Issue B: Folder selection looked successful but nothing returned to FlowPilot

Observed behavior:

1. user selects a folder
2. `Select` button becomes enabled
3. user presses `Select`
4. the Google dialog stays open and resets to its initial state
5. the FlowPilot page does not update
6. runner session remains stuck in:

```text
awaiting_folder_selection
```

Root cause found during browser investigation:

- the Google Picker popup URL was being generated with a broken parent bridge value similar to:

```text
parent=http://127.0.0.1:4317/favicon.ico
```

- this meant the picker callback channel back into the FlowPilot wrapper was not wired correctly
- live browser inspection also showed the detached Google page had no usable opener bridge

### Fixes already applied in code

Runner picker flow was patched to:

- use explicit Google Picker origin:

```text
window.location.protocol + "//" + window.location.host
```

- use an explicit relay page:

```text
/artifact-storage/google-drive/picker-relay
```

- register that relay page as a real runner route
- keep debug output visible on the wrapper page for Picker callback payloads and folder save responses
- preserve the earlier fallback that trusts Picker folder payload when a follow-up Drive lookup returns `403` or `404`

Files updated during investigation:

- `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
- `apps/local-runner/internal/runner/artifact_google_drive_connection_test.go`
- `apps/local-runner/internal/cli/root.go`

### Current live state after the relay/origin fix

The wrapper page now stays on the local runner URL instead of silently losing control to a broken Google page:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/picker?sessionId=...
```

The wrapper debug state now reports at least:

```json
{ "action": "loaded", "docs": [] }
```

And the embedded Google Picker iframe now uses a relay-style parent value based on:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/picker-relay
```

instead of `/favicon.ico`.
