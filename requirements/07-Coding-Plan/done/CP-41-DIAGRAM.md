# CP-41 Diagrams — RAG Harness Flow Mode

> Companion to [CP-41](./CP-41-RAG-Harness-Flow-Mode.md).
> CP-41 **adds no new agent-interaction code** — it only adds context-harness node behavior on top of CP-36's engine. Read [CP-36-DIAGRAM.md](./CP-36-DIAGRAM.md) first.

---

## Diagram 1 — How CP-41 Sits on CP-36

CP-41 is a `FlowDefinition` over the CP-36 executor. The Plan→Code→Test→Audit pipeline is just four nodes with edges — the same engine that runs the review loop runs this.

```mermaid
flowchart TB
    subgraph "CP-36 Engine (substrate — unchanged)"
        ENG["Bounded Executor
        spawnNode · joinSatisfied · route
        applyFlowControl · extendCap
        sessions.ndjson persistence
        auto-reinvoke · single-flight"]
    end

    subgraph "CP-41 (adds node behavior only)"
        FD["FlowDefinition: Plan→Code→Test→Audit
        ─────────────────────────────────
        4 FlowNodes with context-harness instructions
        Testing back-edge = CP-36 back-edge (cap: 3)
        No new coordinator Go code"]
    end

    subgraph "What CP-41 adds"
        T168["Task-168
        FlowContextPackage contract
        deterministic retrieval from feature_key"]
        T169["Task-169
        Plan→Coding prompt handoff
        [FlowPilot flow context package] sentinel"]
        T170["Task-170
        Testing validation runner
        bounded failure summary → retry prompt"]
        T171["Task-171
        Audit draft generation
        CA note + commit message suggestion"]
    end

    ENG --> FD
    FD --> T168
    FD --> T169
    FD --> T170
    FD --> T171
```

---

## Diagram 2 — Pipeline Overview (Plan → Code → Test → Audit)

What data flows between steps and what each step owns.

```mermaid
flowchart TD
    subgraph "Step 1 — Plan (Task-168 + Task-169)"
        P1["Resolve feature_key via Feature Catalog"]
        P2["Retrieve: CA excerpts + commits + chat summaries + source files"]
        P3["Build FlowContextPackage (bounded, typed)"]
        P4["Emit EventFlowContextPackage"]
        P1 --> P2 --> P3 --> P4
    end

    subgraph "Step 2 — Coding (Task-169)"
        C1["Receive FlowContextPackage\nvia [FlowPilot flow context package] prompt section"]
        C2["Implement the change\ngrounded in feature history + CA guardrails"]
        C3["Return code changes to workspace"]
        C1 --> C2 --> C3
    end

    subgraph "Step 3 — Testing (Task-170)"
        T1["Run configured validation command\ngo test ./... · npm run test · etc."]
        T2{{"exitCode == 0?"}}
        T3["Summarize key failure lines\nbounded block · NOT raw full log\nEmit EventFlowValidationRetry (attempt N/3)"]
        T4["Emit EventFlowValidationResult (passed)"]
        T1 --> T2
        T2 -->|"fail"| T3
        T2 -->|"pass"| T4
    end

    subgraph "Step 4 — Audit (Task-171)"
        A1["Generate EventFlowAuditDraft\nfeatureKey + sourceDocId + whatChanged + whyChanged\nchangeLedgerBlock + commitMessage suggestion"]
        A2{"feature_key in\nFEATURE-KEYS.md?"}
        A3["status: ready\nCA note + commit prepped\n(no file written yet)"]
        A4["status: blocked_missing_feature_key\ncommitMessage: empty"]
        A1 --> A2
        A2 -->|"yes"| A3
        A2 -->|"no"| A4
    end

    P4 -->|"FlowContextPackage\nforward · when: done"| C1
    C3 -->|"forward · when: done"| T1
    T3 -->|"🔁 back · when: continue · cap: 3\nCP-36 back-edge\nretry prompt = original pkg + failure block"| C1
    T4 -->|"forward · when: done"| A1
    T3 -->|"forward · when: escalate\n(cap exhausted)"| ESC(["⚠ ask_user"])
```

---

## Diagram 3 — Context Package Assembly (Task-168)

How the Plan step builds the `FlowContextPackage`. Retrieval is **deterministic only** — no vector DB, no embedding index, no similarity search.

```mermaid
flowchart TD
    ISSUE["User ask / Issue description\ne.g. 'Fix BUG-152 — gate fires on child turns'"]

    FK["Feature Resolver\nfeature_catalog/ + FEATURE-KEYS.md\n─────────────────────────────\nOutput: feature_key + confidence\nverified (≥ 5.0) · inferred · unresolved"]

    subgraph "4 Deterministic Sources"
        CA["① CA excerpts\nchange-audit/CA-NNN.md\nfiltered by feature_key\nWhat changed + Why"]
        HI["② Ordered commit history\n.flowpilot/ledger/feature_history.ndjson\nHow & When — newest first"]
        CH["③ Chat summaries\n.flowpilot/ledger/chat_summary.ndjson\nPer-feature long-term memory\nDecisions + rationale from past sessions"]
        SR["④ Source excerpts\nExplicit reads only:\n· Stack trace file paths\n· Feature catalog globs\n· User-provided paths\n· Upstream doc refs"]
    end

    subgraph "FlowContextPackage"
        PKG["packageId (hash)\nworkflow_run_id + plan step_run_id\nfeatureKey + featureConfidence\n─────────────────────────────────\nhistoryBlock  (CA + commits)\ndiscussionBlock  (chat summaries)\nsourceExcerpts\nconstraints\nwarnings  (if confidence < verified)"]
    end

    CODING["Coding agent prompt\n─────────────────────────────────\n[FlowPilot flow context package]\n## Newest prior truth: CA-147 ...\n## Prior work (last 5): ...\n## Prior discussion: ...\n## Source context: ...\n## Constraints: ..."]

    ISSUE --> FK
    FK --> CA & HI & CH & SR
    CA & HI & CH & SR --> PKG
    PKG -->|"stable across retries\n(same packageId until Plan reruns)"| CODING
```

