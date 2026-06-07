- Handle accessToken + refreshToken after login + select folder in google driver
- Invoke new token automatically
  - Access tok expired -> use refresh tok to acquire new one
  - Refresh tok expired -> Confirm user login again (Same func with the button CONNECT Google Drive in /artifacts page)

# Bug note
## 1. (Fixed) Open any file .md we got: The browser blocked the artifact tab. Allow popups for FlowPilot and try again.

In both detail workflow-run page + artifact list page

Root cause:
- Workflow-run detail used `window.open(...)` for artifact opening, so the browser treated it as popup-style behavior and could block it.
- Artifact list links also had edge cases around encoded artifact IDs and nested clickable summary UI, which made file opening unreliable for remote/recovered artifacts.

Fixed solution:
- Replaced the workflow-run detail open action with a normal anchor link using `target="_blank"` instead of scripted `window.open(...)`.
- Normalized artifact open href generation so artifact IDs are URL-encoded and link clicks do not bubble into the details/summary toggle behavior.

## 2. (Fixed) page /artifacts show total 13 artifacts
why we only have 4 artifacts active (that belong our workflow-run + project) in our supabase
Other is old aritfacts.
it is still keep in remote storage for reference. i dont del them after del workflow-runs
But yuo need to show only active in UI

BUt i leave the page for abit then i see it automatically update the number to 4
-> I think we have some bg process here. We need so correctly 4 whenever i access the page, not waiting like that

Root cause:
- Global `/artifacts` initially loaded shared artifact rows directly, including old remote/storage-backed artifacts whose workflow runs had already been deleted.
- A later refresh/state reconciliation path eventually reduced the list, which is why the count looked wrong first and corrected itself later.

Fixed solution:
- During the first `/artifacts` page load, we now load active workflow runs together with artifact rows.
- Remote/shared artifact rows are filtered immediately to workflow run IDs that still exist, so old storage leftovers do not appear as active UI items.

## 3. (Fixed) If the artifact was deleted from local
then the title of it is flicking change between real tile + RESPONSE.md
you need to care the case the artifact sync to remote then was deleted in local

Root cause:
- When the local artifact file was gone, the remote card started from the generic stored title `Response.md`, then fetched remote content to derive a better title.
- That hydrated title was stored only as a UI override, but the generic-title detection was still reading the changing display title instead of the stable source row title, so the override could be cleared and re-added repeatedly.
- In replica-backed cases, title hydration could also read from the DB row with empty `remotePath` instead of the active remote replica path.

Fixed solution:
- Title hydration now decides whether a title is generic from the original artifact row title, not from the already-overridden display title.
- Remote title fetching now uses the active replica `remotePath`, `remoteObjectId`, and provider when the local file is missing.
- Override map updates were stabilized so unchanged empty/derived maps are not recreated every render, preventing flicker loops.

## 4. (Fixed) The Google picker Api key is invalid
I got this one when i connect google drive

Current state key in console is
Set Restriction to Web
and add
127.0.0.1:4317 and 127.0.0.1:4317/*

But when i test reset to NONE restriction then It works, i can select drive folder

Root cause:
- The setup guide only said local MVP can keep the API key application restriction as `None`, but did not show the correct website referrer format for a restricted Google Picker key.
- Google API key website restrictions require full HTTP referrer patterns. Values like `127.0.0.1:4317` and `127.0.0.1:4317/*` omit `http://`, so Google rejects the browser-side Picker key even though an unrestricted key works.

Fixed solution:
- The Google Drive setup page now shows the exact local restricted referrers to add:
  - `http://127.0.0.1:4317/*`
  - `http://localhost:4317/*`
- The guide text now explicitly says not to omit `http://` when using Website application restrictions.

## 5. (Fixed) After selected drive folder
i see we show the folder data done
Data gen in artifact-storage-google-drive.json is ok

But we did not save it as active src. It only supabase in project table of db

Root cause:
- Folder selection only saved the Google Drive connection metadata in the runner/local connection state.
- The project provider switch still required a second manual `Select Google Drive` click, so `projects.artifact_storage_preference` remained `supabase` after the picker completed.
- The status route also attempted to mirror the connected folder into `artifact_storage_connections` with a session-scoped Supabase client and swallowed write failures. When that write was blocked or skipped, the migration endpoint used the admin client, found no connected Google Drive row, and failed with "Google Drive artifact storage is not connected on this runner for the selected project."

Fixed solution:
- When a live Google Drive picker session reports a connected folder, `/artifacts` now automatically activates Google Drive for that project.
- The auto-activation uses the existing `/api/local-runner/artifact-storage/switch-provider` migration endpoint, so artifact sync/backfill still runs before `projects.artifact_storage_preference` changes to `google_drive`.
- A duplicate guard prevents repeated polling/message refreshes from calling the switch endpoint more than once for the same project/folder.
- The Google Drive status route now persists connected folder metadata with the runtime Supabase admin client before migration can run. If a connected-folder metadata write fails, the route returns the real error instead of letting the UI start a migration that will fail validation.

## 6: (Fixed) switch drive sync failed
e730b90d-1650-4e53-aee6-99654368fdc3: google drive upload failed: 403 { "error": { "code": 403, "message": "Properties and app properties are limited to 124 bytes in UTF-8 encoding, counting both the key and the value.", "errors": [ { "message": "Properties and app properties are limited to 124 bytes in UTF-8 encoding, counting both the key and the value.", "domain": "global", "reason": "propertyLengthLimitExceeded" } ] } }

f41d68f7-8d02-416a-b30b-4a89438b7b16: google drive upload failed: 403 { "error": { "code": 403, "message": "Properties and app properties are limited to 124 bytes in UTF-8 encoding, counting both the key and the value.", "errors": [ { "message": "Properties and app properties are limited to 124 bytes in UTF-8 encoding, counting both the key and the value.", "domain": "global", "reason": "propertyLengthLimitExceeded" } ] } }

Provider switch stopped with 2 failures. Synced 0, skipped 0. google drive upload failed: 403 { "error": { "code": 403, "message": "Properties and app properties are limited to 124 bytes in UTF-8 encoding, counting both the key and the value.", "errors": [ { "message": "Properties and app properties are limited to 124 bytes in UTF-8 encoding, counting both the key and the value.", "domain": "global", "reason": "propertyLengthLimitExceeded" } ] } }

Root cause:
- Google Drive `appProperties` has a small 124-byte limit for each key/value pair.
- The Drive canonical artifact upload included long path metadata such as `remotePath`, and some snapshot uploads could include long `relativePath` values. Those paths can exceed Google's appProperties limit even though the actual Drive folder/file path is valid.

Fixed solution:
- Google Drive uploads now sanitize appProperties before sending them to Drive and omit any property whose key plus value exceeds 124 bytes.
- Canonical Drive uploads no longer store `remotePath` in appProperties because the durable remote path is already stored in FlowPilot metadata and represented by the Drive folder layout.
- The fake Google Drive test API now enforces the same appProperties byte limit so this 403 case is covered locally.

UI note:
- Provider switch migration progress now renders as a vertical seven-step list.
- The current step shows a loading icon at the beginning of the line and highlighted text instead of a horizontal badge row.
