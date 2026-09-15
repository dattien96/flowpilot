# Contributing to FlowPilot

Thank you for your interest in contributing to **FlowPilot**! We are building an enterprise-grade AI-assisted engineering harness and workflow platform.

This document guides you through our development setup, testing standards, pull request workflow, and engineering principles.

---

## 🏛️ Core Principles for Contributors

Before writing code, please understand FlowPilot's non-negotiable engineering principles:

1. **The Oracle Rule**:
   Pre-existing unit tests are considered **Ground Truth (The Oracle)**.  
   - **Never modify or weaken assertions in existing unit tests** just to make your new code pass.
   - All bug fixes and features must pass all existing tests while adding new, additive tests to cover new behavior.
   - Any PR that tampers with existing baseline tests without explicit prior discussion will be blocked by our automated `r-additive-tests` gate.

2. **Schema-First & Strong Typing**:
   Avoid free-form string parsing. All communications between the Runner, Agents, and Database must adhere strictly to structured schemas (JSON-RPC 2.0 for MCP, Go structs, and TypeScript types).

3. **Node Isolation & Least Privilege**:
   Evaluating nodes (Reviewers, Scouts, Auditors) operate in read-only / verdict-only posture. Destructive actions must pass through explicit Approval Gates.

---

## 💻 Local Development Setup

### Prerequisites
- **Go**: Version `1.22` or later
- **Node.js**: Version `20` or later (`npm` / `pnpm`)
- **Just**: Command runner ([installation guide](https://github.com/casey/just))
- **Git**: Configured on your system

### 1. Fork and Clone
```bash
git clone https://github.com/<your-username>/flowpilot.git
cd flowpilot
```

### 2. Configure Environment
```bash
cp .env.example .env
```
Ensure your target AI provider CLI tools (`claude`, `codex`, `gemini`) are installed and authenticated in your terminal if you plan to test live agent execution.

### 3. Install Dependencies
You can install dependencies for all modules using `just`:
```bash
just runner-install    # Installs Go module dependencies
just web-install       # Installs Admin Web npm dependencies
just desktop-install   # Installs Desktop App npm dependencies
```

### 4. Running the Development Stack
```bash
# Start all three components together (Admin Web + Local Runner + Desktop App):
just dev

# Or start the lightweight terminal client against any codebase:
just chat-dev /path/to/your/project
```

---

## 🧪 Testing Standards

We enforce high test coverage and strict regression prevention.

### Running Go Tests (Runner, FlowGate, MCP)
```bash
cd apps/local-runner

# Run all unit tests
go test -count=1 ./...

# Run tests with race condition detector
go test -race -count=1 ./...

# Test specific packages
go test -v ./internal/runner/...
go test -v ./internal/flowgate/...
go test -v ./internal/tui/...
```

### Running Frontend Tests (Web & Desktop)
```bash
# Admin Web
cd apps/admin-web
npm run lint
npm run build

# Desktop App
cd apps/desktop-flowpilot
npm run typecheck
```

---

## 📜 Commit Message Convention

FlowPilot enforces a machine-parseable commit contract parsed by our Change Ledger engine (`internal/changeledger`). Every commit must follow this syntax:

```text
[Type][feature][layer] <description>
```

- **`[Type]`** (Mandatory): One of `Feature` | `BugFix` | `Refactor` | `Docs` | `Hotfix` | `Test`
- **`[feature]`** (Mandatory): The kebab-case key from `change-audit/FEATURE-KEYS.md` (e.g. `zcode-parity`, `chat-ui`, `flowgate`, `mcp-tools`, `runtime-intelligence`). If your feature is new, register it in `change-audit/FEATURE-KEYS.md` in the same commit.
- **`[layer]`** (Optional): Architectural layer touched: `ui` | `api` | `domain` | `data` | `infra` | `test` | `build` | `docs`.
- **`<description>`**: Imperative English, concise, no trailing period. Include tracking ID (`Task-NNN`, `BUG-NNN`, `CP-NN`) if applicable. Keep line 1 under 72 characters.

### Examples:
```text
[Feature][mcp-tools][infra] add google drive write approval interceptor Task-120
[BugFix][chat-ui][ui] fix message streaming scroll jump BUG-045
[Docs][zcode-parity] update interview playbook with MCP case study
[Refactor][runner] extract CAS state machine transitions CP-62
```

> **Note**: Do not include `Co-authored-by` or AI generation attribution lines in commit messages.

---

## 🚀 Pull Request Checklist

When submitting a Pull Request:

1. **Branch Naming**: Use `feat/<name>`, `fix/<name>`, `refactor/<name>`, or `task/<name>`.
2. **Oracle Rule**: Verify no existing baseline tests were modified (`git diff **/test*`).
3. **Commit Format**: Verify all commits follow `[Type][feature][layer] <description>`.
4. **Change Audit Note**: For significant changes, create or update a change note under `change-audit/CA-*.md`.
5. **CI Green**: Ensure all GitHub Actions checks pass on your PR branch.

---

## 🤝 Community & Support

- Open a [GitHub Discussion](https://github.com/dattien96/flowpilot/discussions) for architectural discussions or questions.
- Report bugs via [GitHub Issues](https://github.com/dattien96/flowpilot/issues).
- Review our [Code of Conduct](./CODE_OF_CONDUCT.md).
