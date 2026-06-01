DROP FUNCTION IF EXISTS activate_provider_account(UUID);

DROP TABLE IF EXISTS local_provider_accounts CASCADE;
DROP TABLE IF EXISTS runner_instances CASCADE;
DROP TABLE IF EXISTS provider_account_identities CASCADE;
DROP TABLE IF EXISTS provider_account_bindings CASCADE;
