export const workflowCorsHeaders = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers": "authorization, x-client-info, apikey, content-type",
  "Access-Control-Allow-Methods": "POST, OPTIONS",
};

export function withWorkflowCorsHeaders(init?: ResponseInit): ResponseInit {
  return {
    ...init,
    headers: {
      ...workflowCorsHeaders,
      ...(init?.headers ?? {}),
    },
  };
}

export function workflowCorsPreflightResponse() {
  return new Response("ok", withWorkflowCorsHeaders({ status: 200 }));
}

export function workflowTextResponse(body: string, init?: ResponseInit) {
  return new Response(body, withWorkflowCorsHeaders(init));
}

export function workflowJsonResponse(body: unknown, init?: ResponseInit) {
  return Response.json(body, withWorkflowCorsHeaders(init));
}
