# CP-06-01: Artifacts Sync

## 1. Goal

Sync local workflow artifact data to remote storage without losing the version history created by follow-up prompts.

The local artifact model is not a single file:
- each step run has a canonical parent output file, for example `BusinessIdea.md`
- each prompt attempt writes its own immutable snapshot folder under `.snapshots/<artifactId>/`
- the parent file is overwritten by the latest prompt on the same step/session

That means remote sync must preserve both behaviors:
- the latest parent file is the canonical remote artifact
- each snapshot folder is the historical record for one prompt attempt

## 2. Observed Local Contract

From the runtime output at:
- `/.flowpilot/artifacts/<projectId>/<workflowRunId>/<stepType>/BusinessIdea.md`
- `/.flowpilot/artifacts/<projectId>/<workflowRunId>/<stepType>/.snapshots/<artifactId>/`

The local structure is:

```text
.flowpilot/artifacts/<projectId>/<workflowRunId>/<stepType>/
  BusinessIdea.md              # canonical latest artifact content
  .snapshots/
    <artifactId>/
      manifest.json
      prompt.md
      stdout.txt
      stderr.txt
      command.txt
      BusinessIdea.md          # snapshot copy of the output content
```

Key behavior:
- multiple prompts on the same step create multiple snapshot folders
- the parent `BusinessIdea.md` is overwritten by the latest prompt
- the snapshot `manifest.json` records the snapshot metadata and absolute local paths

## 3. Sync Model

The remote side should mirror the local shape in two layers.

### 3.1 Canonical artifact

The canonical remote artifact is the latest file for that step output.

Example remote path:

```text
artifacts/<projectId>/<workflowRunId>/<stepType>/BusinessIdea.md
```

This is the file that should be treated as the current visible output of the step.

### 3.2 Snapshot history

Each snapshot folder should also be uploaded as immutable version history.

Example remote path:

```text
artifacts/<projectId>/<workflowRunId>/<stepType>/.snapshots/<artifactId>/
  manifest.json
  prompt.md
  stdout.txt
  stderr.txt
  command.txt
  BusinessIdea.md
```

This keeps the full prompt/output trail available remotely for debugging and audit.

## 4. Remote Data Rules

1. Syncing a new prompt attempt must not delete earlier snapshots.
2. Syncing the latest attempt must update the canonical remote file path.
3. Remote metadata must identify the artifact run, step run, and snapshot artifact ID.
4. The sync operation must be idempotent so re-syncing the same snapshot does not duplicate rows or objects.
5. If a later prompt rewrites the parent file, the remote canonical file should be replaced, not appended.

## 5. Data Contract

The implementation should use the existing artifact run fields as the source of truth:
- `artifact_runs.local_path`
- `artifact_runs.remote_path`
- `artifact_runs.remote_url`
- `artifact_runs.sync_status`

Proposed interpretation:
- `remote_path` points to the canonical latest file for that artifact definition
- `remote_url` points to the remote object location of the canonical file when available
- snapshot folders are remote children of the canonical artifact path

The sync layer should also read the snapshot `manifest.json` so it can upload the supporting files (`prompt.md`, `stdout.txt`, `stderr.txt`, `command.txt`) alongside the output content.

## 6. Implementation Plan

### 6.1 Local scan

1. Discover all artifact run directories under `.flowpilot/artifacts/`.
2. Load each snapshot `manifest.json`.
3. Group snapshots by `workflowRunId`, `workflowStepKey`, and artifact definition.
4. Detect the canonical latest file in the parent artifact directory.

### 6.2 Remote upload

1. Upload the canonical latest file to the remote artifact path.
2. Upload the snapshot folder to a versioned remote `.snapshots/<artifactId>/` path.
3. Upload the supporting metadata files for every snapshot.
4. Persist the resulting remote URL or remote identifier back to the artifact run row.

### 6.3 Update semantics

1. When the same step gets a new prompt attempt, create a new snapshot remote folder.
2. Replace the remote canonical file with the newest parent file.
3. Leave older snapshot folders intact.

### 6.4 Recovery semantics

1. If remote sync fails for one snapshot, mark that snapshot as failed without blocking the whole run history.
2. Allow retry from the exact snapshot that failed.
3. Keep the local snapshot even if remote upload fails.

## 7. UI Expectations

The artifact browser should be able to:
- show the canonical latest output as the main artifact
- show the snapshot history as version metadata
- open the canonical file remotely when sync has succeeded
- expose a failure state when the latest canonical file has not synced yet

The UI should not treat the snapshot folders as disposable cache. They are part of the user-visible history for repeated prompts on the same step.

## 8. Acceptance Criteria

- A step with multiple prompts produces multiple snapshot folders locally.
- The parent output file always reflects the latest prompt result.
- Remote sync uploads the canonical file and the snapshot history.
- Re-syncing does not duplicate snapshot history.
- Older prompt attempts remain available remotely after later overwrites.
- The sync state on `artifact_runs` reflects the latest upload result.

## 9. Non-Goals

- Do not collapse the artifact model into a single mutable file only.
- Do not delete older prompt snapshots when syncing the latest artifact.
- Do not move artifact ownership away from `artifact_runs`.
- Do not replace the current local artifact layout.

