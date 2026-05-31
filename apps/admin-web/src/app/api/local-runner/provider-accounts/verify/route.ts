import { NextResponse } from "next/server";
import { z } from "zod";

import {
  localRunnerRequest,
  parseLegacyAccountId,
  readRunnerError,
} from "../_shared";

const verifySchema = z.object({
  accountId: z.string().min(1),
});

export async function POST(request: Request) {
  try {
    const payload = verifySchema.parse(await request.json());
    const legacyAccount = parseLegacyAccountId(payload.accountId);
    const response = await localRunnerRequest("/provider-accounts/verify", {
      method: "POST",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({
        ...(legacyAccount
          ? {
              providerKey: legacyAccount.providerKey,
              accountHomePath: legacyAccount.homePath,
            }
          : {
              accountId: payload.accountId,
            }),
      }),
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: await readRunnerError(response) },
        { status: response.status },
      );
    }

    const result = await response.json();
    if (legacyAccount) {
      return NextResponse.json({
        verified: Boolean(result?.verified),
        account: {
          id: payload.accountId,
          provider_key: legacyAccount.providerKey,
          home_path: legacyAccount.homePath,
          slot_index: legacyAccount.slotIndex,
          is_active: legacyAccount.slotIndex === 0,
          auth_status: result?.verified ? "connected" : "failed",
        },
      });
    }

    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Verify failed.",
      },
      { status: 400 },
    );
  }
}
