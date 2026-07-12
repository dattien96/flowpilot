# Walkthrough: Task-225 File Artifact Instance Output Structure

## Verdict

**PASS** after NEEDS_FIX loop (merge same-path sections; schema `additionalProperties: false` + strip `format`).

## Match to plan

| Item | Status |
|------|--------|
| Parse `structure` only (no `format`) | Done |
| Paths-only prompt = write contract only | Done |
| Structured prompt template + Why bullets | Done |
| Flex ATX matcher | Done |
| `r-artifact-output-structure` after existence | Done |
| Child gate family both rules | Done |
| Migration schema + coder-summary heuristic | Done |
| Merge same-path multi-binding sections | Done (post-review) |

## Tests run

```bash
go test ./internal/flowgate/ -count=1
go test ./internal/runner/ -count=1 -run 'FileArtifact|ArtifactOutput|Structure|RequiredFile|ComposeFlow|AppendRequired'
```

## Residual

- Artifacts UI editor for `structure` is follow-up.
- Migration heuristic seeds only path strings matching coder-summary; other coding memos need manual structure in instance config.
