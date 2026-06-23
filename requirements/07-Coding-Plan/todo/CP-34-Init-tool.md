Tool se co cac default skill

Init -> init skill co san vao folder cua tung project ma user bind
Init casc default step/flow
R chay CP-31 de chuan hoa tai lieu co san truoc do de flowpilot continue run on

if any one start using this tool
they can have exist data doc
we need use AI. skills,...

convert to our system SS-SD-CP . something like that

---

**Design & dependents:** This init/setup tool is the install + health-check home for the Context & Regression Engine. It must (1) install the bundled flow skill pack into each bound project's `.claude/.codex/.gemini` dirs, (2) install + verify external tooling (GitNexus, RTK, node), and (3) run [CP-31](./CP-31-Auto-Document-Process.md) to normalize existing docs into SS-SD-CP. The contracts it consumes are defined in [SD-17 §3.6 / §6.4](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md); the consuming slices are [CP-35 P-6 / P-7](../inprogress/CP-35-Context-And-Regression-Engine-Rollout.md).


NOT DONE: init supabase db schema when use config suspabase setting