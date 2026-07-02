# CP-36 Diagrams — Generic Agent-Flow Engine

> Companion to [CP-36](./CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md).
> Read these diagrams top-to-bottom — each one zooms in on a different layer of the same system.

---

## Diagram 1 — Task Build Order (Layer Stack)

Each task is independently testable and stacks on the one below it.
Tasks 089 / 090 / 085 can land in parallel; everything else has a strict dependency.

```mermaid
flowchart TD
    T089["**Task-089** · Engine Vocabulary
    FlowNode · FlowEdge · FlowPolicy
    flow_control signal · declared-face map
    ─────────────────────────────
    agent_orchestrator.go (new section)"]

    T090["**Task-090** · Bounded Executor
    spawnNode · joinSatisfied · route
    applyFlowControl · extendCap
    SubmitFlowControl bridge + 2 HTTP routes
    ─────────────────────────────
    interactive_service.go + provider_registry.go"]

    T085["**Task-085** · Unified Persistence
    sessions.ndjson is sole run sink (chat + flow)
    Restore flow fields on resume
    Drive sync carries extended manifest
    ─────────────────────────────
    local_file_session_store.go + interactive_resume.go
    (can land in parallel with 090)"]

    T091["**Task-091** · Review Template
    submit_review_outcome declared face
    legacy keyword gate OFF in explicit mode
    ─────────────────────────────
    claude_mcp_server.go + codex_adapter.go"]

    T092["**Task-092** · Cohort Join Note
    flowCohortId on child runs
    buildCohortNote → one consolidated note
    appendPendingAgentContextLocked
    ─────────────────────────────
    interactive_service.go:561–588"]

    T093["**Task-093** · Auto-Reinvocation
    maybeAutoReinvokeHub after join
    single-flight + cap guard + Stop-cancellable
    ─────────────────────────────
    interactive_service.go (autoOrchestrate flag)"]

    T094["**Task-094** · Skill + Synthesizer Builtin
    agent-review-loop SKILL.md
    synthesizer builtin agent (dedup → call control tool)
    ─────────────────────────────
    agent_catalog.go + .claude/skills/"]

    T095["**Task-095** · Board + Contract + Client
    OrchestrationBoard.tsx — N children, round/cap, blocked
    contract.ts · store.ts · HttpWsRunnerClient.ts
    ─────────────────────────────
    agent_graph_updated SSE → mergeAgentRunsById"]

    T089 --> T090
    T089 --> T091
    T090 --> T091
    T090 --> T092
    T085 --> T093
    T091 --> T093
    T092 --> T093
    T093 --> T094
    T090 --> T095
    T089 --> T095
```

---

## Diagram 2 — Engine Core Types (Task-089)

The vocabulary introduced by Task-089. Everything in the executor (`route`, `applyFlowControl`, `joinSatisfied`) operates on these types — no role-specific strings anywhere in Go.

```mermaid
classDiagram
    class FlowNode {
        +ID       string
        +Agent    string
        +Run      inline | delegate
        +Lifecycle once | reinvoke
        +Join     all | any | quorum(n)
    }

    class FlowEdge {
        +From   string
        +To     string
        +When   continue | done | escalate
        +Kind   forward | back
    }

    class FlowPolicy {
        +Cap       int     %% default 3
        +OnCap     escalate | stop
        +ExtendBy  int     %% default 2
        +ExtendMax int     %% default 2 (ceiling = Cap + ExtendBy×ExtendMax)
    }

    class FlowControlInput {
        +Status   continue | done | escalate
        +Summary  string
        +Payload  map[string]any
    }

    class DeclaredFaceRegistry {
        +submit_review_outcome → flow_control
        approved           → done
        changes_requested  → continue
        blocked            → escalate
    }

    FlowNode    "1" --> "*" FlowEdge    : connects via
    FlowEdge    "*" --> "1" FlowPolicy  : bounded by
    DeclaredFaceRegistry --> FlowControlInput : maps onto
```

> **Code anchor:** `agent_orchestrator.go` — `FlowNode`, `FlowEdge`, `FlowPolicy`, `FlowControlInput`, face registry map.

---

## Diagram 3 — Review Loop as a FlowDefinition (Task-091)

