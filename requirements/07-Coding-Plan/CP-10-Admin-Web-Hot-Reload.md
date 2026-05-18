# CP-10: Admin-Web Docker Hot Reload

**Maps from:** CP-01 (Vite foundation), local Docker development workflow
**Phase:** Parallel, but should be completed immediately after CP-01

---

## 1. Goal

Make `just docker-up` the default admin-web developer loop:

- Start the admin web inside Docker with Vite dev server reachable from the host.
- Change any frontend source file on the host machine and see either HMR or full-page reload without restarting the container.
- Keep the setup stable on bind-mounted workspaces where native file events are unreliable.

---

## 2. Current State And Gap

Current repo state already gives us a partial Docker dev workflow:

- `Justfile` exposes `docker-up`.
- `docker-compose.yml` bind-mounts the whole repo into `/workspace`.
- `admin-web` already runs `npm run dev -- --host 0.0.0.0 --port 3001`.
- `CHOKIDAR_USEPOLLING=true` is already set in Compose.

Remaining gaps:

- `apps/admin-web/vite.config.ts` does not explicitly configure Docker-safe file watching.
- HMR connection details are not pinned for the browser-to-container path.
- `WATCHPACK_POLLING` is a leftover webpack/Next.js-style env and should not be treated as the Vite solution.
- There is no explicit acceptance checklist for "edit file on host -> browser reloads".

---

## 3. Files In Scope

- `apps/admin-web/vite.config.ts`
- `docker-compose.yml`
- `apps/admin-web/Dockerfile.dev`
- `Justfile`
- `apps/admin-web/README.md`

Optional only if needed:

- `.dockerignore`
- root `.env.example` or admin-web env docs

---

## 4. Implementation Plan

### 4.1 Harden Vite Server Config For Docker

Update `apps/admin-web/vite.config.ts` to make dev-server behavior explicit instead of relying on CLI flags alone:

- Set `server.host = "0.0.0.0"`.
- Set `server.port = 3001`.
- Set `server.strictPort = true`.
- Set `server.watch.usePolling = true`.
- Set a polling interval tuned for Docker bind mounts, such as `300` to `500` ms.
- Set `server.hmr.host = "localhost"` for the host browser entrypoint.
- Set `server.hmr.clientPort = 3001` so the browser reconnects through the published port.

Reference shape:

```ts
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 3001,
    strictPort: true,
    watch: {
      usePolling: true,
      interval: 300,
    },
    hmr: {
      host: "localhost",
      clientPort: 3001,
    },
  },
})
```

### 4.2 Simplify Docker Compose Around Vite

Keep `docker-compose.yml` aligned with Vite instead of mixed framework assumptions:

- Keep the repo bind mount: `.:/workspace`.
- Keep the named volume for `node_modules`.
- Keep `CHOKIDAR_USEPOLLING=true`.
- Remove reliance on `WATCHPACK_POLLING` for admin-web because it is not the Vite watcher.
- Keep port mapping `3001:3001`.
- Keep admin-web depending on local-runner.

If the Vite config becomes authoritative, the Compose command can stay simple:

```yaml
command: sh -c "npm ci && npm run dev"
```

### 4.3 Keep The Dev Image Focused On Local Iteration

Review `apps/admin-web/Dockerfile.dev` only for dev-loop correctness:

- Ensure the container starts in `apps/admin-web`.
- Keep dependency install cached at image build time.
- Avoid introducing production-only optimizations into the dev image.
- If startup time becomes a problem, move repeated install work out of the hot path while preserving deterministic dependencies.

### 4.4 Preserve `just docker-up` As The Single Entry Point

Keep the user-facing workflow unchanged:

```bash
just docker-up
```

No extra manual commands should be required after startup.

### 4.5 Document The Expected Developer Experience

Update `apps/admin-web/README.md` with a Docker section:

- Start: `just docker-up`
- Open: `http://localhost:3001`
- Edit files under `apps/admin-web/src/**`
- Expect instant HMR for component/style changes
- Expect full-page reload for config or route-tree regeneration cases

---

## 5. Verification Checklist

Run the following acceptance checks:

1. Start the stack with `just docker-up`.
2. Open `http://localhost:3001`.
3. Edit a React component under `apps/admin-web/src/` and confirm UI updates without container restart.
4. Edit a CSS or Tailwind-driven component and confirm browser refresh behavior is automatic.
5. Edit a route file and confirm the app reloads cleanly.
6. Confirm container logs do not show watcher failure or websocket connection errors.
7. Confirm `local-runner` still resolves from the admin web container.

---

## 6. Definition Of Done

- [ ] `just docker-up` starts admin-web successfully on `localhost:3001`
- [ ] Host file edits under `apps/admin-web/src/**` trigger HMR or full reload automatically
- [ ] No manual container restart is needed for normal frontend development
- [ ] Vite watcher settings are explicit in `vite.config.ts`
- [ ] Compose config is aligned with Vite rather than legacy webpack/Next.js watcher flags
- [ ] README documents the Docker hot reload workflow

---

## 7. Non-Goals

- Rebuilding the production Docker image strategy
- Solving backend hot reload for `local-runner`
- Adding reverse proxies, TLS, or external dev ingress
- Changing the public developer command away from `just docker-up`
