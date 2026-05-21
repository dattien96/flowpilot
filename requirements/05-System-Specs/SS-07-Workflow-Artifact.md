# FlowPilot - Workflow Artifacts

This document defines how FlowPilot handles artifacts as reusable definitions plus runtime execution records.

## 1. Core Model

Artifacts should follow the same pattern as workflows and workflow steps:

- one reusable definition layer
- one runtime execution layer

That means FlowPilot must model:

1. `artifact definitions`
2. `artifact runs`

## 2. Artifact Definition

An artifact definition is a reusable template that describes what kind of file a step expects or produces.

Artifact definitions are similar to workflow step definitions:

- reusable across multiple workflows
- linkable from workflow steps
- editable in admin UI

### 2.1 Artifact definition fields

Each artifact definition should contain at least:

- name
- description
- default file name
- local output root path
- local input lookup path
- remote sync root path

Example:

```text
Artifact Definition:
  key: plan_artifact
  name: Plan Artifact
  description: Main implementation plan document
  default file name: Plan.md
  local output root path: /root/plan_architect
  local input lookup path: /root/plan_architect
  remote sync root path: /artifacts/plan_architect
```

### 2.2 Why local output path and local input path both exist

The output root path tells the runner where a step should write the artifact.

The input lookup path tells the runner where a later step should expect to find the artifact when that artifact type is declared as input.

In many cases those two roots may be the same, but they should still be modeled explicitly so future workflows can route write and read paths differently if needed.

## 3. Artifact Run

An artifact run is the real runtime artifact record created when a workflow step executes.

It must reference:

- artifact definition
- workflow run
- workflow step run
- resolved local path
- resolved remote path
- sync status

### 3.1 Artifact run behavior

When a step with one or more output artifact definitions executes:

1. resolve each configured artifact definition
2. resolve the final runtime local path for each definition
3. generate the local file for each produced artifact
4. create an artifact-run record for each produced artifact
5. optionally sync each artifact to remote storage later

When a step with one or more input artifact definitions executes:

1. resolve each configured artifact definition
2. find the matching prior artifact-run record for this workflow run
3. verify the local file exists at the resolved path
4. load each resolved local file as step input
5. fail if any required input artifact file is missing

## 4. Step Relationship to Artifacts

Each workflow step may link:

- zero or more input artifact definitions
- zero or more output artifact definitions

Rules:

- input artifact definitions are optional as a set
- output artifact definitions are optional as a set
- if one or more required inputs are configured and the runner cannot resolve any required local file, the step fails
- if no output artifact definitions are configured, the step may still run and simply produce no file-backed artifact

### 4.1 Example

If a step is configured with:

- output artifact definitions = `plan_artifact`

and the definition has:

- local output root path = `/root/plan_architect`
- default file name = `Plan.md`

then the runner should create the real output under a runtime folder such as:

```text
/root/plan_architect/{projectId}/{workflowRunId}/{stepType}/Plan.md
```

Then if the next step is configured with:

- input artifact definitions = `plan_artifact`

the runner should:

1. find the matching artifact-run for `plan_artifact`
2. verify the file exists in the resolved local path
3. load `Plan.md`
4. fail if the file is missing

If the next step has no input artifact definitions, it ignores artifact lookup.

If the next step instead requests `coding_artifact`, it must resolve `Coding.md` or the configured runtime file for that artifact definition, not `Plan.md`.

## 5. Local-first Storage Principle

FlowPilot should treat local artifact files as the first execution surface.

This means:

- each step writes local file first
- next step reads local file first
- remote sync must not be required for workflow execution

Remote storage is a secondary concern used for:

- persistence
- backup
- sharing
- audit

## 6. History and Versioning

Artifact runs should preserve version history.

Every retry or regeneration should create either:

- a new artifact-run version
- or a new artifact-run row linked to the same definition and workflow step lineage

Users should be able to inspect:

- which artifact definition was used
- which workflow run created it
- which step created it
- which file path was written
- whether remote sync succeeded

## 7. Synchronization

Artifacts must support:

- local-only execution
- optional sync to online storage such as Supabase Storage or Drive

Sync should be explicit in the model:

- `local_only`
- `queued`
- `syncing`
- `synced`
- `failed`

## 8. Built-in Artifact Definitions

For MVP, built-in workflow steps that produce file outputs should seed predefined artifact definitions.

Examples:

- `business_idea_artifact`
- `tech_spec_artifact`
- `coding_plan_artifact`
- `architecture_artifact`
- `task_breakdown_artifact`

Steps such as `Telegram Notification` may have no output artifact definitions.

The product model should support multiple input and output artifact definitions now, even if an early code slice persists only one primary input binding and one primary output binding internally.

## 9. Relationship to Artifact Memory

The raw artifact file remains the source of truth.

After a runtime artifact is created:

1. the local file is the execution-time source
2. the artifact-run record is the audit/source registry
3. working memory may be derived from that artifact-run later

This keeps the separation clear:

- artifact definition = reusable contract
- artifact run = real generated file instance
- artifact memory = searchable summary/embedding layer
