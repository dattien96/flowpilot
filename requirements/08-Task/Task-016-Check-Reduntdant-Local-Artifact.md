Sometimes local workflow artifact folders remain after their workflow run has been deleted from Supabase.

Example local path:

```text
.flowpilot/artifacts/392355a6-5573-44f1-9aa5-4313502a5816/180cac0e-a709-4f52-9e11-44087dd2d8c5
```

In this path:

- `392355a6-5573-44f1-9aa5-4313502a5816` is the `project_id`.
- `180cac0e-a709-4f52-9e11-44087dd2d8c5` is the `workflow_run_id`.

When the local runner server starts, run a Go background cleanup job:

1. Scan `.flowpilot/artifacts/{project_id}/{workflow_run_id}` directories.
2. For each `project_id`, query the current Supabase `workflow_runs` table.
3. If a local `workflow_run_id` no longer exists in `workflow_runs` for that project, delete the whole local run folder and all child folders.
4. Skip `.flowpilot/artifacts/local/...` prompt-execution artifacts because those are local-only runner artifacts.
5. If Supabase config is unavailable or the lookup fails, log and skip cleanup instead of blocking server startup.

Current redundant example:

```text
.flowpilot/artifacts/392355a6-5573-44f1-9aa5-4313502a5816/180cac0e-a709-4f52-9e11-44087dd2d8c5
```
