# CA-028: Workflow Run Log Fallback and Response Versioning

## Scope

This audit covers the log-backed response fallback and the fix for response version overwrites on follow-up prompts.

## Completed

- Added log-backed fallback content so the run detail page can show response text even when artifact files are unavailable.
- Added parsing for `Begin prompt:` and provider output logs so the run title and response tab can resolve from logs.
- Fixed the bug where the latest response could overwrite earlier response versions for the same step.
- Kept the prompt tab bound to the correct prompt attempt while preserving the correct response per version.

## Verification

- Verified the response body stays aligned with the matching local file version.
- Confirmed the prompt and response tabs no longer collapse into the latest prompt only.

