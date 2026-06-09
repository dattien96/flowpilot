# ============================================================================
# FlowPilot Justfile
# Modern command runner for the FlowPilot project
# Install: brew install just
# Usage: just --list
# ============================================================================

set windows-shell := ["C:\\Users\\dat.nguyen\\AppData\\Local\\Programs\\Git\\bin\\bash.exe", "-lc"]

# --- Variables ---
ADMIN_WEB_PATH := "apps/admin-web"
ADMIN_WEB_PORT := "3002"
LOCAL_RUNNER_PATH := "apps/local-runner"
LOCAL_RUNNER_PORT := "4317"

# --- Default Target ---
default: help

# Show help message
help:
    @just --list

# ============================================================================
# WEB
# ============================================================================

# Install admin web dependencies
web-install:
    @echo "Installing admin web dependencies..."
    @cd {{ADMIN_WEB_PATH}} && npm install
    @echo "Done"

# Install admin web dependencies from the repo root with an explicit command name
admin-web-install:
    @echo "Installing admin web dependencies..."
    @cd {{ADMIN_WEB_PATH}} && npm install
    @echo "Done"

# Start admin web on the default local port
web-dev:
    @echo "Starting admin web on port {{ADMIN_WEB_PORT}}..."
    @cd {{ADMIN_WEB_PATH}} && npm run dev -- --port {{ADMIN_WEB_PORT}}

# Start admin web on a custom port
web-dev-port port:
    @echo "Starting admin web on port {{port}}..."
    @cd {{ADMIN_WEB_PATH}} && npm run dev -- --port {{port}}

# Run lint for admin web
web-lint:
    @echo "Linting admin web..."
    @cd {{ADMIN_WEB_PATH}} && npm run lint

# Run production build for admin web
web-build:
    @echo "Building admin web..."
    @cd {{ADMIN_WEB_PATH}} && npm run build

# Install local runner dependencies
runner-install:
    @echo "Downloading local runner dependencies..."
    @cd {{LOCAL_RUNNER_PATH}} && go mod tidy

# Start the local runner on the default local port
runner-dev:
    @echo "Starting local runner on port {{LOCAL_RUNNER_PORT}}..."
    @cd {{LOCAL_RUNNER_PATH}} && go run ./cmd/flowpilot runner serve --port {{LOCAL_RUNNER_PORT}}

# Start admin web and local runner together
dev:
    @node scripts/supervisor.js --web-port {{ADMIN_WEB_PORT}} --runner-port {{LOCAL_RUNNER_PORT}}


# Print runner health as JSON
runner-health:
    @cd {{LOCAL_RUNNER_PATH}} && go run ./cmd/flowpilot runner health

# Detect local AI provider CLIs
runner-providers:
    @cd {{LOCAL_RUNNER_PATH}} && go run ./cmd/flowpilot providers detect

# Choose a connected provider account, review its usage limits, and open a terminal in the current workspace
runner-provider-terminal:
    @cd {{LOCAL_RUNNER_PATH}} && go run ./cmd/flowpilot providers terminal

# List local skills markdown
runner-skills:
    @cd {{LOCAL_RUNNER_PATH}} && go run ./cmd/flowpilot skills list

# List local flows markdown
runner-flows:
    @cd {{LOCAL_RUNNER_PATH}} && go run ./cmd/flowpilot flows list

# ============================================================================
# DOCKER
# ============================================================================

# Build and run the admin web + local runner with Docker Compose
docker-up:
    @echo "Starting admin web + local runner with Docker Compose..."
    @USERPROFILE="${USERPROFILE:-$HOME}" docker compose up --build admin-web local-runner

# Stop Docker Compose services
docker-down:
    @docker compose down

# ============================================================================
# SUPABASE
# ============================================================================

# Deploy workflow engine edge functions to a Supabase project
supabase-functions-deploy project_ref:
    @echo "Deploying workflow engine edge functions to project {{project_ref}}..."
    @npx supabase functions deploy workflow-engine-start-run --project-ref {{project_ref}}
    @npx supabase functions deploy workflow-engine-toggle-yolo-mode --project-ref {{project_ref}}
    @npx supabase functions deploy workflow-engine-submit-step-approval --project-ref {{project_ref}}
    @echo "Done"
