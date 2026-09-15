## Description

Briefly describe the intent of this change and what problems it solves.

Closes # (issue / Task ID)

---

## Type of Change

- [ ] `[Feature]` New capability or enhancement
- [ ] `[BugFix]` Bug or regression fix
- [ ] `[Refactor]` Code cleanup or structural improvement (no behavior change)
- [ ] `[Docs]` Documentation, specification, or learning playbook update
- [ ] `[Test]` Additional test coverage
- [ ] `[Hotfix]` Critical production fix

---

## Architectural & Quality Checklist

Please verify that your pull request complies with FlowPilot's engineering contracts:

- [ ] **The Oracle Rule**: I have **NOT** modified, deleted, or weakened any pre-existing unit tests to make my code pass. All existing tests pass cleanly.
- [ ] **Additive Testing (`r-additive-tests` / `r-newtest`)**: I have included new unit test(s) that verify this change and protect against future regressions.
- [ ] **Commit Message Contract**: All commits follow the machine-parseable format:
  `[Type][feature][layer] <description>` (e.g. `[Feature][chat-ui][ui] add model badge`).
- [ ] **Change Audit (`r-ca`)**: For functional changes, I have created or updated a corresponding audit note under `change-audit/CA-*.md`.
- [ ] **Node Isolation & Permissions**: If modifying agent behaviors, read-only postures (`silent-deny`) and approval gates have been respected.
- [ ] **Local Verification**:
  - [ ] Go tests: `cd apps/local-runner && go test -count=1 ./...`
  - [ ] Frontend: `npm run typecheck` or `npm run lint`

---

## Screenshots / Terminal Output (if applicable)

*Attach screenshots of UI changes, or paste terminal outputs from `flowpilot chat` / test runs.*
