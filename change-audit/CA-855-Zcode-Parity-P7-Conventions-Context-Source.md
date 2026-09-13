# CA-855 — CP-62 P-7: conventions context source (repo-as-config) via the SD-22 registry

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-343
change_type: task
summary: new deterministic conventions ContextSource (priority 0) — user ~/.flowpilot/conventions.md > workspace .flowpilot/conventions.md (AGENTS.md fallback), all-missing → empty section no error; body renders under a "## Context" heading so the Task-334 packer classifies it mandatory_doc (CP-23 R-1 Tier-1 non-droppable) with zero packer changes; flagship flows' contextProfiles opt in via the registered "conventions" id — flow YAML stays fork-free across projects
# --->8---

## Why

Project conventions (test framework, build commands, naming) lived scattered in prompt templates or had to be hand-copied into specs; the same flow YAML needed forking per project. ZCode's AGENTS.md pattern shows one static file shaping every agent deterministically.

## Change

- `runner/context_source_conventions.go` (new): `conventionsSource` implementing the 4-method `ContextSource` interface (Deterministic=true — registry-enforced); layered read (user > workspace conventions, AGENTS.md fallback), silent-skip on missing files.
- `runner/context_sources_builtin.go`: registered at priority 0 (packs before canonical.head).
- Flow YAMLs: `conventions` added to every contextProfile's candidateSources in task-harness, bug-plan-harness, vibe-sprint.
- Tier-1 enforcement reuses the packer's own mandatory_doc classification via the section heading — no promptpacker edits (composition over modification, CP-23 D-5 style).

## Tests

`runner/context_source_conventions_test.go` (5): user>workspace merge order, AGENTS.md fallback, all-missing → empty/no-error, Tier-1 non-droppable end-to-end (tiny budget: fillers drop, conventions survive — via real splitPromptIntoSections+PackPrompt), flow-YAML invariant (3 flagship flows reference conventions in every profile + full-pack validation passes). agentpack + runner targeted suites green.

## Providers

Case 1 provider-agnostic — static file read rendered into the assembled prompt; no adapter involvement.

## Prior claims intact

Task-191/192 registry contract untouched (registered through mustRegisterContextSource like every builtin); CA-853 profile resolution untouched (conventions is just another validated source id); golden fixtures for default context sets untouched — default-set injection (all flows incl. profile-less) is the documented follow-up, flagship flows opt in via profiles today.
