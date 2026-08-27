# Task-282: TUI Image Attach Via Existing Turn API (CP-56 P-3b)

## Metadata

- Document ID: `Task-282`
- Title: `TUI Image Attach Via Existing Turn API`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-3b / D-10), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-052](../done/Task-052-Desktop-Chat-Image-Attachments.md), [Task-281](./Task-281-TUI-Slash-Skill-Attachments.md)
- Child Documents: `none`
- Related Documents: Runner `PromptAttachment` / Task-052; desktop ChatInput attachments
- Replaces: `None`
- Tags: `cli-tui, attachments, vision`

## AI Quick View

### Summary

- `/image <path>|list|clear` queues images; next **chat** turn sends `attachments[]` base64 via existing API — **no runner changes**.
- Match Desktop safety posture: vision-provider gate, max 6 images, 1568px long-edge normalization, and 2 MiB normalized cap.

### Current Ask

Implement P-3b path-based image attach.

### Key Decisions

- `T-1` Chat mode only (omit attachments in flow/step turns — desktop parity).
- `T-2` Only `codex` and `claude` may queue/send images; Grok is explicitly non-vision and other providers fail closed.
- `T-3` Decode png/jpeg/webp/gif, use the first frame, and downscale the long edge to 1568px. Preserve PNG sources as PNG; encode JPEG/WebP/GIF sources as JPEG quality 80 (pure-Go stack has no WebP encoder). Then enforce 2 MiB and record normalized width/height and byte size.
- `T-4` Queue at most 6 images. Clear only after the turn POST is accepted; retain on POST failure.
- `T-5` `id` = uuid; `kind` = `"image"`.
- `T-6` No ANSI pixel preview required.
- `T-7` Before full decode, enforce a bounded raw-file and decoded-pixel limit to avoid memory bombs; errors identify the rejected file.

### Constraints

- Forbidden runner/desktop edits. Read desktop normalize only as reference.

### Open Questions

- None.

### Source Refs

- CP-56 D-10, P-3b; A3b.1–A3b.10; M3b.

---

## 1. Goal

Users can attach local image files to chat turns from the CLI.

## 2. Parent Links

- coding plan: CP-56 P-3b, D-10
- tech design: existing Task-052 `PromptAttachment` turn contract
- system spec: n/a; client-only delta
- specific upstream ids: CP-56 A3b.1–A3b.10

## 3. Trigger

Task-281 establishes pending attachment semantics; this slice adds image normalization and wire DTOs without runner changes.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/images.go
internal/tui/app/images_test.go
internal/tui/app/testdata/tiny.png  # fixture for tests
```

### 4.2 Code

```go
type ImageState struct {
    Pending []client.PromptAttachment
}

func LoadImageAttachment(path string) (client.PromptAttachment, error)
// read, sniff/decode, normalize, enforce cap, base64.StdEncoding

func (s *ImageState) HandleSlash(args []string) (msg string, err error)
func (s ImageState) SnapshotForTurn() []client.PromptAttachment
func (s *ImageState) ClearAfterTurnAccepted()
```

```go
func NormalizeImage(raw []byte, mime string) (out []byte, outMime string, width, height int, err error)
```

### 4.3 Wire

- `/image` → ImageState
- Composer hint: `images: a.png (2)`
- Send path (chat only): snapshot attachments; clear only after `SendTurn` succeeds
- Timeline prompt row may list attachment names (text only)

### 4.4 Allowed mime

`image/png`, `image/jpeg`, `image/webp`, `image/gif` — reject others.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/images.go`, additive tests and tiny fixtures
- modules: add pure-Go image decode/resize dependency if stdlib is insufficient; no CGO encoder
- routes: existing turn POST only
- tables: none

## 6. Acceptance Check

- [ ] A3b.1–A3b.10 green: normalization/dimensions, provider gate, max-count, memory bounds, and retain-on-POST-failure included
- [ ] Manual M3b on a vision-capable provider

## 7. Out of Scope

- Clipboard paste matrix, drag-drop, terminal image preview, workflow-mode attachments

## 8. Completion Notes

- result: pending
- follow-ups: Task-283
- upstream docs updated: CP-56-Test-Steps evidence when complete
