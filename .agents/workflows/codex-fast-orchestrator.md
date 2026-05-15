---
description: Lean Hybrid Codex/Gemini Workflow (Arch in Codex → Implementation in Gemini → Review in Codex)
---

# ⚡ Codex Fast Orchestrator (Lean)

## 📌 Load Context

Read these files in order:

1. **`.agents/skills/token-optimization/SKILL.md`** - Token optimization rules
2. **`.agents/skills/codex-fast-orchestrator/SKILL.md`** - Lean workflow definition

## 🎯 Execute

Follow the lean workflow defined in **`.agents/skills/codex-fast-orchestrator/SKILL.md`**:

- **Phase 1 (Codex)**: Architecture → `implementation_plan.md` (Root)
- **Phase 2 (Gemini)**: Coding → Implementation (+ Build/Test Verification)
- **Phase 3 (Codex)**: Review → `walkthrough.md` (Root)

**Start by announcing**:
> "Operating as the **Codex Fast Orchestrator** (Phase 1: Architecture via Codex) - Mode: **Lean Hybrid** | Status: **Delegating to Codex (GPT-5.4 / High Reasoning)**"