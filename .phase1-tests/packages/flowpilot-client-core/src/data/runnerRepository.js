"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.HttpRunnerRepository = void 0;
async function readError(response) {
    const text = await response.text();
    if (!text)
        return `Request failed with status ${response.status}.`;
    try {
        const payload = JSON.parse(text);
        return payload.error || text;
    }
    catch {
        return text;
    }
}
class HttpRunnerRepository {
    httpClient;
    runnerBaseUrl;
    constructor(httpClient, runnerBaseUrl) {
        this.httpClient = httpClient;
        this.runnerBaseUrl = runnerBaseUrl;
    }
    async loadHealth() {
        const response = await this.httpClient.request(new URL("/health", this.runnerBaseUrl), { cache: "no-store" });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        const payload = (await response.json());
        return {
            cwd: payload.cwd ?? null,
            os: payload.os ?? null,
            startedAt: payload.startedAt ?? null,
            version: payload.version ?? null,
        };
    }
    async loadCompatConfig() {
        const response = await this.httpClient.request(new URL("/compat-config", this.runnerBaseUrl), { cache: "no-store" });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return (await response.json());
    }
    async saveCompatConfig(input) {
        const response = await this.httpClient.request(new URL("/compat-config", this.runnerBaseUrl), {
            method: "PUT",
            cache: "no-store",
            headers: { "content-type": "application/json" },
            body: JSON.stringify(input),
        });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return (await response.json());
    }
    async loadCompatInfo() {
        const response = await this.httpClient.request(new URL("/compat", this.runnerBaseUrl), { cache: "no-store" });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return (await response.json());
    }
    async runCompatCheck() {
        const response = await this.httpClient.request(new URL("/compat", this.runnerBaseUrl), { method: "POST", cache: "no-store" });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return (await response.json());
    }
    async runCompatDeepCheck() {
        const response = await this.httpClient.request(new URL("/compat/deep", this.runnerBaseUrl), { method: "POST", cache: "no-store" });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return (await response.json());
    }
}
exports.HttpRunnerRepository = HttpRunnerRepository;