The review-until-clean loop is not hardcoded in Go. It is **data** — a `FlowDefinition` of nodes + edges + policy. Any topology that fits the same schema runs on the same executor.

```mermaid
flowchart LR
    START(("▶")) --> coder

    coder["**coder**
    run: delegate
    lifecycle: reinvoke
    agent: coder.md"]

    revA["**reviewer-A**
    run: delegate
    lifecycle: reinvoke
    dependsOn: coder"]

    revB["**reviewer-B**
    run: delegate
    lifecycle: reinvoke
    dependsOn: coder"]

    synth["**synthesis**
    run: inline
    join: all
    (hub does this turn)"]

    coder  -->|"forward · when: done"| revA
    coder  -->|"forward · when: done"| revB
    revA   -->|"join: all"| synth
    revB   -->|"join: all"| synth
    synth  -->|"🔁 back · when: continue · cap: 3"| coder
    synth  -->|"forward · when: done"| DONE(("✓ done"))
    synth  -->|"forward · when: escalate"| ASK(("⚠ ask_user"))
```

> **Key point:** Replace coder/reviewer labels with alpha/beta — the engine behaves identically. The words "coder" and "reviewer" are agent definition filenames, not Go branches.

---

## Diagram 4 — Runtime Execution Trace (Full Review Round)

What actually runs inside the runner for Task-903 (add Modulo to calc.go).

```mermaid
sequenceDiagram
    actor U as User
    participant H as Hub (main agent)
    participant R as Runner (interactive_service.go)
    participant C as Coder
    participant RA as Reviewer-A
    participant RB as Reviewer-B

    U->>H: "Implement Task-903: add Modulo(a,b int) int"

    H->>R: spawn_agent(coder, delegate, reinvoke)
    R->>C: start turn · round 1
    C-->>R: EventTurnCompleted · finalMsg: "Implemented Modulo, commit abc123"
    R-->>H: pendingAgentContext: coder result

    H->>R: spawn_agent(reviewer-A, cohortId:C1, dependsOn:coder)
    H->>R: spawn_agent(reviewer-B, cohortId:C1, dependsOn:coder)
    H->>R: end turn  (autoOrchestrate: true)

    par reviewers run in parallel
        R->>RA: start turn
    and
        R->>RB: start turn
    end

    RA-->>R: EventTurnCompleted · finalMsg: "CHANGES REQUESTED\n- missing b==0 test"
    Note over R: appendCohortResult(reviewer-A, cohortId:C1)

    RB-->>R: EventTurnCompleted · finalMsg: "CHANGES REQUESTED\n- CA note wrong feature_key"
    Note over R: appendCohortResult(reviewer-B, cohortId:C1)
    Note over R: joinSatisfied(all) ✓

    Note over R: buildCohortNote → ONE consolidated note
    Note over R: appendPendingAgentContextLocked(hub)
    Note over R: maybeAutoReinvokeHub() → scheduleChildTurn

    R->>H: [system-note] join note (both reviewers' text)
    R->>H: [user-turn] "[flow-engine] Agent results ready. Synthesize..."

    H->>R: submit_review_outcome("changes_requested", issues=[...])
    Note over R: applyFlowControl(continue)
    Note over R: route: back-edge fires · round 1 → 2

    R->>C: start turn · round 2 · reinvoke with merged feedback
    C-->>R: EventTurnCompleted · "Fixed issues, commit def456"

    par round 2 reviewers
        R->>RA: start turn
    and
        R->>RB: start turn
    end

    RA-->>R: EventTurnCompleted · "APPROVED"
    RB-->>R: EventTurnCompleted · "APPROVED"
    Note over R: joinSatisfied(all) ✓ · buildCohortNote → hub

    R->>H: [system-note] join note: both APPROVED
    H->>R: submit_review_outcome("approved")
    Note over R: applyFlowControl(done) · status: done
    R-->>U: loop complete · round: 2 · issues: 0
```

---

## Diagram 5 — Cohort Join Note Chain (Task-092 + Task-093)

Zoom in on what happens after the last reviewer's `EventTurnCompleted` fires, through to the hub waking up.

