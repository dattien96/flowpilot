import { NextResponse } from "next/server";
import { z } from "zod";

import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";
import { mapProvider, type RawProvider } from "@/data/repository/local-runner/local-runner-mappers";

const installSchema = z.object({
  providerName: z.string().min(1),
});

export async function POST(request: Request) {
  try {
    const payload = installSchema.parse(await request.json());
    const response = await fetch(new URL("/providers/install", getLocalRunnerBaseUrl()), {
      method: "POST",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(payload),
      cache: "no-store",
    });

    const text = await response.text();
    if (!response.ok) {
      return NextResponse.json(
        {
          error: text || `Provider install failed with status ${response.status}.`,
        },
        { status: response.status },
      );
    }

    if (text) {
      const data = JSON.parse(text) as { providers: RawProvider[] };
      return NextResponse.json({
        providers: data.providers.map(mapProvider),
      });
    }

    return NextResponse.json({});
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Provider install failed.",
      },
      { status: 400 },
    );
  }
}
