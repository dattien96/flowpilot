import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

export async function localRunnerRequest(path: string, init?: RequestInit) {
  return fetch(new URL(path, getLocalRunnerBaseUrl()), {
    cache: "no-store",
    ...init,
  });
}

export async function localRunnerSupportsProviderRegistry() {
  const response = await localRunnerRequest("/provider-accounts");
  return response.status !== 404;
}

export async function readRunnerError(response: Response) {
  const text = await response.text();
  if (!text) {
    return `Runner request failed with status ${response.status}.`;
  }

  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error || text;
  } catch {
    return text;
  }
}

type LegacyAccountRef = {
  providerKey: string;
  slotIndex: number;
  homePath: string;
};

export function parseLegacyAccountId(accountId: string): LegacyAccountRef | null {
  if (!accountId.startsWith("legacy:")) {
    return null;
  }

  const parts = accountId.split(":");
  if (parts.length < 4) {
    return null;
  }

  const providerKey = parts[1]?.trim();
  const slotIndex = Number(parts[2]);
  const encodedHomePath = parts.slice(3).join(":");

  if (!providerKey || Number.isNaN(slotIndex) || !encodedHomePath) {
    return null;
  }

  try {
    return {
      providerKey,
      slotIndex,
      homePath: Buffer.from(encodedHomePath, "base64url").toString("utf8"),
    };
  } catch {
    return null;
  }
}
