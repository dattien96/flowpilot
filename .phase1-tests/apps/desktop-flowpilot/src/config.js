"use strict";
// Client config (Part A). Part B reads these from a settings file / env.
Object.defineProperty(exports, "__esModule", { value: true });
exports.RUNNER_URL = exports.ADMIN_WEB_URL = void 0;
/** Admin-web dev URL — supervisor.js serves the web on port 3002. */
exports.ADMIN_WEB_URL = "http://localhost:3002";
/** Local runner base URL (used by HttpWsRunnerClient in Part B). The runner serves
 * on 4317 by default (supervisor.js / Justfile). */
exports.RUNNER_URL = "http://127.0.0.1:4317";
