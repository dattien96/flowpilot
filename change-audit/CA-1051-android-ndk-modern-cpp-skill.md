# CA-1051 — ndk-modern-cpp-mastery skill added to the android flow-pack group

## What changed

`apps/local-runner/internal/skillpack/flow-pack/android/ndk-modern-cpp-mastery/SKILL.md` (new):

- Enterprise C++20 / Android NDK rules distilled from Lessons 1-16 of the
  Native NDK curriculum: RAII wrappers for fd/mmap/OS handles, Rule-of-5
  `noexcept` moves, zero-copy JNI via direct `ByteBuffer`, `JNI_OnLoad`
  caching + local-ref frames, cache-line-aligned atomics, `mmap`/`msync`
  persistence, two-layer CMake (.a core + .so JNI bridge), and ASan/LSan +
  key zeroization hygiene.
- Frontmatter carries `version: 6` so the BUG-415 pack-version stamp keeps
  repeated bind-init installs idempotent.

## Reach

- `platformGroups` maps `android` projects to the `android` group and `kmm`
  projects to `kmm` + `android` + `ios`, so the skill installs for both
  Android-type and KMP-type projects on the next `bind init` / skillpack
  install — matching the skill's stated targets (TiToCompose, PrivaVault
  `:core:*-ndk` modules).
- No Go code touched; `skillsForPlatform` discovers the new directory
  automatically via `go:embed` + `fs.ReadDir`.

## Invariant

Pack data, not Go enforcement: new guidance lands as a skill file; the
runner's install path and version-stamp contract are unchanged.

## Tests

`go test -count=1 ./internal/skillpack/` — all green, including
`TestBug415_InstalledSkillsCarryPackVersion` (the new SKILL.md reports
current pack version).
