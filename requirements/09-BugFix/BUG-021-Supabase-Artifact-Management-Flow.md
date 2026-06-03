# Prepare
Make sure PROJECT use supabase as remote storage
- The local folder .flowpilot/artifacts empty now
- The supabase storage empty now

# Flow test 1 - Done check and fixed
## Step1: Trigger a new flow with output artifact - Ok, all bug fixed

### Correct parts
- Local artifacts generated in local path (.flowpilot/artifacts/...)
- Artifact count + 1 for loca.notsync tab in /artifacts page
- Supabase bucket is empty now

### Bug check round 1 - Fixed
- No new row in table artifact_runs on supabase db

### Bug check round 2
- New row created in artifact_runs even for steps do not have OUTPUT artifact definition -> OK

- New bug: in artifacts page now
Show both duplicate artifact in local + remote
Rule: if 1 artifact row exist in both remote + local -> show in remote tab only

### Bug check round 3
After fix round 2, now 1 arifact count in remote tab. It is not correct actually

Remote mean: We synced it to supabase/driver
It is not we have 1 row in artifact_runs table. this row is just for save metadata
You need see the sync_status field to know this artifact was synced or not

So for this case, the status is local_only
-> must count artifact in local tab

## Step 2: Open artifact in local tab - OK

## Step 3: Press SYNC artifact - Ok, all bug fixed

Got error: artifact-run-browser-panel.tsx:211 
 POST http://localhost:3002/api/local-runner/artifacts/5c315ed6-a133-42dc-ae24-aed78f539078/sync 400 (Bad Request)
(anonymous)	@	artifact-run-browser-panel.tsx:211
(anonymous)	@	artifact-run-browser-panel.tsx:257
(anonymous)	@	artifact-run-browser-panel.tsx:302

### Bug check round 1
After fix bugs in Step 1 this sync func work
Pressing sync we got

- +1 count for Remote tab. 0 for local part
- Local part data in .flowpilow still exist
- Data artifact pushed to supabase storage bucket already
- sync-status field in artifact-runs table changed to "synced" instead of local-only

## Step 4: Del local data - just keep remote - Ok, all bug fixed

### Correct parts
Correct, after synced, even del local part but we still have count for remote tab

### Bug round 1 part
But when we back to the workflow-runs page
Then runs show ID instead of real data (workflow-runs card)

My guest is: cause we del local data -> while we show title base on local data

-> Expectation: need check sync-status of the artifact runs related to that workflow-runs
to know which src we can get title

## Step 5: Open artifact in remote tab - Ok, all bug fixed

### Correct parts
It can download exact file from remote

### Bug parts
But i want to open it directly in browser like local mode instead download it


## Step 6: Del workflow runs - Ok, all bug fixed
### Correct parts
- table artifact-runs empty
- table workflow-runs empty
- local part deleted

### Bug parts
The remote artifacts was not del

# Flow test 2 - Check BG thread that sync automatically after start server - Ok Fixed Bug
## Step 1 - Trigger a new flow
### Correct parts
- Local artifacts generated in local path (.flowpilot/artifacts/...)
- Artifact count + 1 for loca.notsync tab in /artifacts page
- Supabase bucket is empty now
- New row with local_only status in artifact_runs table

## Step2 - Turn off and Re-start server

### Bug part
It might not start Bg Golang job to check 
all row in artifact_runs with local status
-> find artifacts in local .flowpilot folder
-> auto sync them

The sync button must be disable when the bg sync flow was started already

### Fixed root cause
- Startup bootstrap now uses admin/shared Supabase access so it can see `artifact_runs` rows with `local_only` status and trigger reconciliation correctly after server start.
- The local runner now loads missing `SUPABASE_*` values from the workspace root `.env`, so background sync can reach Supabase Storage after restart without requiring exported shell env.
- Supabase wrapped missing-object responses (`400` with `statusCode=404`) are treated as "object does not exist yet", so first-time startup sync continues to upload instead of failing early.
- Bootstrap reconcile is now requested only once per runner `startedAt`. The old 30-second health poll was retriggering bootstrap repeatedly and producing repeated `bootstrap requested/scheduled` and `reconcile candidates=0` logs even when nothing changed.

# Flow test 3 - Check BG thread that sync automatically after Timeout when workflow done - OK fixed

## Step1 - Trigger a new flow
## Step2 - Wait job done

## BUG
after timeout, there is no sync process trigger

### Fixed root cause
- The delayed auto-sync scheduler in `artifact-auto-sync.ts` expects arguments as `(context, filters)`.
- `workflow-start-runtime.ts` was calling it in the reverse order `(filters, context)`.
- That meant the timeout callback later ran with `context.supabase === undefined` and crashed on `context.supabase.from(...)`, so the background sync never started after the timeout.
- The call order is now fixed in both workflow execution paths, and the scheduler now throws a clear error immediately if it is called without a valid sync context.

# Flow test 4 - Check BG thread that sync automatically after Timeout when workflow done

## Step1: Trigger a new flow
## Step2 - Wait 1/2 timeout
## Step 3: continue make prompt

### Verified behavior
- Auto-sync debounce is keyed by `projectId:workflowRunId`.
- If the same workflow run produces another artifact/update before the 5-minute timeout expires, the previous timer is cleared and a new 5-minute timer starts from the latest activity.
- Reconcile should therefore happen 5 minutes after the latest prompt/activity for that workflow run, not 5 minutes after the first artifact in the run.

# Flow test 5 - Navigate away during timeout countdown

## Step1: Trigger a workflow that creates local artifact output
## Step2: Wait while the 5-minute auto-sync timeout is counting down
## Step3: Navigate to other pages in admin-web before timeout finishes

### Expected behavior
- Page navigation should not cancel the 5-minute countdown.
- The delayed auto-sync timer is scheduled from workflow runtime code and lives in the admin-web server process, not in the artifacts page component state.
- The artifact should still be reconciled and synced when the 5-minute timeout expires, even if the user is currently viewing another page.

### Important caveat
- If the admin-web server process restarts before the timeout expires, the in-memory countdown is lost.
- In that case, startup bootstrap reconciliation should recover pending `local_only` artifacts after the server comes back up.

# Flow test 6 - mutliple artifacts in 1 workflows missed in synced
## Step 1: Trigger single simple step

- See the local artifact in .flowpilot/artifacts
- New row in artifact_runs table with local_only status
- No data in supabase bucket

## Step 2: Continue add 1 more prompt in same flow
- See the local artifact in .flowpilot/artifacts-> now we have 2 child folder artifacts
- 1 New row in artifact_runs table with local_only status -> now we have 2 rows
- No data in supabase bucket

## Step 3: Press Sync button

### Correct parts
- 2 rows in artifact_runs table changed to synced
- still 2 artifacts data in local .flowpilot/artifacts

### BUG parts

- Ui state in View: ONLY 1 Synced in remote tab
- New data push to bucket storate but only artifact of 1 prompt instead of 2 prompts pushed

### Fixed root cause
- Supabase canonical object paths were only keyed by `projectId/workflowRunId/workflowStepKey/outputFilename`.
- When the same workflow step produced multiple artifacts in one workflow run, later syncs overwrote the earlier canonical file because both rows pointed to the same remote path.
- The artifacts page also merged remote rows by `remotePath`, so those overwritten artifacts collapsed into a single remote item in the UI.
- Canonical sync paths are now artifact-scoped by including `artifactId`, while snapshot/history paths stay unchanged.
- Remote path parsing and open-file resolution remain backward-compatible with the older non-artifact-scoped paths so previously synced artifacts still render and open correctly.
