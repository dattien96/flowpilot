import { NextResponse } from "next/server";
import { z } from "zod";

import {
  localRunnerRequest,
  parseLegacyAccountId,
  readRunnerError,
} from "../_shared";

const activateSchema = z.object({
  accountId: z.string().min(1),
});

export async function POST(request: Request) {
  try {
    const payload = activateSchema.parse(await request.json());
    const legacyAccount = parseLegacyAccountId(payload.accountId);
    if (legacyAccount) {
      if (legacyAccount.slotIndex === 0) {
        return NextResponse.json({
          account: {
            id: payload.accountId,
            provider_key: legacyAccount.providerKey,
            home_path: legacyAccount.homePath,
            slot_index: legacyAccount.slotIndex,
            is_active: true,
            auth_status: "connected",
          },
        });
      }

      return NextResponse.json(
        {
          error:
            "This runner build does not support persisting active selection for scanned legacy accounts. Restart the local runner to enable it.",
        },
        { status: 400 },
      );
    }

    const response = await localRunnerRequest("/provider-accounts/activate", {
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
        error: error instanceof Error ? error.message : "Activate failed.",
      },
      { status: 400 },
    );
  }
}
