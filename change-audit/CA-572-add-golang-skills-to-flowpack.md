# CA-572: Import 46 Golang agent skills into flow-pack/golang

## What

Imports 46 agent skills specialized for Golang from samber/cc-skills-golang into apps/local-runner/internal/skillpack/flow-pack/golang/:

- Cobra, Viper, CLI patterns, gopls, Go testing, testify, benchmark, linting, troubleshooting.
- Architecture patterns, project layout, data structures, structs/interfaces, concurrency, context, safety.
- Dependency injection (Wire, Dig, Fx), APIs (gRPC, GraphQL, Swagger), database, observability, slog.
- Samber ecosystem libraries (lo, mo, do, oops, hot, ro).
- Best practices (code-style, error-handling, performance, modernize, security, etc.).

## Why

Provides complete agentic skill coverage for Golang projects managed and initialized by FlowPilot local-runner.

# ---8<--- flowpilot:change-ledger
feature_key: skill-injection
change_type: feature
summary: add 46 golang agent skills from cc-skills-golang to flow-pack
# --->8---
