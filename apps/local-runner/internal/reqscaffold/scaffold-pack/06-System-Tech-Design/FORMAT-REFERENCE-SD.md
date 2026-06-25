# Tech Design Format Reference

## Metadata

- Document ID: `SD-XX`
- Title: `<short topic title>`
- Phase: `tech_design`
- Status: `draft | reviewing | approved | superseded`
- Owner: `<name or team>`
- Reviewers: `<name or team>`
- Created: `YYYY-MM-DD`
- Last Updated: `YYYY-MM-DD`
- Parent Documents: `<linked SS docs>`
- Child Documents: `<linked CP docs when created>`
- Related Documents: `<optional>`
- Replaces: `<optional prior doc id>`
- Tags: `<feature, module, domain>`

## AI Quick View

### Summary

- `<3 to 5 bullets describing the technical approach>`

### Current Ask

- `<what technical decision this design needs to settle>`

### Key Decisions

- `D-1` `<design decision>`

### Constraints

- `<runtime, infra, security, compatibility, cost, or rollout constraints>`

### Open Questions

- `<unknowns>`

### Source Refs

- `<SS ids, AC ids, related SD or external references>`

## 1. Goal

Describe what the design must achieve technically.

## 2. Input Documents

- `<SS doc ids>`
- `<specific AC ids>`

## 3. Architecture Decision

- `D-1` `<decision>`
- Alternatives considered:
- Why this option was chosen:

## 4. Component Impact

- Impacted modules:
- New modules:
- Unchanged modules:

## 5. Data Model

- Entities:
- Fields:
- State transitions:

## 6. Interfaces and Contracts

- API contracts:
- DB contracts:
- file or artifact contracts:
- provider or MCP contracts:

## 7. Execution Flow

1. `<step>`
2. `<step>`
3. `<step>`

## 8. Failure and Edge Handling

- `F-1` `<failure case>`

## 9. Security and Operational Concerns

- auth:
- secrets:
- audit:
- rollback:

## 10. Risks and Trade-Offs

- `R-1` `<risk>`

## 11. Validation Strategy

- unit:
- integration:
- manual:
- observability:

## 12. Traceability to Spec

- `AC-1` -> `<design section or decision>`
- `AC-2` -> `<design section or decision>`
