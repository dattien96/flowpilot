# CA-672 — Task-311: composer solid #1e1e1e, bottom notice outside frame

## Context

Task-311 makes the right sidebar width-reactive (no F2/F4) and moves chrome into
the composer frame (top-left Chat/Flow · ready · agent, bottom-right Model ·
reasoning · YOLO). The previous paint turned the whole chat column uniform
`#1e1e1e` (`styleChatBar`), but inner styled segments (Chat: label, skill
mentions, attach chip, caret) each end with a lipgloss reset that punches a
black hole in the gray card. The watchdog "input stalled: terminal is not
delivering keys — close this window to exit" was also rendered inside the
top-left title, cluttering the chrome. User request: transcript stays canvas
`#0d0d0d` (CA-532), only the composer frame is solid `#1e1e1e`; warnings and
Copied toasts become a single small line at the bottom of the chat pane,
outside the frame.

## Changes

1. **Solid composer (CA-532 restore):**
   - `chat_box.go:isYouBoxRow` now plain-only (`s==stripANSI(s)`) so styled
     composer rows (`╭`/`│` with ANSI title) are not mistaken for user You-box.
   - `chat_box.go:paintComposerRow` wraps `paintRow(..., styleChatBar)` and
     re-applies the bar bg after every inner `\x1b[0m` reset, so `Chat:`,
     posture, `[coding]` mentions, etc. keep the solid `#1e1e1e` card.
   - `app.go:renderChatPane` now tracks `inputStart/inputEnd` and paints
     `input` rows with `paintComposerRow`, transcript/filler with `styleCanvas`,
     You-box via `padYouBoxRow`. `padYouBoxRowUniform` is no longer used in
     the main loop (composer-only bar, transcript canvas).
   - `app.go:chatFrameTitle` carries the bar bg on every segment
     (`chatBarBg`) and filters `isStalledStatus` so the long stall warning does
     not appear in top-left. `chatBarBg` helper added.
   - `chat_textarea.go:newChatTextArea` sets `FocusedStyle`/`BlurredStyle` Text,
     Placeholder and Base with `#1e1e1e` bg so `textarea.View()` inside the
     composer does not create holes.
   - `file_mention.go`/`skill.go` add `highlightMentionsOnBar` helpers (kept
     additive; composer paint now fixes holes at the View layer, preserving
     `renderInputLine`’s original style for old tests).

2. **Bottom notice outside the frame:**
   - `app.go:renderBottomNotice` + `mouse.go:tuiChrome.bottomNoticeBlock/H`
     combine flash toast (`Copied ...`) and the stalled warning into one small
     line after the composer frame. `tuiChrome.messagesHeight` now subtracts
     `bottomNoticeH` (not `statusH`), so the viewport stays `h`. Legacy
     `renderStatusLine` is kept for old callers (`Copied` still returned).
   - `app.go:renderChatPane` renders the bottom notice after the input block
     and paints it with `styleCanvas` (outside the elevated card).

3. **Additive tests:** `tui_composer_solid_bg_test.go` — `TestComposer_SolidChatBarAndTranscriptCanvas`,
   `TestComposer_MentionHighlightsSurviveSolidBar`, `TestChatFrameTitle_DoesNotLeakStalled`,
   `TestBottomNotice_ShowsStalledOutsideFrame`, `TestBottomNotice_ShowsCopiedOutsideFrame`
   across claude/codex/grok widths.

## Verification

- `go test ./internal/tui/app -count=1` — all green (including
  `TestStatusBar_*`, `TestRenderInputLine_SkillTokensHighlighted`,
  `TestCopiedMsg_FlashToastNotChatTimeline`, `TestView_PaintsCanvas*`).
- Manual `View` inspection with `forceTrueColor`: composer `╭`/`│`/`╰` rows
  contain `48;2;30;30;30` and no `48;2;13;13;13`; transcript filler is
  `48;2;13;13;13`; stalled `input stalled` appears after `╰` border, not in
  `Chat:` title; `Copied selection` appears once after `╰`, not in header.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: bugfix
summary: Composer solid #1e1e1e with canvas transcript and stalled/Copied moved to bottom outside-frame notice
# --->8---
