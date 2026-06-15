type BridgeHttpRequest = {
  url: string;
  method?: string;
  headers?: Record<string, string>;
  body?: string;
};

type BridgeHttpResponse = {
  status: number;
  headers: Array<[string, string]>;
  body: string;
};

type FlowpilotHttpBridge = {
  requestHttp(request: BridgeHttpRequest): Promise<BridgeHttpResponse>;
};

function getFlowpilotHttpBridge(): FlowpilotHttpBridge | undefined {
  const candidate = globalThis as typeof globalThis & {
    window?: { flowpilot?: FlowpilotHttpBridge };
  };
  return candidate.window?.flowpilot;
}

export const desktopBridgeFetch: typeof fetch = async (input, init) => {
  const request = new Request(input, init);
  const bridge = getFlowpilotHttpBridge();
  if (!bridge?.requestHttp) {
    return fetch(request);
  }

  const headers = Object.fromEntries(request.headers.entries());
  const body =
    request.method === "GET" || request.method === "HEAD"
      ? undefined
      : await request.text();

  const response = await bridge.requestHttp({
    url: request.url,
    method: request.method,
    headers,
    body,
  });

  const nullBodyStatuses = new Set([101, 103, 204, 205, 304]);
  return new Response(nullBodyStatuses.has(response.status) ? null : response.body, {
    status: response.status,
    headers: response.headers,
  });
};
