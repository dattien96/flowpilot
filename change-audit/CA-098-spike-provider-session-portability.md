# CA-098 — Spike: Provider Session File Portability (Cross-Account / Cross-PC)

> **Spike, not a code change.** This note records a feasibility test that gates Task-067, Task-068, Task-069. No production code was modified.

## Purpose

The whole "share chat across accounts / across PCs" goal hinges on one question:

> Can a provider session file (Codex rollout / Claude `*.jsonl`) be **moved to a different account home or a different PC and still resume?**

- **YES** → native resume path (copy file + resume, reuse the conversation). The user's preferred outcome.
- **NO** → the run is shown **greyed-out / disabled** with a reason (the agreed fallback, user decision 2026-06-17).

## Result (2026-06-17)

| Scenario | Provider | Verdict | Evidence |
|---|---|---|---|
| Cross-account, same PC | **Codex** | ✅ **PORTABLE (end-to-end)** | Account B's `codex` resumed account A's copied rollout **by id, with no `session_index.jsonl` entry**, then **completed a new turn** — model replied `PORTABILITY_OK`, loading the full 28k-token prior context, under B's own auth. |
| Cross-account, same PC | Claude | ⏳ untested | Only one Claude account on this machine. |
| Cross-PC | Codex | ⏳ untested | Needs a second machine; but the file is account-agnostic and resolved by id, so expected to work. |
| Cross-PC | Claude | ⏳ untested | Needs a second machine. |

**Two independent confirmations of Codex portability:**

1. **Structural** — the rollout's session-meta payload keys are `id, timestamp, cwd, originator, cli_version, source, model_provider, base_instructions, dynamic_tools`. **No account / auth / user / token / email / org field.** Nothing binds the file to the account that created it.
2. **Empirical** — under `CODEX_HOME=<account B>`, `codex exec resume <id>` found and resumed a rollout copied from account A. Run twice, which cleanly separates the two variables:
   - `.codexHome3` (**expired** login) — session loaded and the turn started, then failed on auth (`refresh_token_reused` / `token_expired`). Proves the *file* is accepted regardless of origin account.
   - `.codexHome4` (**valid** login) — model replied `PORTABILITY_OK` end-to-end, loading the full 28k-token prior context (exit 0). Proves the turn completes once the target account is authenticated.
   - Conclusion: **file portability = YES**, independent precondition = **target account must be logged in**.

**Key precondition discovered:** the *target* account must be **validly logged in**. Portability of the file is necessary but not sufficient — the account that resumes it must have working credentials to make the new model call. FlowPilot already manages account auth/verification, so this fits the existing model.

## Reusable Checklist

### A. Codex — cross-account (same PC)

```powershell
# 1. Pick a recent rollout from account A and note its session id (the UUID in the filename).
#    e.g. .codex\sessions\2026\06\17\rollout-...-<UUID>.jsonl
# 2. Copy it into account B's session tree at the same YYYY\MM\DD path (additive).
$src = "<A_HOME>\sessions\2026\06\17\rollout-...-<UUID>.jsonl"
$dstDir = "<B_HOME>\sessions\2026\06\17"; New-Item -ItemType Directory -Force $dstDir | Out-Null
Copy-Item $src (Join-Path $dstDir (Split-Path $src -Leaf))
# 3. Resume under account B, non-interactive, read-only, no approvals.
$env:CODEX_HOME = "<B_HOME>"
codex exec resume <UUID> --all -c 'sandbox_mode="read-only"' -c 'approval_policy="never"' "Reply with exactly: PORTABILITY_OK"
# 4. Clean up: remove the copied rollout (and any date dir you created) from B.
```

- **PASS** = Codex resumes and the model replies (target account must be logged in).
- **"session not found"** = Codex needs the `session_index.jsonl` entry too (copy that line as well).
- **auth/401 only** = file is portable; just re-login the target account (`codex login` under that `CODEX_HOME`).

### B. Codex — cross-PC

Same as A, but step 2 copies the rollout to the **second machine's** `CODEX_HOME\sessions\...`. Watch the recorded `cwd` — Codex filters by cwd unless `--all` is passed; the project may live at a different path on PC2.

### C. Claude — cross-account / cross-PC

```
# Claude sessions: ~/.claude/projects/<cwd-hash>/<sessionId>.jsonl   (cwd is HASHED into the path)
# 1. Note sessionId + the project hash dir on PC1.
# 2. Copy the .jsonl to the target home/PC at the same projects/<hash>/ path.
#    Cross-PC: if the project path differs, the <cwd-hash> differs — may need the matching path.
# 3. Resume:  claude --resume <sessionId>     (run from the matching cwd)
```

- PASS = Claude resumes the conversation. FAIL/"not found" = not portable → greyout fallback.

## Implications for the task chain

- **Task-068 (cross-account):** Codex confirmed portable. Build the relocate-file + resume path (T-7), gated only on the target account being logged in. Greyout fallback (T-6/T-8) applies when login is missing or a provider rejects the file.
- **Task-067 (post-restart resume):** even easier — same account, same PC, new process. Codex resolves a rollout by id straight from `CODEX_HOME/sessions` with no index entry, so reconstruction is viable.
- **Task-069 (cross-PC):** Codex cross-PC still to verify on a 2nd machine, but the account-agnostic file + id-based resolution make it likely. Claude (both axes) still to verify.

## Cleanup performed

- Removed the test rollout + the `sessions\2026\06\17\` date dir created in both `.codexHome3` and `.codexHome4` (neither had 06/17 sessions before). Both trees restored to pre-test state (newest dirs back to 06/15 and 06/16 respectively).
- Note: each target account's `session_index.jsonl` may carry one dangling entry from the resume (points to the now-removed rollout). Harmless; left untouched to avoid hand-editing the index.
- Observed separately: `.codexHome3`'s stored login is stale (`refresh_token_reused`) and `.codexHome1` has no `auth.json` — both need re-login before FlowPilot can use them for live turns. `.codexHome4` auth is valid.
