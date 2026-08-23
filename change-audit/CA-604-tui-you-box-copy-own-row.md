# CA-604: [copy] rides its own last row in the You box

## What

Operator: "cứ show line by line thôi, xong cuối cùng tạo 1 dòng cuối hiện nút copy". The [copy] chip was embedded in the last content row, which cut/offset prompt text (`symbols: …` row) to make room for the chip.

## Why

- `youBox` appended the chip to the last line's body and padded/truncated around it — any prompt whose last line neared the inner width got its tail cut.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go` — `youBox`: every content line renders as `│` + " " + line + plain pad + `│` (never touches the chip). When `copyOn`, a dedicated last row is appended after all content rows: `│` + pad + `[copy]` + `│` with `Copy=true` (click/copy hit-test unchanged). Bottom border after it.

## Tests

- `you_box_lines_test.go` — `TestRegression_YouBoxCopyChipOnLastRow` now asserts the chip row holds only the chip (borders stripped), sits directly above the bottom border, and contains no content text; `TestRegression_YouBoxUserPastedPromptAllLines` asserts the chip does NOT ride the `symbols:` line.
- `go test ./internal/tui/app -count=1` 14s green; `go vet ./internal/tui/...` clean. Manual dump 120/197: 5 content lines full width, `[copy]` on its own row flush right.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: give the You box a dedicated last row for the copy chip so prompt lines always render uncut
# --->8---