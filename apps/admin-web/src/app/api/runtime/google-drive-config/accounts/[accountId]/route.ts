import { NextResponse } from "next/server";

import { localRunnerRequest, readRunnerError } from "../../_shared";

type RouteContext = {
  params: Promise<{
    accountId: string;
  }>;
};

export async function DELETE(_request: Request, context: RouteContext) {
  try {
    const { accountId } = await context.params;
    if (!accountId.trim()) {
      return NextResponse.json({ error: "accountId is required." }, { status: 400 });
    }

    const response = await localRunnerRequest(
      `/google-drive-config/accounts/${encodeURIComponent(accountId)}`,
      { method: "DELETE" },
    );
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
        error: error instanceof Error ? error.message : "Unable to disconnect Google Drive account.",
      },
      { status: 400 },
    );
  }
}
