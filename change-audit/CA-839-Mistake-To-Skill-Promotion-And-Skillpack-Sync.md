# CA-839 — Task-336: mistake-to-skill promotion + skillpack sync (CP-23 Phase 3)

# ---8<--- flowpilot:change-ledger
feature_key: runtime-intelligence
source_doc_id: Task-336
change_type: feature
summary: add internal/skilllearn (drift-event aggregation into LessonCandidates with human approve/edit/reject gate, SKILL.md exporter with core-conflict guard and version-suffix dedupe, CompactRuleCard) + additive skillpack.InstallFromRoot
# --->8---

## Why

CP-23 Phase 3 / D-4: repeated drift incidents must crystallize into reusable knowledge, but never as auto-created skill files. Candidates form at ≥2 same-pattern drift events and stay `candidate` until a human approves; export targets the target project's `.agents/.claude/.grok/skills/<group>/<slug>/SKILL.md` or the platform core `flow-pack/<group>/` so future `skillpack.Install()` runs inherit the lesson.

## Change

- `internal/skilllearn/` (new, stdlib-only): `AggregateDriftEvents` — dedupe (RunID,TurnID) keep-first (CA-838 reviewer note, tested), pattern key = normalized TriggeredSignals, threshold clamped ≥2, deterministic double-call equality; `LessonCandidate` store JSON at `<workspace>/.flowpilot/workflow_lesson_candidates.json` (atomic tmp+rename; missing = empty; corrupt = error); `PromoterService` List/Update (edit-before-approval)/Approve/Reject — candidate→approved→promoted, export failure keeps `approved`, rejected/promoted terminal; `SkillExporter` — YAML frontmatter (name/description) + `# Title` + Core Rules/Anti-Pattern/Preferred Behavior/Example, `.agents/.claude/.grok` dirs, duplicate slug → `-v2/-v3` never overwrite, `DetectCoreSkillConflict` (protected slugs safe-fix-contract/additive-tests-only + forbidden directive phrases) → `ConflictError` until `Force` (file provably never written unforced), `CompactRuleCard` for the Task-334 budget packer.
- `skillpack/install.go` (+72/−0 purely additive): `InstallFromRoot(targetRepoDir, platform, flowPackRoot)` sharing `platformGroups`/`installRoots` with `Install` (embedded go:embed Install cannot see runtime flow-pack roots); never overwrites existing destination skills. GitNexus: Install unmodified; callers = 9 in-package tests → LOW.

## Tests

`skilllearn_test.go`: all 9 Task-336 §10 signatures exact-name + 9 additive (lifecycle, export-failure-keeps-approved, edit-before-approval, store robustness, T-1 upsert, dedupe, multi-provider dirs, compact card) + `TestSkillPromotion_EndToEnd_CandidateToSkill` — 18/18 PASS. `TestSkillExporter_InstalledSkillIsDiscoverable` exercises the REAL `InstallFromRoot` (byte-compares installed vs published SKILL.md). skillpack/driftdetect/promptpacker suites green; gofmt/vet clean; tests write only t.TempDir().

## Providers

Case 1 agnostic: deterministic Go, 0 LLM, no provider-specific path — identical for Claude/Codex/Grok by construction.

## Prior claims intact

CA-837 (CompactRuleCard output uses `## Core Rules`, matched by the packer's CompactSkillCard heading), CA-838 (gate-resume dedupe implemented + tested), CA-833..CA-836, CA-695/CA-442/CA-441. Review hardening notes (group slug validation, ProjectID in upsert key when multi-project stores appear, store locking when TUI wires in) recorded in Task-336 Completion Notes. CP-23 closed: doc moved to `07-Coding-Plan/done/` with all 6 DOD items checked; supersedes CP-24/25/26/39 links updated.
