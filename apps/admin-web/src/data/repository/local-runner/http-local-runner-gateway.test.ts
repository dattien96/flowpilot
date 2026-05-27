import { describe, it, expect, vi, beforeEach } from "vitest";
import { HttpLocalRunnerGateway } from "./http-local-runner-gateway";
import { LocalRunnerError } from "./http-local-runner-gateway";

describe("HttpLocalRunnerGateway", () => {
  let gateway: HttpLocalRunnerGateway;
  
  beforeEach(() => {
    gateway = new HttpLocalRunnerGateway("http://localhost:8080");
  });

  describe("sendMessage", () => {
    it("parses structured session_dead error into LocalRunnerError", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: () => Promise.resolve({
          code: "session_dead",
          message: "session process exited or is no longer registered",
          details: "session_dead: session not found"
        })
      });
      global.fetch = mockFetch;

      const req = {
        session: { transportType: "codex_mcp", providerSessionId: "thread-old", processKey: "proc-old" },
        prompt: "hello",
        skillIds: [],
        contextSourceIds: []
      };

      try {
        await gateway.sendMessage(req);
        expect.fail("Should have thrown");
      } catch (err: any) {
        expect(err).toBeInstanceOf(LocalRunnerError);
        expect(err.code).toBe("session_dead");
        expect(err.message).toBe("session process exited or is no longer registered");
        expect(err.details).toBe("session_dead: session not found");
      }
    });
  });
});
