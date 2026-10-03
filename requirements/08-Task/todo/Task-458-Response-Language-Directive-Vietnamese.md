# Task-458: Response-Language Directive — AI chat output in Vietnamese

- Document ID: `Task-458`
- Title: `Per-project response_language setting injected as a prompt directive so all AI chat text renders in Vietnamese`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-10-04`
- Last Updated: `2026-10-04`
- Parent Documents: ``
- Child Documents: ``
- Related Documents: `CA-1160` (reprompt seams), `gate_blind_hook.go`, `composeAgentSpawnPrompt`
- Replaces: ``
- Tags: `i18n`, `prompt`, `ux`, `vibe`, `chat`

## AI Quick View

### Summary

- The operator wants every AI-authored chat surface — final messages,
  questions, reprompt replies, leg results, debate summaries — in
  Vietnamese, without weakening the English instruction layer that
  carries machine-parseable contract language.
- Chosen approach: a `response_language` setting resolved per project
  (`.flowpilot/settings/chat-language.json`, runner default fallback)
  injected as a **directive clause** into composed prompts — NOT a
  translation of system prompts to Vietnamese.
- Explicit boundary: tool calls, verdict enums, flow_control payloads,
  file paths, `Task-NNN`/`AC-N`/`BUG-NNN` ids, commit subjects, and CA
  doc bodies stay English/machine-format — the runner's schema-first
  parsers consume them.

### Current Ask

- Implement the setting + directive injection at the two prompt seams
  (spawned-leg composer and the root-turn envelope), with provider-parity
  evidence that structured-output compliance does not degrade.

### Key Decisions

- `T-1` **Directive injection, not prompt translation.** System prompts
  stay English: they carry contract language the runner parses (tool
  names `submit_review_outcome`, verdict enums
  `approved|changes_requested|blocked`, schema literals, doc ids).
  Mixed-language prompts are strictly worse for instruction-following
  than either pure form, and ~20 pack files would need re-translation on
  every edit.
- `T-2` The directive is a fixed clause appended at composition time,
  e.g. `"Respond to the user in Vietnamese (Tiếng Việt) for all
  natural-language output — summaries, questions, explanations, final
  messages. Keep tool calls, status/verdict values, code, file paths,
  and structured outputs in their exact required format."` Rendered only
  when the resolved language is not `en`.
- `T-3` Two injection seams cover the whole surface: (a)
  `composeAgentSpawnPrompt` — the spawned-leg system prompt (coder,
  reviewer, tester, synthesizer, owner) whose final messages are the
  user-visible leg results; (b) the root/chat turn path — the durable
  directive applied to the run's prompt envelope so the main chat and
  every reinvoke keep responding in Vietnamese.
- `T-4` Resolution order: project `.flowpilot/settings/chat-language.json`
  `{"language": "vi"}` → runner default setting → `"en"`. No
  auto-detect from user prompt (a Vietnamese user may still want English
  answers; the setting is the contract).
- `T-5` Go-composed strings (gate messages, reprompt bodies, option-card
  text, step labels) stay English — they are runner telemetry, not AI
  output. Card/i18n of runner UI strings is a separate, larger feature
  and out of scope.

### Constraints

- Provider parity required (Claude/Codex/Grok): the directive must not
  degrade structured-output compliance — `submit_review_outcome` verdict
  payloads, `flow_control` statuses, contract declarations must remain
  byte-exact machine values. Verify each provider still emits valid
  tool calls with the clause present.
- Do NOT translate or weaken any existing prompt/pack file.
- The directive must not stamp `systemPromptTag` semantics — it is a
  composition-layer clause, not a system-prompt marker.

### Open Questions

- Should hub-authored chat (awaiting-user explanations, debate
  summaries) also localize? Recommended yes — same directive covers it;
  only runner-Go strings stay English.
- Per-run override (chat-level `/lang` toggle) or project-level only?
  Recommend project-level for v1.

### Source Refs

- `internal/runner/agent_catalog.go` (AgentDefinition.SystemPrompt)
- `internal/runner/interactive_service.go` (`composeAgentSpawnPrompt`,
  turn envelope, agentContextBlock pattern)
- `apps/local-runner/internal/agentpack/flow-pack/agents/*.md`
- `flowgate.LoadTestConfig` / `.flowpilot/settings/` file pattern

## 1. Goal

Every AI-generated natural-language chat surface responds in Vietnamese
when the project opts in, with zero impact on machine-format outputs
the runner parses or produces.

## 2. Parent Links

- coding plan: — (standalone enhancement; may spawn a CP if scoped up)
- tech design: —
- system spec: —
- specific upstream ids: —

## 3. Trigger

