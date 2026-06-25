If unit test failed -> check business logic -> ask user
Dont change tests to make it pass new code

---

**Design & delivery:** This rule is formalized as the **Oracle Integrity Guard** in [SD-17 §7.3](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) and delivered by [CP-35 P-5](../inprogress/CP-35-Context-And-Regression-Engine-Rollout.md). The guard enforces exactly this: a failing pre-existing test triggers an SS/SD re-check and may never be weakened to pass; the oracle changes only after the upstream spec is updated and a human confirms.