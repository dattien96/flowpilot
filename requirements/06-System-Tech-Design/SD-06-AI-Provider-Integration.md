# FlowPilot Tech Design - AI Provider & Model Integration

This document translates `SS-05-Workflow-Ai-Provider` into technical implementation details.

## 1. UI/UX Design (Frontend - React)
- **Settings Screen:** 
  - UI to list installed providers (Gemini, Claude, Codex) and their models.
  - Global project defaults for Provider and Model.
- **Workflow / Step Override UI:** Dropdowns within the Workflow Builder to override the Provider/Model at the Workflow level or the individual Step level.

---

## 2. Provider Installation Commands

The Go-Runner's Cobra CLI will implement a sub-command: `flowpilot install-provider <provider_name>`.
Before installing, the runner uses `exec.LookPath` (Go) to auto-detect if the CLI is already present.

### 2.1 Claude Code (Anthropic)
- **Official docs:** https://code.claude.com/docs/en/overview
- **macOS / Linux / WSL:**
  ```bash
  curl -fsSL https://claude.ai/install.sh | bash
  ```
- **Windows (PowerShell):**
  ```powershell
  irm https://claude.ai/install.ps1 | iex
  ```
- **Windows (Command Prompt):**
  ```cmd
  curl -fsSL https://claude.ai/install.cmd -o install.cmd && install.cmd && del install.cmd
  ```
- **macOS (Homebrew alternative):**
  ```bash
  brew install --cask claude-code
  ```
- **Verification:**
  ```bash
  claude --version
  ```
- **Prerequisites:** Anthropic account with Claude Pro, Team, or Enterprise subscription (or API key via Anthropic Console).

### 2.2 Codex CLI (OpenAI)
- **All platforms (npm):**
  ```bash
  npm install -g @openai/codex
  ```
- **macOS (Homebrew alternative):**
  ```bash
  brew install codex
  ```
- **Prerequisites:** Node.js >= 18. After install, run `codex` or `codex auth` to authenticate with ChatGPT account or OpenAI API key.
- **Verification:**
  ```bash
  codex --version
  ```

### 2.3 Gemini CLI (Google)
- **All platforms (npm):**
  ```bash
  npm install -g @google/gemini-cli
  ```
- **No-install alternative:**
  ```bash
  npx @google/gemini-cli
  ```
- **Prerequisites:** Node.js >= 20. On first run, user is prompted to sign in with Google account.
- **Verification:**
  ```bash
  gemini --version
  ```

---

## 3. Provider Instruction Files

Each AI provider uses a different file to receive persistent project-level instructions:

| Provider | Instruction File | Location | Skills Location |
|----------|-----------------|----------|-----------------|
| Claude   | `CLAUDE.md`     | Project root | `.claude/skills/<name>/SKILL.md` |
| Codex    | `AGENTS.md`     | Project root | `.codex/skills/<name>/SKILL.md` (or via `config.toml`) |
| Gemini   | `GEMINI.md`     | Project root | `.gemini/skills/<name>/SKILL.md` |

**This is critical for the Workflow Engine** — FlowPilot must generate and write to the correct file depending on which provider is selected for a step.

---

## 4. Configuration Resolution Logic
Before making an LLM call, the Go-runner resolves the model configuration by checking:
1. Does the specific `workflow_run_step` have a provider/model override? (Highest priority)
2. Does the `workflow_run` have an override?
3. Fallback to `project` default provider/model. (Lowest priority)

---

## 5. Go-Runner Installation Flow
1. User selects a Provider in the React UI → writes to `project_settings.default_provider`.
2. Go-runner receives the config → runs `exec.LookPath("claude")` (or `codex`, `gemini`).
3. If NOT found → Go-runner executes the OS-appropriate install command (Section 2).
4. After install → runs `<provider> --version` to verify.
5. Updates `project_settings.provider_status` to `INSTALLED` or `FAILED`.
6. If auth is needed → opens browser for OAuth/login and waits for confirmation.