Operator request 2026-10-04: "Tôi muốn dùng Tiếng Việt cho mọi chat với
AI. Chúng ta thêm guide language hay cần chuyển mọi system prompt về
vietnamese?" — answer: directive injection, captured here for
implementation.

## 4. Exact Change

- `T-1` New settings file `.flowpilot/settings/chat-language.json`
  (`{"language": "vi"|"en"|…}`) + loader `LoadChatLanguage(dotFP) string`
  returning the resolved BCP-47-ish tag, defaulting `"en"` on missing/
  corrupt (fail-open to English — chat language is UX, never a gate).
- `T-2` `responseLanguageClause(lang string) string` — the fixed English
  directive clause for non-`en` languages, with the explicit
  keep-machine-format list (tool calls, enums, paths, ids).
- `T-3` Wire into `composeAgentSpawnPrompt`: clause appended after the
  agent system prompt, before the identity line, when resolved language
  is non-`en`. Requires the composer to receive the workspace dotFP (or
  resolved lang) — thread from the spawn call site, same pattern as
  other per-workspace inputs.
- `T-4` Root/chat turn path: apply the clause to the run's prompt
  envelope on user turns so the main chat + reinvokes respond in
  Vietnamese. Implementation detail TBD — either a durable one-shot
  prefix on first turn or a suffix appended per turn; must not corrupt
  `isSystemPrompt` classification or `systemPromptTag` replay logic.
- `T-5` Debate/owner legs and gate reprompts inherit the directive via
  their composed prompts; reprompt Go-text stays English (telemetry) —
  the leg's *reply* lands in Vietnamese.

## 5. Touched Areas

- files: `agent_catalog.go` / `interactive_service.go` (composer), new
  `chat_language.go`, `flow-pack` (no file edits — composition-time only)
- modules: `internal/runner`, `.flowpilot/settings/`
- routes: —
- tables: —

## 6. Code Guide Signatures

```go
// internal/runner/chat_language.go
func LoadChatLanguage(dotFP string) string                 // T-1; "en" default, never errors
func responseLanguageClause(lang string) string            // T-2; "" for "en"/""
func (s *InteractiveService) resolvedChatLanguage(workspaceCwd string) string // T-1; runner default → project file
```

```go
// internal/runner/interactive_service.go — mechanically touched
func composeAgentSpawnPrompt(agentDef *AgentDefinition, userPrompt string) string // T-3: gains lang clause; signature may grow a lang param — document deviation in §11
```

## 7. Test Signatures

- `TestChatLanguage_DefaultsEnglish` — missing file → clause empty,
  composed prompt unchanged byte-for-byte (backward compat)
- `TestChatLanguage_VietnameseClauseInSpawnPrompt` — settings `vi` →
  spawn prompt contains the clause after system prompt, before identity
  line; clause names the machine-format keep-list
- `TestChatLanguage_ClausePreservesToolContract` — composed prompt still
  contains `submit_review_outcome`, `approved|changes_requested|blocked`
  verbatim (no translation of contract literals)
- `TestChatLanguage_CorruptFileFallsBackEnglish` — malformed JSON →
  "en", no panic, no clause
- `TestChatLanguage_RootTurnEnvelope` — root user turn carries the
  clause; `isSystemPrompt`/`systemPromptTag` replay classification
  unchanged
- Provider parity: fake-adapter prompt capture per provider
  (Claude/Codex/Grok spawn prompts carry the clause identically —
  composition is provider-neutral by construction)

## 8. Acceptance Check

- With `chat-language.json: {"language":"vi"}` in a project, a live chat
  turn's assistant reply is Vietnamese; a vibe sprint's leg results
  (coder report, reviewer summary) are Vietnamese; debate owner verdict
  *prose* is Vietnamese while `verdict`/`status` fields stay exact.
- With the setting absent/`en`, every prompt is byte-identical to today.
- `go test ./internal/runner` — no regressions in prompt-composition
  tests (`composeAgentSpawnPrompt`, spawn/identity tests).

## 9. Out of Scope

- Translating flow-pack prompts/agents/skills to Vietnamese.
- i18n of runner-Go strings (gate messages, option cards, step labels,
  TUI/Desktop chrome) — separate feature.
- Per-run `/lang` toggles, auto-detection from user language.
- Translating requirements docs, CA entries, or commit messages —
  English remains the repo artifact language.

## 10. Definition of Done

- [ ] §6 signatures implemented (deviations documented in §11)
- [ ] §7 tests green, additive-only
- [ ] Existing prompt-composition tests still green
- [ ] Provider parity: clause identical on all three providers;
  structured-output contract literals intact in composed prompts
- [ ] `feature_key` set; CA ledger entry written
- [ ] §8 acceptance verified live (a real run replies in Vietnamese)
- [ ] GitNexus `detect_changes` shows only expected symbols

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
