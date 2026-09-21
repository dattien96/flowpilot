# CA-891 — CP-68 Skill-Anchored Project Scaffolding & AI-Guided Init Engine

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: docs
summary: Define CP-68 plan, 4 tasks (Task-383..386), blueprint skills, and scaffold recipe for autonomous project initialization
# --->8---

## Why

Upgrading FlowPilot's project initialization from static skillpack file copying (CP-34) to an autonomous, skill-anchored scaffolding engine. Enables FlowPilot to automatically bootstrap production-ready Monorepos and core architectures across tech stacks, with compiler-verified accuracy and adaptive UI gating.

## Change

1. **Blueprint Skills & Recipe Manifest (`apps/local-runner/internal/skillpack/flow-pack/react-native/`)**:
   - `react-native-scaffold-bootstrap/SKILL.md`: Step 0 monorepo bootstrap guide (Turborepo + pnpm + Expo SDK 51).
   - `react-native-mobile-plumbing/SKILL.md`: Implementation specs for 9 core mobile pillars.
   - `react-native-core-ui-tokens/SKILL.md`: Design tokens, NativeWind v4, and trade/utility components.
   - `scaffold.yaml`: Platform scaffold recipe defining `scaffold_skills`, `verification_gate`, and `enabled: true`.

2. **Planning & Task Breakdown (`requirements/`)**:
   - `requirements/07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md`: Full architectural plan.
   - `requirements/08-Task/todo/Task-383-Platform-Scaffold-Recipe-Discovery-And-Skill-Integrity-Validator.md`: Slice P-1.
   - `requirements/08-Task/todo/Task-384-Runner-Scaffold-Dispatcher-And-AI-Turn-Orchestration.md`: Slice P-2.
   - `requirements/08-Task/todo/Task-385-TUI-Subcommand-And-Desktop-UI-Adaptive-Scaffold-Trigger.md`: Slice P-3.
   - `requirements/08-Task/todo/Task-386-Compiler-Verification-Gate-And-Self-Healing-Loop.md`: Slice P-4.

3. **Registry (`change-audit/FEATURE-KEYS.md`)**:
   - Registered `skill-anchored-init` key.

## Verification

- Ran `go test -v ./internal/skillpack/...` -> PASS (13/13 tests pass in 0.88s).
- All documents audited against `SS-13`, `FORMAT-REFERENCE-CP.md`, and `FORMAT-REFERENCE-TASK.md`.
