# CA-1072 — skillpack: add android-adaptive-multi-screen and edge-to-edge skills

## What changed

Added two new skills under
`apps/local-runner/internal/skillpack/flow-pack/android/`:

- `android-adaptive-multi-screen`: Jetpack Compose multi-screen support,
  WindowSizeClass breakpoints, canonical two-pane layouts, foldables, and
  desktop windowing parity (Google Play Tier 1 quality guidelines).
- `edge-to-edge`: adaptive edge-to-edge migration for Compose apps — system
  bar overlap, IME insets, status/navigation bar legibility.

No Go code changes: the pack is embedded whole via `//go:embed flow-pack` and
`skillsForPlatform` enumerates group directories dynamically, so the new
skills install automatically for `platform=android` (and `kmm`, which pulls
the android group). Already-initialized projects pick them up on the next
bind-init because the missing skills flip `PackStatus.Current` to false,
un-skipping `skillpack.Install`. No `PackVersion` bump needed for pure
additions.

## Invariant

Install surface unchanged — `Install` still copies only `SKILL.md` per skill
into the four provider roots; both new skills are single-file and conform.

## Tests

- `go test -count=1 ./internal/skillpack/...` — pass (android install-count
  assertions derive from `skillsForPlatform`, no list updates required).
