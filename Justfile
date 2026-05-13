# ============================================================================
# FlowPilot Justfile
# Modern command runner for the FlowPilot project
# Install: brew install just
# Usage: just --list
# ============================================================================

set windows-shell := ["C:\\Users\\dat.nguyen\\AppData\\Local\\Programs\\Git\\bin\\bash.exe", "-lc"]

# --- Variables ---
ADMIN_WEB_PATH := "apps/admin-web"
ADMIN_WEB_PORT := "3001"

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
