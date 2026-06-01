DO $$
DECLARE
    has_migration_history BOOLEAN;
    has_local_provider_accounts BOOLEAN;
    has_runner_instances BOOLEAN;
    has_provider_account_identities BOOLEAN;
    has_provider_account_bindings BOOLEAN;
    has_workflow_runs_provider_account_id BOOLEAN;
    has_workflow_steps_provider_account_override_id BOOLEAN;
    ran_create_local_provider_accounts BOOLEAN := false;
    ran_add_activate_provider_account_rpc BOOLEAN := false;
    ran_fix_local_provider_accounts_home_path_uniqueness BOOLEAN := false;
    ran_scope_local_provider_accounts_by_runner_instance BOOLEAN := false;
BEGIN
    has_migration_history := to_regclass('supabase_migrations.schema_migrations') IS NOT NULL;
    has_local_provider_accounts := to_regclass('public.local_provider_accounts') IS NOT NULL;
    has_runner_instances := to_regclass('public.runner_instances') IS NOT NULL;
    has_provider_account_identities := to_regclass('public.provider_account_identities') IS NOT NULL;
    has_provider_account_bindings := to_regclass('public.provider_account_bindings') IS NOT NULL;
    has_workflow_runs_provider_account_id := EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'workflow_runs'
          AND column_name = 'provider_account_id'
    );
    has_workflow_steps_provider_account_override_id := EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
        AND table_name = 'workflow_steps'
        AND column_name = 'provider_account_override_id'
    );

    IF has_migration_history THEN
        EXECUTE
            'SELECT EXISTS (SELECT 1 FROM supabase_migrations.schema_migrations WHERE version = ''20260530093000'')'
            INTO ran_create_local_provider_accounts;
        EXECUTE
            'SELECT EXISTS (SELECT 1 FROM supabase_migrations.schema_migrations WHERE version = ''20260530094000'')'
            INTO ran_add_activate_provider_account_rpc;
        EXECUTE
            'SELECT EXISTS (SELECT 1 FROM supabase_migrations.schema_migrations WHERE version = ''20260530103000'')'
            INTO ran_fix_local_provider_accounts_home_path_uniqueness;
        EXECUTE
            'SELECT EXISTS (SELECT 1 FROM supabase_migrations.schema_migrations WHERE version = ''20260530114500'')'
            INTO ran_scope_local_provider_accounts_by_runner_instance;
    END IF;

    RAISE NOTICE 'migration history table exists: %', has_migration_history;
    RAISE NOTICE '20260530093000_create_local_provider_accounts applied: %', ran_create_local_provider_accounts;
    RAISE NOTICE '20260530094000_add_activate_provider_account_rpc applied: %', ran_add_activate_provider_account_rpc;
    RAISE NOTICE '20260530103000_fix_local_provider_accounts_home_path_uniqueness applied: %', ran_fix_local_provider_accounts_home_path_uniqueness;
    RAISE NOTICE '20260530114500_scope_local_provider_accounts_by_runner_instance applied: %', ran_scope_local_provider_accounts_by_runner_instance;
    RAISE NOTICE 'local_provider_accounts exists: %', has_local_provider_accounts;
    RAISE NOTICE 'runner_instances exists: %', has_runner_instances;
    RAISE NOTICE 'provider_account_identities exists: %', has_provider_account_identities;
    RAISE NOTICE 'provider_account_bindings exists: %', has_provider_account_bindings;
    RAISE NOTICE 'workflow_runs.provider_account_id exists: %', has_workflow_runs_provider_account_id;
    RAISE NOTICE 'workflow_steps.provider_account_override_id exists: %', has_workflow_steps_provider_account_override_id;
END $$;
