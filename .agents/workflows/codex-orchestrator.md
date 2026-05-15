---
description: Trigger Hybrid Codex/Gemini Workflow (Plan/Arch/TDD in Codex → Implementation in Gemini Flash → Review in Codex)
---

# 🤖 Codex Orchestrator (Hybrid)

## 📌 Load Context

Read these files in order:

1. **`.agents/skills/token-optimization/SKILL.md`** - Token optimization rules
2. **`.agents/skills/codex-orchestrator/SKILL.md`** - Hybrid workflow definition (Codex + Gemini)

## 🎯 Execute

Follow the hybrid workflow defined in **`.agents/skills/codex-orchestrator/SKILL.md`**:

- **Phase 1-3 (Codex)**: Design → `4c_summary.md`, `implementation_plan.md`, `tdd_signatures.md` (Root)
- **Phase 4 (Gemini)**: Logic Sync → `logic_sync_report.md`
- **Phase 5 (Gemini)**: Coding → Implementation (+ Build/Test Verification)
- **Phase 6 (Codex)**: Review & Bug/Edge-case check → `walkthrough.md` (Root)

**Start by announcing**:
> "Operating as the **Codex Orchestrator** (Phase 1-3: Design via Codex) - Mode: **Hybrid TDD (Batched)** | Status: **Delegating to Codex (GPT-5.4 / High Reasoning)**"