```mermaid
flowchart TD
    A["Reviewer EventTurnCompleted\ninteractive_service.go:1242\nev.FinalMessage extracted as finalMsg"]

    B["appendCohortResult()\ncohortEntry{\n  Label: rs.label\n  Provider: rs.providerKey\n  FinalMessage: truncateDisplayField(finalMsg, 1500)\n  Status: completed | failed\n}"]

    C{{"joinSatisfied(all)?\n= all cohortIds completed"}}

    D["buildCohortNote()\ninteractive_service.go:561\n─────────────────────────────────\n[FlowPilot flow round N — M results joined]\n'reviewer-A' (claude): <finalMessage>\n'reviewer-B' (claude): <finalMessage>\n---\nSynthesize: dedup findings, call control tool"]

    E["appendPendingAgentContextLocked(parentRunID, note)\nstored on hub's interactiveRun\npersisted in sessions.ndjson"]

    F["maybeAutoReinvokeHub()\ninteractive_service.go\n─────────────────────────────────\nGuards checked:\n✓ autoOrchestrate == true\n✓ status ∉ {paused, stopped, blocked, done}\n✓ !turnInFlight\n✓ !reinvokeInFlight  ← single-flight\n✓ round < cap"]

    G["scheduleChildTurn(hub)\nsystem-note path (not a user bubble)\n─────────────────────────────────\nPayload delivered to hub:\n[system-note] join note\n[user-turn] autoReinvokePrompt"]

    H["Hub wakes up\nreads join note inline\ncalls submit_review_outcome(...)"]

    A --> B
    B --> C
    C -->|"no — wait"| WAIT(["...waiting for remaining reviewers"])
    C -->|"yes — all done"| D
    D --> E
    E --> F
    F -->|"guards pass"| G
    G --> H
    F -->|"any guard fails"| DROP(["reinvoke suppressed"])
```

---

## Diagram 6 — Flow State Machine (AgentLoopState.Status)

The states stored in `sessions.ndjson` and surfaced on the Orchestration Board.

```mermaid
stateDiagram-v2
    [*]           --> idle

    idle          --> running        : hub turn starts / agent spawned

    running       --> waiting_join   : hub ends turn, cohort in flight
    running       --> done           : applyFlowControl(done)
    running       --> blocked        : back-edge fires AND round ≥ cap
    running       --> stopped        : user clicks Stop

    waiting_join  --> synthesizing   : joinSatisfied(all) + autoReinvoke fires
    waiting_join  --> stopped        : user clicks Stop

    synthesizing  --> running        : submit_review_outcome → applyFlowControl → route

    blocked       --> running        : extendCap → cap raised, back-edge unblocks
    blocked       --> stopped        : user clicks Stop

    done          --> [*]
    stopped       --> [*]
```

> **Board mapping:** `blocked` = renders Extend +2 / Stop buttons. `waiting_join` = spinner on all child nodes. `done` = all nodes green, no further reinvoke.

---

## Diagram 7 — Persistence Model (Task-085)

Where run data lives and how it travels across machines.

```mermaid
flowchart LR
    subgraph "Local Machine A"
        R["Runner\ninteractive_service.go"]
        NDJSON["sessions.ndjson\n.flowpilot/sessions/\n─────────────────────────\nrunId, mode, round, cap\nactiveNode, autoOrchestrate\nflowCohortId, cohortResults\npendingAgentContext"]
        R -->|"write on every state change"| NDJSON
        R -->|"restore on resume"| NDJSON
    end

    subgraph "Drive Sync"
        DS["chat_session_sync.go\ncarries full sessions.ndjson manifest"]
    end

    subgraph "Local Machine B"
        NDJSON2["sessions.ndjson\n(synced copy)"]
        R2["Runner B\nreads via HTTP gateway\nnot Supabase"]
        NDJSON2 --> R2
    end

    subgraph "Supabase (definitions only)"
        SB["workflow_runs\nworkflow_run_steps\n─────────────────────────\nFlow / Step DEFINITIONS\n(not run data)"]
    end

    NDJSON --> DS
    DS --> NDJSON2
    R -.->|"definition reads only"| SB

    style SB stroke-dasharray: 5 5
```

> **Rule:** Production never writes `workflow_run_logs` / `workflow_run_sessions` / `workflow_run_steps` for run data. `cli/root.go:120-123` wires `localFileSessionStore` as the sole run sink.
