import { NextResponse } from "next/server";

import {
  localRunnerRequest,
  parseLegacyAccountId,
  readRunnerError,
} from "../_shared";

type RouteContext = {
  params: Promise<{
    id: string;
  }>;
};

export async function DELETE(request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const legacyAccount = parseLegacyAccountId(id);
    if (legacyAccount) {
      return NextResponse.json(
        {
          error:
            "This runner build does not support deleting scanned legacy accounts. Restart the local runner to enable it.",
        },
        { status: 400 },
      );
    }

    const response = await localRunnerRequest(`/provider-accounts/${id}`, {
      method: "DELETE",
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: await readRunnerError(response) },
        { status: response.status },
      );
    }

    return new NextResponse(null, { status: 204 });
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Delete failed.",
      },
      { status: 400 },
    );
  }
}
