## BUG-026 Failed Regression After Task014

### Regression Checklist

1. `/workflow-runs`
   - Verify the workflow run list loads normally.
   - Verify realtime or live update behavior still works.
   - Verify no Supabase runtime error appears in the list view.

2. `/workflow-runs/:runId`
   - Verify the workflow run detail page loads.
   - Verify terminate-session and other run actions still work.
   - Verify the detail view still renders with the current runtime config.
   - FIXED: browser previously showed `Something went wrong!` and `getGatewayBundle is not defined`.

3. `/projects/:projectId/settings`
   - Verify the project settings page loads normally.
   - Verify project save/update actions still work.
   - Verify artifact storage-related controls still behave as expected.

4. `/artifacts`
   - Verify the artifact page loads normally.
   - Verify the cloud storage panel renders.
   - Verify artifact sync and storage actions still work with the current runtime source.
