-- Task-159: iOS xcodebuild test command fields
-- Stored at the project level so the local runner can construct the
-- test-config.json automatically without manual user setup.
alter table projects
  add column if not exists xcode_scheme      text,
  add column if not exists xcode_destination text;