---

## Diagram 4 — Testing Retry = CP-36 Back-Edge (Task-170)

The Testing→Coding retry is **not** a new state machine. It reuses the CP-36 bounded back-edge directly. The only new code is the validation runner and the failure summary builder.

```mermaid
sequenceDiagram
    participant R as Runner (executor)
    participant T as Testing step (inline)
    participant C as Coding agent
    participant U as User

    Note over R: Testing node: run: inline, lifecycle: once

    R->>T: run validation command (go test ./...)
    T-->>R: exitCode=1, raw output (may be large)

    Note over R: Summarize failure (Task-170)
    Note over R: key failure lines only · Truncated=true if >50 lines
    Note over R: EventFlowValidationRetry {retryAttempt: 1, status: retrying}

    R->>R: applyFlowControl(continue) → back-edge fires (CP-36)
    Note over R: round: 1→2 · same FlowContextPackage (pkg unchanged)

    R->>C: Coding turn · round 2
    Note over C: Prompt =
    Note over C:   [FlowPilot flow context package] (original)
    Note over C:   ## Validation Failure — Retry 1/3
    Note over C:   Command: go test ./...  · Exit: 1
    Note over C:   Key failure lines: (bounded)

    C-->>R: code changes applied
    R->>T: run validation command again
    T-->>R: exitCode=0
    Note over R: EventFlowValidationResult {passed}
    Note over R: applyFlowControl(done) → forward to Audit

    alt cap exhausted (3 failures)
        R->>R: route: escalate
        R->>U: ask_user [Accept as-is / Stop]
    end
```

> **Environment errors:** If the command binary is not found (`exec: not found in $PATH`), the runner emits `status: skipped_env_error` and does **not** trigger a Coding retry. Environment failures are reported, not retried.

---

## Diagram 5 — Audit Draft Gate (Task-171)

The Audit step prepares output but writes **nothing** automatically. Every write is behind an explicit user confirmation.

```mermaid
flowchart TD
    PASS["Testing passed\napplyFlowControl(done) → Audit node"]

    GEN["Generate EventFlowAuditDraft\n─────────────────────────────────
    featureKey: agent-flow-engine
    sourceDocId: BUG-152
    whatChanged: added rs.parentRunID == '' guard at :2182
    whyChanged: child runs change code but never write CA notes
    changedFiles: [interactive_service.go]
    validationResult: passed
    changeLedgerBlock: # ---8<--- flowpilot:change-ledger ...
    commitMessage: [BugFix][agent-flow-engine] exempt child runs BUG-152"]

    CHECK{"feature_key in\nFEATURE-KEYS.md?"}

    READY["status: ready
    Draft visible in board
    ─────────────────
    No file written yet
    No commit made yet"]

    BLOCKED["status: blocked_missing_feature_key
    commitMessage: ''
    changeLedgerBlock: ''
    ─────────────────────────────────────────
    User must register key in FEATURE-KEYS.md
    then rerun Audit step"]

    CONFIRM["User confirms in board
    ────────────────────
    ① Runner writes change-audit/CA-NNN.md
       (with ledger block)
    ② Stages: source files + CA file
    ③ Commits: [BugFix][feature-key] ... BUG-NNN
    ④ CA + code ship in ONE commit"]

    PASS --> GEN --> CHECK
    CHECK -->|"yes"| READY
    CHECK -->|"no"| BLOCKED
    READY -->|"user clicks Commit"| CONFIRM
```

---

## Diagram 6 — Full Data Lineage (CP-36 + CP-41 Together)

How a single FlowPilot run flows data from user ask to committed audit note.

```mermaid
flowchart TD
    USER["User: 'Fix BUG-152 — gate fires on child turns'"]

    subgraph "CP-41 — Context Harness"
        PLAN["Plan step\nresolves: feature_key=agent-flow-engine\nretrieves: CA-147..CA-143, commits, chat-summaries\nassembles: FlowContextPackage pkg-001"]
    end

    subgraph "CP-36 — Flow Engine (executor)"
        SPAWN1["spawnNode(coding, delegate, reinvoke)"]
        CODE1["Coding agent\nreads pkg-001\napplies rs.parentRunID=='' guard\ncommits: abc123"]
        SPAWN2["spawnNode(testing, inline)"]
        TEST["Testing step\ngo test ./... → exitCode=0\nEventFlowValidationResult(passed)"]
        AFC["applyFlowControl(done)\nroute: forward to audit"]
    end

    subgraph "CP-41 — Audit"
        AUDIT["Audit step\nEventFlowAuditDraft\nCA note content + commit message"]
        WRITE["User confirms\nCA-148.md written\ngit commit [BugFix][agent-flow-engine] BUG-152"]
    end

    USER --> PLAN
    PLAN -->|"FlowContextPackage"| SPAWN1
    SPAWN1 --> CODE1
    CODE1 --> SPAWN2
    SPAWN2 --> TEST
    TEST --> AFC
    AFC --> AUDIT
    AUDIT --> WRITE

    PERSIST["sessions.ndjson\nautoOrchestrate, round, cap\nflowCohortId, activeNode\npkg reference"]
    CODE1 -.->|"state persisted"| PERSIST
    TEST -.->|"state persisted"| PERSIST
    PERSIST -.->|"restored on runner restart\ncross-PC via Drive sync"| SPAWN1
```
