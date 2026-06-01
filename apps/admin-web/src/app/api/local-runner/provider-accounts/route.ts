import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { NextResponse } from "next/server";

import { enrichProviderAccounts } from "./_account-metadata";
import { localRunnerRequest, readRunnerError } from "./_shared";

type ProviderKey = "codex" | "claude" | "gemini";

type ProviderAccountRow = {
  id: string;
  provider_key: string;
  display_name: string;
  home_path: string;
  slot_index: number;
  is_active: boolean;
  auth_status: "pending" | "connecting" | "connected" | "failed";
  created_at: string;
  last_authenticated_at: string | null;
};

type LegacyDefaultsPayload = {
  defaults?: Array<{
    providerKey?: string;
    homePath?: string;
  }>;
};

const SUPPORTED_PROVIDER_KEYS: ProviderKey[] = ["codex", "claude", "gemini"];

export async function GET() {
  try {
    const response = await localRunnerRequest("/provider-accounts");
    if (response.status === 404) {
      const accounts = await buildLegacyAccounts();
      return NextResponse.json({
        accounts: await enrichProviderAccounts(accounts),
      });
    }

    if (!response.ok) {
      return NextResponse.json(
        { error: await readRunnerError(response) },
        { status: response.status },
      );
    }

    const payload = (await response.json()) as {
      accounts?: ProviderAccountRow[];
    };
    const accounts = (payload.accounts ?? []) as ProviderAccountRow[];

    return NextResponse.json({
      accounts: await enrichProviderAccounts(accounts),
    });
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Fetch failed.",
      },
      { status: 400 },
    );
  }
}

async function buildLegacyAccounts() {
  const response = await localRunnerRequest("/provider-accounts/defaults");
  if (!response.ok) {
    throw new Error(await readRunnerError(response));
  }

  const payload = (await response.json()) as LegacyDefaultsPayload;
  const defaults = new Map<ProviderKey, string>();
  for (const entry of payload.defaults ?? []) {
    const providerKey = entry.providerKey;
    const homePath = entry.homePath;
    if (
      providerKey === "codex" ||
      providerKey === "claude" ||
      providerKey === "gemini"
    ) {
      if (typeof homePath === "string" && homePath.trim().length > 0) {
        defaults.set(providerKey, homePath);
      }
    }
  }

  const now = new Date().toISOString();
  const accounts: ProviderAccountRow[] = [];
  const homeDir = os.homedir();

  for (const providerKey of SUPPORTED_PROVIDER_KEYS) {
    const defaultHomePath = defaults.get(providerKey);
    if (defaultHomePath) {
      accounts.push(
        createLegacyAccountRow(providerKey, 0, defaultHomePath, {
          displayName: "Default Account",
          isActive: true,
          createdAt: now,
        }),
      );
    }

    const prefix = managedProviderPrefix(providerKey);
    for (let slotIndex = 1; slotIndex < 1000; slotIndex += 1) {
      const slotHomePath = path.join(homeDir, `${prefix}${slotIndex}`);
      if (!hasLegacyLocalAuth(providerKey, slotHomePath)) {
        continue;
      }

      accounts.push(
        createLegacyAccountRow(providerKey, slotIndex, slotHomePath, {
          displayName: `Account ${slotIndex}`,
          isActive: false,
          createdAt: authModifiedAt(providerKey, slotHomePath) ?? now,
        }),
      );
    }
  }

  return accounts;
}

function createLegacyAccountRow(
  providerKey: ProviderKey,
  slotIndex: number,
  homePath: string,
  options: {
    displayName: string;
    isActive: boolean;
    createdAt: string;
  },
): ProviderAccountRow {
  return {
    id: legacyAccountId(providerKey, slotIndex, homePath),
    provider_key: providerKey,
    display_name: options.displayName,
    home_path: homePath,
    slot_index: slotIndex,
    is_active: options.isActive,
    auth_status: "connected",
    created_at: options.createdAt,
    last_authenticated_at: authModifiedAt(providerKey, homePath),
  };
}

function legacyAccountId(
  providerKey: ProviderKey,
  slotIndex: number,
  homePath: string,
) {
  return `legacy:${providerKey}:${slotIndex}:${Buffer.from(homePath).toString("base64url")}`;
}

function managedProviderPrefix(providerKey: ProviderKey) {
  switch (providerKey) {
    case "codex":
      return ".codexHome";
    case "claude":
      return ".claudeHome";
    case "gemini":
      return ".geminiHome";
  }
}

function authCandidatePaths(providerKey: ProviderKey, homePath: string) {
  switch (providerKey) {
    case "codex":
      return [
        path.join(homePath, "auth.json"),
        path.join(homePath, ".codex", "auth.json"),
        path.join(homePath, "codex", "auth.json"),
      ];
    case "claude":
      return [
        path.join(homePath, ".claude.json"),
        path.join(homePath, "claude", "auth.json"),
        path.join(homePath, ".config", "claude", "auth.json"),
      ];
    case "gemini":
      return [
        path.join(homePath, ".gemini", "oauth_creds.json"),
        path.join(homePath, "gemini", "oauth_creds.json"),
        path.join(homePath, "oauth_creds.json"),
      ];
  }
}

function hasLegacyLocalAuth(providerKey: ProviderKey, homePath: string) {
  return authCandidatePaths(providerKey, homePath).some((candidate) => {
    if (!fs.existsSync(candidate)) {
      return false;
    }
    try {
      return fs.statSync(candidate).size > 0;
    } catch {
      return false;
    }
  });
}

function authModifiedAt(providerKey: ProviderKey, homePath: string) {
  for (const candidate of authCandidatePaths(providerKey, homePath)) {
    if (!fs.existsSync(candidate)) {
      continue;
    }
    try {
      return fs.statSync(candidate).mtime.toISOString();
    } catch {
      continue;
    }
  }
  return null;
}
