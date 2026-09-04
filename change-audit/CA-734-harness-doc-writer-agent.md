# CA-734 — harness plan/doc writers use a dedicated doc-writer agent (task-harness S3)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: plan_writer/cp_plan_writer/task_splitter now spawn from a new document-only doc-writer agent (Read/Write/Grep/Glob, no Edit/Bash) instead of the coder agent, and the templated write contract carries an ONLY-these-files + no-commands scope guard — fixing plan_writer (and sibling doc nodes) implementing source+tests+CA during the plan phase instead of writing only the plan artifact
# --->8---

## Problem

- Live task-harness S3 (run-198924): the `plan_writer` node (still in the plan phase, before freeze) implemented the entire change — wrote `calc.go`, `calc_gcd_test.go`, a `change-audit/CA-*.md` note, `Task-904`, pushed `done/`, then `Task-905` — instead of writing the single `requirements/08-Task/todo/Task-<n>-*.md` plan artifact.
- Root cause: `plan_writer` spawned from `agents/coder.md`, whose system prompt is "You are the implementation agent… make focused changes… run the requested validation" with tools `[Read, Edit, Write, Bash, Grep, Glob]`. That identity won over `plan-task.md` (appended) + the user GCD prompt. The templated write contract said a file *must* be written but never said *only* those paths and *no* commands.
- Same class: `cp_plan_writer` and `task_splitter` (cp-harness, cp-harness-smoke) also spawned from `coder.md`.

## Changes

- New `flow-pack/agents/doc-writer.md`: document-only identity (role `doc-writer`), tools `[Read, Write, Grep, Glob]` — no Edit (cannot patch source), no Bash (cannot run tests/commands) — and an explicit must-NOT scope guard (no source/tests/config, no `change-audit/*.md`, no git). Registered in `manifest.yaml`.
- `flows/task-harness.yaml`: `plan_writer` → `agents/doc-writer.md`. `flows/cp-harness.yaml` + `flows/cp-harness-smoke.yaml`: `cp_plan_writer` + `task_splitter` → `agents/doc-writer.md`. Real code writers unchanged (`implement` stays `agents/coder.md`, `test_signatures` stays `agents/tester.md`).
- `runner/artifact_type_registry.go` (`appendTemplatedFileArtifactOutputsPrompt`): the templated write contract now states the node must write ONLY the listed template files, not source/tests/config/CA, and must not run commands or tests. (Templated-output nodes are exactly the doc writers; `implement` uses the concrete-path contract, unaffected.)
- `flow-pack/prompts/plan-task.md`, `plan-cp.md`, `task-splitter.md`: added a `## Scope guard — you are a DOCUMENT WRITER` section mirroring the same boundary.

## Tests added (new files only)

- `agentpack/harness_doc_writer_pack_test.go`: all five doc nodes reference `agents/doc-writer.md`; `implement`/`test_signatures` stay on coder/tester; `doc-writer` spec exposes Write but not Edit/Bash and carries the must-NOT guard; task-harness/cp-harness/cp-harness-smoke still validate.
- `runner/harness_doc_writer_prompt_test.go`: `composeFlowNodeAgentPrompt(plan_writer)` contains the ONLY-these-files guard + no-commands guard + templated write contract; `resolvePackAgentDefinition("doc-writer")` resolves from the embedded pack without Edit/Bash.

## Verification

- `go test ./internal/agentpack/ -count=1` and `go test ./internal/tui/app/ -count=1` full packages PASS; `go test ./internal/runner/ -run 'TestHarnessDocWriter|TestResolvePackAgentDefinition|TestHarnessTemplated|TestRun198699|TestTaskHarness|TestApplyFlowControl|TestCPHarness|TestBUG327|TestResolveContinueBackEdge|TestBug353'` PASS. Old tests untouched and green (R1).
- Provider-agnostic by construction: the node's agent identity + composed prompt are injected verbatim into every provider's spawn; the run198699 provider matrix (Claude/Codex/Grok) already exercises the plan_writer spawn path (R2).
- Not a hard runtime write-allowlist: doc-writer relies on identity + prompt scope guard; the freeze + flowgate exact-path gate still governs real code writers downstream.