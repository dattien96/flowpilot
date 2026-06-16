# Task-052: Desktop Chat Image Attachments

## Metadata

- Document ID: `Task-052`
- Title: `Desktop Chat Image Attachments`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [04-01: Desktop App Implementation Plan](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [08: Desktop Chat New Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md), [Task-051: Desktop Chat Empty Project Guard And Control Header Cleanup](../done/Task-051-Desktop-Chat-Empty-Project-Guard-And-Control-Header-Cleanup.md), [BUG-062: Desktop Chat Skill Picker Clears Prompt And Ignores Provider Folders](../../09-BugFix/done/BUG-062-Desktop-Chat-Skill-Picker-Clears-Prompt-And-Ignores-Provider-Folders.md), [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](../../09-BugFix/done/BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md)
- Replaces: `none`
- Tags: `desktop, chat, image-attachment, composer, provider-runtime`

## AI Quick View

### Summary

- Add an image attachment icon to the desktop chat controller/composer so users can select screenshots or image files before sending a normal chat prompt.
- Store selected images as local temporary attachment files with metadata for preview, removal, and turn submission.
- Extend the desktop-to-runner turn contract so image attachments are sent as structured attachment metadata and provider adapters can convert them into provider-specific vision payloads.
- Keep this task focused on normal chat image input; workflow artifact storage and long-term attachment library behavior are deferred.

### Current Ask

- Plan and later implement image attachment support for desktop normal chat, with the attach icon located in the Chat controller/composer area.

### Key Decisions

- `D-1` The renderer-side local file path is preview metadata only and must NOT be the value the provider consumes; the provider adapter reads bytes and builds the provider-specific multimodal payload.
- `D-2` Image bytes must cross the desktop→runner HTTP boundary (`POST .../turns` is JSON over a base URL — the runner is a separate process/host). **V1 decision: base64-inline in the turn payload** — simplest, transport-agnostic, and post-normalization images (`D-6`) are small (typically a few hundred KB). A dedicated runner upload endpoint (bytes sent once, referenced by handle on later turns) is deferred to a follow-up and only justified if multi-turn resend of large images becomes a problem. A desktop-renderer path is not guaranteed readable by the runner and is rejected as the source of truth.
- `D-3` Provider payload shape differs and is built runner-side: Codex appends an image input item to the `turn/start` `input` array (the app-server reads the file by path on the runner host); Claude switches the stream-json user `content` from a string to a content-block array with a base64 `image` block.
- `D-4` The desktop renderer owns selection and preview state; the runner owns safe temp persistence on the runner host (Codex needs a real on-host path; Claude needs in-memory bytes to encode).
- `D-5` Attachments are allowed only in `normal_chat` for this slice; workflow/step mode remains unchanged.
- `D-6` Normalize each image in the renderer before it crosses the `D-2` boundary: downscale to long edge <= 1568 px (satisfies both Claude's ~1.15 MP sweet spot and GPT/Codex tiling; never upscale), recompress photographic content to WebP/JPEG q~=0.8 (keep PNG only when alpha or text crispness matters), and strip metadata. This cuts both the base64 payload size and provider vision-token cost; a separate ~128-256 px thumbnail is generated for previews so the chip/timeline never holds the full image.

### Constraints

- Do not send only a local path as the model-visible image content.
- Keep existing provider, model, reasoning, YOLO, and skill controls working as they do after `Task-044` through `Task-051`.
- Use a real icon button in the Chat controller/composer, not explanatory in-app text.
- Preserve current send gating: selected project and provider are still required in normal chat.
- GitNexus MCP tools were not exposed in the current thread; run repository-required `gitnexus_impact` before editing any functions/classes/methods during implementation.

### Open Questions

- Which providers ship in the first pass: Codex only, Claude only, or both capability-gated per adapter? (Gemini is a placeholder adapter and stays gated off.)
- Confirm the exact Codex app-server image input-item schema against a real `codex app-server` build (e.g. `localImage`/`image` with a `path`, vs an inline-data item) — the fake app-server in tests cannot validate this (see `§9.6`).
- Should attachments persist in run history after restart, or be treated as send-time-only temp inputs for V1? (Current lean: send-time-only for V1.)

### Source Refs

- `requirements/10-Refactor/New-System/08-Desktop-Chat-New-Plan.md`
- `Task-044`
- `Task-048`
- `Task-049`
- `Task-051`
- `SS-11`
- `SD-12`
- `CP-18`

## 1. Goal

Allow users to attach one or more images from the desktop chat composer/controller, preview or remove them before send, and submit those images with the current prompt so supported provider adapters can process the image content as real multimodal input.

## 2. Parent Links

- coding plan: [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [04-01: Desktop App Implementation Plan](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [08: Desktop Chat New Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [03: Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)
- specific upstream ids: `04 Shared Contract ProviderTurnInput`, `08 Desktop Chat New Plan attach image`, `CP-18 Phase 7 Follow-Up Runtime`, `CP-18 Phase 8 UI Exposure`, `SS-11 section 6 Follow-Up Prompt Behavior`

## 3. Trigger

The desktop chat plan already calls out `attach image`, and the user wants a Codex/Claude-like image attach button in the Chat controller. The current `ChatInput` and `sendPrompt` flow only submit prompt text and selected skills, so there is no structured path for local image selection, preview, temp storage, or provider-specific vision payload conversion.

## 4. Exact Change

- `T-1` Add an image attach icon button to `ChatInput.tsx` near the existing composer/controller controls. The control should open a native image picker where available and accept common image MIME types such as PNG, JPEG, WebP, and GIF when safe.
- `T-2` Add desktop attachment state for pending prompt images, including `id`, `originalName`, `mimeType`, `localPath`, `sizeBytes`, optional `width`, optional `height`, and optional preview URL or thumbnail data.
- `T-3` Carry image bytes across the desktop→runner HTTP boundary (`D-2`): either embed base64 in the turn payload, or add a runner upload endpoint that persists bytes on the runner host and returns a runner-side handle. The runner owns temp persistence on its own host; a desktop-renderer `localPath` is preview metadata only and must not be sent as the provider source.
- `T-4` Render compact pending-attachment previews above or inside the composer with remove buttons, disabled state while a turn is running, and no layout shift that breaks the existing bottom composer.
- `T-5` Extend `TurnInput`, `HttpWsRunnerClient.sendTurn`, `MockRunnerClient.sendTurn`, Go `TurnInput`, and `TurnRequest` to carry `attachments?: PromptAttachment[]` for chat-mode turns, where each attachment carries the image bytes/base64 (or a runner-side handle per `D-2`), not just a desktop path.
- `T-6` Build provider payloads runner-side per `D-3`: Codex appends an image input item to `codexTurnStartParams` (writing a temp file on the runner host where needed); Claude changes `claudeStream.writeUserTurn` to emit a content-block array with a base64 `image` block. Unsupported providers (e.g. the placeholder Gemini adapter) must fail gracefully or show a capability-gated disabled attach state before send.
- `T-7` Add timeline visibility for sent image attachments on the prompt bubble, using thumbnails or filename chips so the user can confirm what was sent.
- `T-8` Add validation/limits: maximum attachment count, maximum encoded size per image (~1-2 MB post-normalize) and total payload cap, allowed MIME types, missing/unreadable source, and send retry behavior when an attachment cannot be read.
- `T-9` Add focused tests for attachment state, send payload shape, unsupported-provider behavior, and runner contract decoding.
- `T-10` Add a renderer-side normalization pipeline (`D-6`) using `createImageBitmap`/`OffscreenCanvas`/`canvas.toBlob`: downscale to the long-edge target, recompress to WebP/JPEG, strip metadata, and emit both the normalized payload image and a small preview thumbnail. Run before the attachment is added to the turn payload so only normalized bytes cross the `D-2` boundary.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/components/Timeline.tsx`, `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/state/timelineReducer.ts`, `apps/desktop-flowpilot/src/types/contract.ts`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`, `apps/desktop-flowpilot/src/types/flowpilotBridge.d.ts`, `apps/desktop-flowpilot/electron/preload.ts`, `apps/desktop-flowpilot/electron/main.ts`, `apps/local-runner/internal/runner/provider_event.go`, `apps/local-runner/internal/runner/provider_registry.go`, provider adapter files as capability allows
- modules: `desktop-flowpilot`, `local-runner`
- routes: `desktop shell`, `POST /client/workflow-runs/{runId}/turns`
- tables: `none for V1 unless run-history persistence is expanded`

## 6. Acceptance Check

- In normal chat mode, the Chat controller/composer shows an image attach icon button without reintroducing removed header text.
- Selecting an image stores/copies it into a FlowPilot-managed temp/cache location and displays a pending preview chip or thumbnail.
- Users can remove a pending image before send.
- Sending a prompt includes both text and structured image attachment metadata in the desktop and runner turn payload.
- Supported provider adapters receive actual image content or uploaded image references, not just the local path as prompt text.
- Unsupported providers either disable image attachment with clear control state or return a visible non-destructive error before the prompt is lost.
- Sent prompt bubbles show attached image thumbnails or filename chips in the timeline.
- Existing normal chat controls still work: provider, model, reasoning, YOLO, and selected skills.
- Existing workflow/step mode behavior is unchanged and does not require image attachment support.
- Large source images (e.g. a multi-MB full-resolution screenshot) are downscaled/recompressed before send so the per-image payload stays within the configured cap, and the previewed thumbnail is not the full-resolution image.
- Targeted TypeScript and Go tests cover the new attachment path, including the normalization output (target dimension/size) and the send payload shape.

## 7. Out of Scope

- Long-term image library or reusable attachment management.
- Supabase or Google Drive artifact sync for prompt attachments.
- Workflow/step-mode image attachments.
- Non-image file attachments such as PDFs, documents, archives, or source files.
- Screenshot capture tooling; this task only covers selecting existing image files.
- Full provider parity if one adapter lacks documented image support in this pass.

## 8. Completion Notes

- result: Planned only. No code implementation was performed in this turn.
- follow-ups:
  - Run GitNexus impact analysis before modifying `ChatInput`, `sendPrompt`, `TurnInput`, `codexTurnStartParams`, `claudeStream.writeUserTurn`, or other runner turn handlers.
  - Resolve the `D-2` transport decision (base64-in-turn vs runner upload endpoint) and the first supported provider before implementation.
  - Decide whether V1 run history should persist attachments across app restart.
  - Update upstream contract docs once the turn-payload shape is approved: the `attachments` field is an interface-contract change (SS-13 §7.3), so `SD-12`, `CP-18`, and the `04 Shared Contract ProviderTurnInput` definition must record it — do not leave the contract delta only in this task.
- upstream docs updated: `Task-052` (upstream `SD-12` / `CP-18` / `04 Shared Contract` updates deferred until the payload shape is approved — see follow-ups)

## 9. Implementation Guide (Appendix)

This appendix is the code-level execution map. It is grounded in the current data flow:

```text
ChatInput.send()
  -> store.sendPrompt(text, skills, attachments)
    -> TurnInput { ..., attachments }                         contract.ts
    -> client.sendTurn(turnInput)
       HttpWsRunnerClient: POST /client/workflow-runs/{id}/turns (JSON body)
       MockRunnerClient:   in-process echo
  ====================== HTTP boundary (D-2) ======================
Runner (Go):
  handleStartTurn: decode turnBody -> TurnInput (provider_event.go)
    -> startTurn -> runTurn: build TurnRequest (provider_registry.go)
      -> adapter.SendTurn(req, bridge)
         codexAdapter:  codexTurnStartParams(threadId, prompt, skill, attachments)  -> turn/start input[]
         claudeAdapter: claudeStream.writeUserTurn(prompt, attachments)             -> stream-json content[]
```

> Run `gitnexus_impact({target, direction:"upstream"})` on each symbol before editing it (project rule). The high-traffic ones are `sendPrompt`, `TurnInput`, `runTurn`, `codexTurnStartParams`, `writeUserTurn`.

### 9.1 Shared attachment shape

`PromptAttachment` is the wire shape that crosses `D-2`. The renderer fills it after normalization (`D-6`); `localPath`/`previewUrl` are renderer-only and are NOT sent.

TypeScript (`apps/desktop-flowpilot/src/types/contract.ts`):

```ts
export interface PromptAttachment {
  id: string;
  kind: "image";
  originalName: string;
  /** Encoded MIME after normalization: image/webp | image/jpeg | image/png | image/gif. */
  mimeType: string;
  /** Base64 of the NORMALIZED bytes (no data: prefix). V1 transport = inline (D-2). */
  data: string;
  sizeBytes: number;
  width?: number;
  height?: number;
}

export interface TurnInput {
  // ...existing fields...
  /** Image attachments for chat-mode turns (Task-052). Omitted in workflow/step mode. */
  attachments?: PromptAttachment[];
}
```

Go (`apps/local-runner/internal/runner/provider_event.go`, mirror of contract.ts):

```go
type PromptAttachment struct {
    ID           string `json:"id"`
    Kind         string `json:"kind"` // "image"
    OriginalName string `json:"originalName"`
    MimeType     string `json:"mimeType"`
    Data         string `json:"data"` // base64, no data: prefix
    SizeBytes    int64  `json:"sizeBytes"`
    Width        int    `json:"width,omitempty"`
    Height       int    `json:"height,omitempty"`
}
```

### 9.2 Renderer: ChatInput (`ChatInput.tsx`) — `T-1`, `T-4`, `T-10`

- Add a hidden `<input type="file" accept="image/png,image/jpeg,image/webp,image/gif" multiple>` plus a real icon button in the controller strip (next to the Skills/YOLO controls, `T-1`). Reuse the existing `.chat-controller-*` button styling; no explanatory text (`Constraints`).
- Local state: `const [attachments, setAttachments] = useState<PendingAttachment[]>([])` where `PendingAttachment = PromptAttachment & { previewUrl: string }`.
- On file pick: run normalization (`§9.3`), validate caps (`T-8`), append. Disable the button while `blocked` and when `attachments.length >= MAX_COUNT`.
- Render pending chips above `.input-bar` (thumbnail + filename + remove `×`), disabled while a turn runs (`T-4`). No layout shift of the bottom composer.
- `send()`: pass attachments and clear them with the text:

```ts
const send = () => {
  if (!canSend) return;
  void sendPrompt(text.trim(), isChatMode ? selectedSkills : undefined,
                  isChatMode ? attachments.map(toWire) : undefined);
  setText(""); setSelectedSkills([]); setAttachments([]); setSkillPickerOpen(false);
};
```

- `canSend` is unchanged: text is still required (image-only sends are out of scope for V1; keep current gating — project + provider still required).

### 9.3 Renderer: normalization pipeline (`D-6`, `T-10`)

New util `apps/desktop-flowpilot/src/lib/normalizeImage.ts` (renderer-only; uses `createImageBitmap`/`OffscreenCanvas`, available in the Electron renderer):

```ts
const MAX_EDGE = 1568;          // long-edge cap (Claude sweet spot; safe for Codex/GPT)
const THUMB_EDGE = 192;
const QUALITY = 0.8;

export async function normalizeImage(file: File): Promise<{ wire: PromptAttachment; previewUrl: string }> {
  const bmp = await createImageBitmap(file);
  const scale = Math.min(1, MAX_EDGE / Math.max(bmp.width, bmp.height)); // downscale only
  const w = Math.round(bmp.width * scale), h = Math.round(bmp.height * scale);

  // Keep PNG only when transparency must be preserved; else WebP for size.
  const keepPng = file.type === "image/png" /* && needsAlpha(bmp) */;
  const mime = keepPng ? "image/png" : "image/webp";

  const canvas = new OffscreenCanvas(w, h);
  canvas.getContext("2d")!.drawImage(bmp, 0, 0, w, h);
  const blob = await canvas.convertToBlob({ type: mime, quality: keepPng ? undefined : QUALITY });
  const data = await blobToBase64(blob); // strip the "data:...;base64," prefix

  const thumb = new OffscreenCanvas(/* THUMB_EDGE-scaled */ 0, 0);
  // ...draw scaled-down thumbnail, convertToBlob -> previewUrl (object URL or small data URL)...

  bmp.close();
  return {
    wire: { id: crypto.randomUUID(), kind: "image", originalName: file.name, mimeType: mime,
            data, sizeBytes: blob.size, width: w, height: h },
    previewUrl,
  };
}
```

- Validation (`T-8`): reject MIME outside the allowlist; if `blob.size` exceeds the per-image cap (~1-2 MB) after one pass, re-encode at lower quality or reject with a visible message. Enforce `MAX_COUNT` and a total-payload cap.

### 9.4 Renderer: store + transport (`T-5`)

- `store.ts` — widen the action signature and thread it through:

```ts
sendPrompt(prompt: string, skills?: string[], attachments?: PromptAttachment[]): Promise<void>;
// inside: include `attachments` in the TurnInput; also stash on the timeline prompt item
//         so the bubble can render thumbnails (T-7) — see §9.5.
const turnInput: TurnInput = { /* ...existing... */, attachments: attachments?.length ? attachments : undefined };
```

- `HttpWsRunnerClient.sendTurn` — add one line to the POST body:

```ts
{ stepId, prompt, selectedSkills, reasoningEffort, model, yoloMode,
  attachments: input.attachments, scenario: this.scenario }
```

- `MockRunnerClient.sendTurn` — accept `input.attachments` and echo them onto the rendered prompt (no provider call); lets the UI/timeline be developed against the mock.

### 9.5 Renderer: timeline (`T-7`)

- The timeline `prompt` item already carries `selectedSkills`; add `attachments?: { id; previewUrl?; originalName; mimeType }[]` to the prompt item type (`timelineReducer.ts` / timeline types).
- `store.sendPrompt` sets it when pushing the prompt bubble (`§9.4`). For round-tripped runs from history, the bubble falls back to filename chips when no thumbnail is available.
- `Timeline.tsx` renders thumbnails (or filename chips) on the prompt bubble. Reuse chip styling; lazy-load thumbnails.

### 9.6 Runner: contract plumbing (`T-5`)

Thread `attachments` through the existing decode → TurnInput → TurnRequest chain (each is a one-field addition):

1. `interactive_handlers.go` `turnBody`: add `Attachments []PromptAttachment json:"attachments"`.
2. `handleStartTurn`: pass `Attachments: body.Attachments` into the internal `TurnInput{...}`.
3. `provider_event.go` `TurnInput`: add `Attachments []PromptAttachment json:"attachments,omitempty"`.
4. `interactive_service.go` `runTurn`: add `Attachments: in.Attachments` to the `TurnRequest{...}` literal.
5. `provider_registry.go` `TurnRequest`: add `Attachments []PromptAttachment`.

### 9.7 Runner: Codex adapter (`T-6`, `D-3`)

Codex reads images by **path on the runner host**, so the adapter writes the base64 bytes to a temp file and passes a path item:

- `codex_adapter.go SendTurn`: before `turn/start`, for each image attachment, decode base64 and write to a runner-managed temp file (e.g. under `os.TempDir()/flowpilot-attach/<turnId>/`). Collect the absolute paths. `defer os.RemoveAll(dir)` after the turn.
- `codex_appserver.go codexTurnStartParams(threadID, prompt, skill, imagePaths []string)`: append image items to the existing `input` array:

```go
input := []any{ map[string]any{"type": "text", "text": prompt, "text_elements": []any{}} }
for _, p := range imagePaths {
    input = append(input, map[string]any{"type": "localImage", "path": p}) // confirm exact key vs real app-server
}
```

> **Open item (`§Open Questions`):** the exact image item type/keys (`localImage` path vs inline image data) must be validated against a real `codex app-server` build — the scripted fake in tests cannot confirm it. Isolate the item construction behind a small helper so the schema can change in one place.

### 9.8 Runner: Claude adapter (`T-6`, `D-3`)

Claude takes a stream-json content-block array; the adapter base64-encodes inline (it already holds the bytes):

- `claude_stream.go writeUserTurn`: overload to accept attachments and emit blocks instead of a bare string when present:

```go
func (s *claudeStream) writeUserTurn(text string, images []PromptAttachment) error {
    var content any = text
    if len(images) > 0 {
        blocks := []any{ map[string]any{"type": "text", "text": text} }
        for _, im := range images {
            blocks = append(blocks, map[string]any{
                "type": "image",
                "source": map[string]any{"type": "base64", "media_type": im.MimeType, "data": im.Data},
            })
        }
        content = blocks
    }
    return s.writeJSON(map[string]any{
        "type": "user",
        "message": map[string]any{"role": "user", "content": content},
        "parent_tool_use_id": nil,
    })
}
```

- `claude_adapter.go SendTurn`: pass `req.Attachments` into `writeUserTurn`. `media_type` must match the normalized encoding (Anthropic accepts jpeg/png/gif/webp).

### 9.9 Capability gating (`T-6`)

- `provider_event.go ProviderCapabilities`: add `Vision bool json:"vision"`.
- Set `Vision: true` in `codexAdapter.Capabilities()` and `claudeAdapter.Capabilities()`; leave it false on the placeholder/Gemini adapter.
- Desktop: surface vision capability per provider (extend the provider/registration data the renderer already loads) and disable the attach button + hide pending chips when the selected provider has `vision=false`. If an attachment somehow reaches an unsupported provider, the runner returns a visible, non-destructive error (the prompt text is preserved per the existing BUG-050 up-front-render pattern).

### 9.10 Tests (`T-9`)

- TS: `normalizeImage` output (downscaled dims ≤ target, size within cap, correct mime); `ChatInput` add/remove/disabled-while-running; `store.sendPrompt` includes `attachments` in `TurnInput`; `HttpWsRunnerClient` body shape; capability-gated disabled state.
- Go: `turnBody` JSON decode with `attachments`; Codex `codexTurnStartParams` appends image items in order; Claude `writeUserTurn` emits the content-block array with correct `media_type`/`data`; `Vision=false` provider rejects/ignores attachments gracefully; the temp-file write/cleanup path.

### 9.11 Suggested build order

1. `§9.1` shared types (TS + Go) — lock the contract first.
2. `§9.6` runner plumbing (compiles, no behavior change).
3. `§9.8`/`§9.7` adapters behind `Vision` capability.
4. `§9.3` normalization util + `§9.2` ChatInput against `MockRunnerClient`.
5. `§9.4` real transport, `§9.5` timeline, `§9.9` gating.
6. `§9.10` tests alongside each layer; then update upstream docs (`§8` follow-ups).
