import { proxyRunnerJSON } from "../_shared";

export async function POST(request: Request) {
  const payload = await request.json();
  return proxyRunnerJSON("/supabase-config/validate", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
  });
}
