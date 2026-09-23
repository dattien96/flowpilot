# BUG-376: Devin missing from `shouldInjectFeatureHistory` allowlist — no feature-history/CA "why" injection on devin turns

## Metadata

- Document ID: `BUG-376`
- Title: `Devin turns receive raw prompt only — canonical head, ordered History, and Discussion blocks never injected`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-35-Context-And-Regression-Engine-Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-37-Prompt-Context-Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md)
- Feature Keys: `context-regression-engine, ai-providers`

## AI Quick View

### Summary

- `shouldInjectFeatureHistory` (`apps/local-runner/internal/runner/interactive_service.go:8310`) whitelists only `codex|claude|gemini|grok`; `ProviderKeyDevin` falls to `default: false` → the call site (`:7556`) skips `injectFeatureHistoryPromptCtx` for every devin turn.
- Devin prompt artifacts contain ONLY the raw user text — no `## Canonical "<feature>"`, `## History … (newest = truth)`, or `## Discussion` blocks. Feature confidence/sticky context cannot be verified in-prompt for devin; the gate reprompt also lacks inherited feature context.
- Confirmed on two independent live beds: CP-35 (devin turn-3 prompt = 151 B raw vs codex turn-731 = 2434 B and grok turn-747 = 2698 B, both with full blocks) and CP-37 (all devin turns raw-only; claude control turn-933 = 1265 B with History + Discussion + CA excerpt, proving fixture + renderer work).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: On devin runs, `prompt-turn-*.txt` artifacts are byte-identical to the raw user prompt; no feature-resolution context is ever prepended.
- **Expected**: `## History "<feature>" (newest = truth)` + `## Discussion "<feature>" (newest last)` blocks prepended, as on whitelisted providers.
- **Actual**: cp37 run-1/run-1981 — turn-3 (74 B), turn-938, turn-1512, turn-1973, turn-1983, turn-2292, turn-3063, turn-3154, turn-3262 all carry raw text only (`[prompt] … bytes=74`); reprompt turn-2783 also lacked inherited feature context. cp35 run-1 — turn-3 = 151 B, turn-1138 = 409 B raw only.
- **Impact**: HIGH for CP-35/CP-37 features — the entire ordered-history + CA-"why" injection is inert for provider devin; sticky resolution/pivot and cross-feature mixing guards are unverifiable end-to-end on devin, and devin agents work with systematically less context than other providers.

## Reproduction

1. Bed with seeded `.flowpilot/ledger/feature_history.ndjson` + `chat_summary.ndjson` + catalog (cp37 fixture), gate `enforce`.
2. `POST /client/workflow-runs` `{projectId, providerKey:"devin", model:"devin/swe-2-max", yoloMode:true}` → run-1.
3. `POST /client/workflow-runs/run-1/turns` `{stepId:"chat-run-1", prompt:"What would it take to add a safe arithmetic divide operation to calc-core?"}` → turn-3.
4. Inspect `<runner-cwd>/.flowpilot/runs/<proj>/run-1/prompt-turn-3.txt` → raw prompt only (74 B on cp37; `runner.log:37`). Controls: codex `L351-codex-prompt-turn-731.txt` (2434 B), grok `L352-grok-prompt-turn-747.txt` (2698 B), claude `l371-claude-prompt-turn-933.txt` (1265 B) all contain the injected sections.

## Root cause

- `apps/local-runner/internal/runner/interactive_service.go:8310-8317` — `shouldInjectFeatureHistory` switch: `case ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGemini, ProviderKeyGrok: return true; default: return false` — devin absent.
- `interactive_service.go:7556` — `if s.shouldInjectFeatureHistory(rs.providerKey) && !skipHistory { … }` → devin turns never reach `injectFeatureHistoryPromptCtx`.

## Evidence

- `~/fp-beds/lt-evidence/cp35/RESULT.md` (BUG-LIVE-001), `L351-devin-prompt-turn-3.txt` (151 B), `L352-devin-prompt-turn-1138.txt` (409 B), `L351-codex-prompt-turn-731.txt` (2434 B), `L352-grok-prompt-turn-747.txt` (2698 B), `L351-events.json`.
- `~/fp-beds/lt-evidence/cp37/RESULT.md` (BUG-LIVE-CP37-001), `prompts/prompt-turn-3.txt` / `prompt-turn-1983.txt` / `prompt-turn-3154.txt`, control `l371-claude-prompt-turn-933.txt`, `gate-log-greps.txt`.

## Severity

- high

## Completion Notes (implemented 2026-09-22, CA-916)

- Root cause: `shouldInjectFeatureHistory` allow-list omitted `ProviderKeyDevin`, so Devin turns received the raw prompt without canonical head/history/discussion context.
- Fix: added `ProviderKeyDevin` to the allow-list (`interactive_service.go`).
- Tests: `bug376_devin_feature_history_test.go`. Additive-only change; claude/codex/gemini/grok paths untouched.
