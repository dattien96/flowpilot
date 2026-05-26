# CP-12: Artifact Memory, RAG & Prompt Context

> **This plan has been merged into [CP-10-Integrations-Hardening.md](./CP-10-Integrations-Hardening.md).**
>
> Reason: CP-12 shares the same audit trail, RLS hardening pass, Google Drive integration, and Admin Web phase as CP-10. Keeping them separate created duplicate work and a sequencing dependency that was unnecessary.
>
> All content from this file — database migration, `step_context_slots` table, `generate-embedding` Edge Function, Context Resolver Go-Runner module, HyperRAG query construction, working memory generation, prompt assembly, testing, and Definition of Done — is now in **CP-10, Part B (§3 through §8)**.

## Quick Reference

| Topic | Location in CP-10 |
|---|---|
| Database migration (artifact_memories, workflow_prompt_context_items, step_context_slots) | §3.1 |
| generate-embedding Edge Function | §3.2 |
| Working memory generation | §3.3 |
| Context slot model + resolver types | §3.4 |
| Context Resolver Go-Runner module | §3.5 |
| HyperRAG query construction | §3.6 |
| Prompt assembly | §3.7 |
| Admin Web (artifact management + context slots panel + prompt context drawer) | §7 |
| Testing | §8 |
| Definition of Done | §9, Part B |
