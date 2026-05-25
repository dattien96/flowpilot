import { NextResponse } from "next/server";
import { z } from "zod";

import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

const authSchema = z.object({
  providerName: z.string().min(1),
});

export async function POST(request: Request) {
  try {
    const payload = authSchema.parse(await request.json());
    const response = await fetch(new URL("/providers/auth", getLocalRunnerBaseUrl()), {
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
          error: text || `Provider auth failed with status ${response.status}.`,
        },
        { status: response.status },
      );
    }

    return NextResponse.json({ status: "success" });
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Provider auth failed.",
      },
      { status: 400 },
    );
  }
}
