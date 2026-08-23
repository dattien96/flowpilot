# 4C Summary Example

## Scenario: Adding a New "Emotion Metric" Feature

### C1 - Context
- **Module**: `:features:presentation:emotion`
- **Layers**: Domain (UseCases), Data (Repository/Remote), Presentation (Compose UI).
- **Type**: New Feature.

### C2 - Command
- **Goal**: Implement a screen to track user emotions daily.
- **Scope**: Screen + ViewModel + Repository + Supabase integration.

### C3 - Constraints
- **Off-limit**: Do not touch `:core:auth` logic.
- **Breaking changes**: None.

### C4 - Criteria
- **Done**: 
  - [ ] Unit tests for UseCase and ViewModel.
  - [ ] UI matches Figma (Material 3).
  - [ ] Data persists to Supabase.
- **Review**: Architecture Agent + Lead Review.
