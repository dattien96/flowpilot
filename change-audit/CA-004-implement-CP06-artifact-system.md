# CA-004 Implement CP-06 Artifact System

## Scope

This audit records the artifact-system implementation pass for the CP-06 workstream.

It covers the full local-first artifact model and the supporting UI, domain, persistence, runtime, and docs changes required to make it real.

The implementation spans:

- artifact definition catalog management
- step input and output artifact bindings
- runtime artifact-run records
- project artifact browsing by workflow run
- runner-side local-first artifact resolution and sync metadata
- workflow step editor support for artifact bindings
- workflow-step catalog summaries and workflow reference notes
- Supabase migrations for schema, seeds, and binding updates
- demo-mode gateway support so non-Supabase mode stays aligned
- route splits and route-tree updates for the artifact pages
- CP-06, SS-04, SS-07, SD-05, and SD-08 documentation alignment

## Completed

- [x] Updated the CP-06 documentation to describe the local-first artifact model. Summary: replaced the remote-storage-first framing with a definition layer, runtime artifact layer, and local-path-first execution flow.
- [x] Updated the artifact system spec and technical design docs. Summary: aligned SS-04, SS-07, SD-05, and SD-08 with reusable artifact definitions, runtime artifact runs, and multi-input/multi-output bindings.
- [x] Added the artifact definition domain model. Summary: introduced reusable catalog entities for artifact key, name, description, local path template, remote path template, and default file name.
- [x] Added the artifact run domain model. Summary: introduced runtime artifact records with workflow, workflow run, step-run, local path, remote path, remote URL, and sync status.
- [x] Added workflow-step artifact binding fields. Summary: extended step definitions to carry input and output artifact definition key arrays.
- [x] Added workflow-engine gateway support for artifact definitions and artifact runs. Summary: expanded the gateway contract so UI and runtime code can list and save the catalog and read runtime artifact history.
- [x] Added the Supabase workflow-engine mapper updates. Summary: mapped artifact definitions, artifact runs, and step binding tables into the domain model.
- [x] Added the Supabase workflow-engine repository support for artifact data. Summary: wired list/save behavior for artifact definitions, artifact runs, and step binding persistence through the database.
- [x] Added the in-memory demo gateway support for artifact data. Summary: kept demo mode aligned with the real gateway by seeding artifact definitions and built-in step bindings.
- [x] Added the artifact-definition selector UI. Summary: introduced a reusable multi-select chip selector for step input and output artifact bindings.
- [x] Added step create/edit support for artifact bindings. Summary: workflow step pages now let users bind multiple input and output artifact definitions instead of free-text artifact strings.
- [x] Added artifact summary information to the workflow-step catalog page. Summary: each step card now shows the resolved input and output artifact definitions for quick scanning.
- [x] Added the workflow reference note above the step catalog. Summary: surfaced the intended main flow as a plain text reference for Idea -> Business -> Architecture -> Tech Spec -> Code Plan -> TDD -> Coding Implementation Checklist -> Review.
- [x] Added the artifact definition catalog page. Summary: moved definition editing to a dedicated artifact-management page with storage configuration and a definition list.
- [x] Added the dedicated artifact creation page. Summary: moved create-definition behavior to its own route and made the main artifact list page stay focused on browsing and editing.
- [x] Split the artifact routes into parent and child pages. Summary: fixed nested routing so `/artifacts/create` is rendered through the parent artifact route outlet instead of being swallowed by the list page.
- [x] Added project artifact browsing. Summary: created a project-scoped artifact page grouped by workflow run so generated artifact runs are visible in context.
- [x] Fixed the project artifact loader. Summary: removed an accidental dependency on the broader project-detail use case and loaded only the project record needed for the artifact page.
- [x] Added the project settings link to the global artifact page. Summary: project settings now link out to the dedicated global artifact management page instead of embedding artifact storage settings inline.
- [x] Added built-in artifact definitions for the core workflow pipeline. Summary: seeded reusable artifacts for business, spec, plan, architecture, review, traceability, analytics, onboarding, and related outputs.
- [x] Added the coding implementation checklist artifact definition. Summary: introduced a new artifact type for the coding implementation checklist output requested in the workflow flow.
- [x] Updated built-in step input/output bindings for the intended workflow flow. Summary: adjusted the seed bindings so the main path follows Idea -> Business -> Architecture -> Tech Spec -> Code Plan -> TDD -> Coding Implementation Checklist -> Review.
- [x] Added a schema migration for the artifact catalog and runtime tables. Summary: created `artifact_definitions`, step binding tables, `artifact_runs`, and the workflow-run step artifact reference column.
- [x] Added a migration to seed built-in step artifact bindings. Summary: populated the database with the core input and output artifact bindings for predefined steps.
- [x] Added a migration to update the workflow artifact flow. Summary: inserted the new coding implementation checklist artifact and adjusted the seeded bindings for architecture and code review.
- [x] Added local-first artifact runtime support in the workflow engine runtime. Summary: workflow runs now resolve local artifact paths first, create runtime artifact rows, and attach artifact-run references to workflow step runs.
- [x] Added route tree regeneration and route-map updates. Summary: registered the new `/artifacts`, `/artifacts/create`, and project artifact routes in the TanStack router manifest and tests.
- [x] Updated the settings navigation and project navigation surfaces. Summary: exposed the artifact and project artifact pages in the relevant navigation sections.
- [x] Added and updated route tests for the artifact pages and workflow-steps page. Summary: covered the new route split, artifact list behavior, create flow, project artifact page, and the workflow reference note.
- [x] Added mapper and repository tests for artifact support. Summary: covered artifact definition mapping, artifact run mapping, and persistence behavior in the Supabase gateway.
- [x] Added workflow-engine use-case coverage for artifact definition and run behavior. Summary: verified the new use cases fit the domain and repository changes.
- [x] Added the audit trail entry for the implementation plan. Summary: captured the task decomposition and execution plan used to land the artifact work.

## Verification

Verified during the implementation pass:

- targeted admin-web Vitest coverage for artifact pages, workflow steps, route map, project settings, and repository behavior
- route tree regeneration for the TanStack router manifest
- Supabase migration files added for the artifact catalog, runtime records, and step binding seeds
- local-first workflow runtime updates for artifact-run creation and step-run linkage

## Residual Notes

- The artifact system now depends on the new catalog and binding tables being present in Supabase. The seeded migration must be applied in the target database for the runtime and UI to show the new bindings correctly.
- The demo gateway mirrors the real artifact flow with static seed data. Any future catalog expansion should update both the seeded catalog and the migration seeds.
- The workflow-step catalog still presents artifact bindings as reusable definition keys. The runtime artifact-run layer remains the source of truth for generated files and sync state.
- The artifact list page intentionally uses collapsible cards so the editor remains usable as the catalog grows.
- The current implementation supports multiple input and output artifact bindings per step, but several built-in steps still use a single primary output binding for readability.

