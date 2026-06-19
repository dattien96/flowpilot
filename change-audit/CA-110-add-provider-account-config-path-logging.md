# CA-110: Add Provider Account Config Path Logging

## Scope

Add diagnostic log lines to `loadProviderAccountState` and `saveProviderAccountState` so config-path drift and missing-file events are immediately visible in server logs.

## Completed

- `loadProviderAccountState`: logs `[provider-accounts] config not found path=%q` (warning) when the file does not exist.
- `saveProviderAccountState`: logs `[provider-accounts] config saved path=%q accounts=%d` after a successful write.

## Verification

- All existing provider-accounts tests continue to pass.
- Log lines appear in runner output when config is absent or first written.

## Residual Notes

- No log on successful reads to avoid spam on every `ListProviderAccounts` call.
- Log prefix `[provider-accounts]` is grep-friendly.
