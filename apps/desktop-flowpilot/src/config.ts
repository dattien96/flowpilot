// Client config (Part A). Part B reads these from a settings file / env.

/** Admin-web dev URL — supervisor.js serves the web on port 3002. */
export const ADMIN_WEB_URL = "http://localhost:3002";

/** Local runner base URL (used by HttpWsRunnerClient in Part B). */
export const RUNNER_URL = "http://localhost:8080";
