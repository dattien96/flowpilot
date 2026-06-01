import { NextResponse } from "next/server";
import { z } from "zod";

import { localRunnerRequest, readRunnerError } from "../_shared";

const testSchema = z.object({
  accountId: z.string().min(1),
});

export async function POST(request: Request) {
  try {
    const payload = testSchema.parse(await request.json());
    const response = await localRunnerRequest("/provider-accounts/test", {
      method: "POST",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({
        accountId: payload.accountId,
      }),
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: await readRunnerError(response) },
        { status: response.status },
      );
    }

    return NextResponse.json(await response.json());
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Test failed.",
      },
      { status: 400 },
    );
  }
}
