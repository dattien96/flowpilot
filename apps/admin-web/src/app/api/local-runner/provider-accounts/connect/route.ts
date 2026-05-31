import { NextResponse } from "next/server";
import { z } from "zod";

import {
  localRunnerRequest,
  localRunnerSupportsProviderRegistry,
  readRunnerError,
} from "../_shared";

const connectSchema = z.object({
  providerKey: z.string().min(1),
});

export async function POST(request: Request) {
  try {
    const payload = connectSchema.parse(await request.json());
    const supportsProviderRegistry = await localRunnerSupportsProviderRegistry();
    if (!supportsProviderRegistry) {
      return NextResponse.json(
        {
          error:
            "Your local runner is still on the legacy provider-account build. Restart the local runner before connecting a new account, otherwise it may reuse the default auth path instead of creating an isolated .codexHomeN slot.",
        },
        { status: 400 },
      );
    }

    const response = await localRunnerRequest("/provider-accounts/connect", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        providerKey: payload.providerKey,
      }),
    });

    if (!response.ok) {
      return NextResponse.json(
        {
          error: await readRunnerError(response),
        },
        { status: response.status },
      );
    }

    return NextResponse.json(await response.json());
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Connect failed.",
      },
      { status: 400 },
    );
  }
}
