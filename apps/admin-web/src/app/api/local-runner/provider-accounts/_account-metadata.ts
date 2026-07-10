import fs from "node:fs";
import { execFileSync } from "node:child_process";
import path from "node:path";

type ProviderKey = "codex" | "claude" | "gemini" | "grok";

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

export type EnrichedProviderAccountRow = ProviderAccountRow & {
  auth_store_path: string | null;
  account_email: string | null;
  account_name: string | null;
  usage_summary: string | null;
  remaining_5h_percent: number | null;
  remaining_7d_percent: number | null;
  remaining_5h_reset_at: string | null;
  remaining_7d_reset_at: string | null;
  usage_source: "provider_api" | "unavailable";
  access_token_expires_at: string | null;
  refresh_token_expires_at: string | null;
  refresh_token_expiry_note: string | null;
  usage_detail_lines: Array<{
    label: string;
    remaining_percent: number;
    reset_at: string | null;
  }>;
  display_label: string;
};

type AccountMetadata = {
  authStorePath: string | null;
  accountEmail: string | null;
  accountName: string | null;
  usageSummary: string | null;
  remaining5hPercent: number | null;
  remaining7dPercent: number | null;
  remaining5hResetAt: string | null;
  remaining7dResetAt: string | null;
  usageSource: "provider_api" | "unavailable";
  accessTokenExpiresAt: string | null;
  refreshTokenExpiresAt: string | null;
  refreshTokenExpiryNote: string | null;
  usageDetailLines: Array<{
    label: string;
    remainingPercent: number;
    resetAt: string | null;
  }>;
};

type GeminiQuotaBucket = {
  modelId?: string | null;
  remainingFraction?: number | null;
  resetTime?: string | null;
};

type ClaudeAuthStatus = {
  loggedIn?: boolean;
  email?: string | null;
  orgName?: string | null;
  subscriptionType?: string | null;
};

type ClaudeOauthAccount = {
  emailAddress?: string | null;
  displayName?: string | null;
  organizationBillingType?: string | null;
  billingType?: string | null;
  hasExtraUsageEnabled?: boolean;
  subscriptionCreatedAt?: string | null;
};

const GEMINI_CONFIG = {
  clientId:
    "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
  clientSecret: "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",
  tokenUrl: "https://oauth2.googleapis.com/token",
  loadCodeAssistUrl:
    "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist",
  retrieveQuotaUrl:
    "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota",
};

function decodeJwtPayload(token: string | null | undefined) {
  const parts = String(token ?? "").split(".");
  if (parts.length < 2) {
    return null;
  }

  try {
    const normalized = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(
      Buffer.from(normalized, "base64").toString("utf8"),
    ) as Record<string, unknown>;
  } catch {
    return null;
  }
}

function firstExistingPath(paths: string[]) {
  for (const candidate of paths) {
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }

  return null;
}

function getOAuthPlatformEnum() {
  switch (process.platform) {
    case "darwin":
      return process.arch === "arm64" ? 2 : 1;
    case "linux":
      return process.arch === "arm64" ? 4 : 3;
    case "win32":
      return 5;
    default:
      return 0;
  }
}

function parseResetTime(resetValue: unknown) {
  if (!resetValue) {
    return null;
  }

  try {
    if (typeof resetValue === "number" && Number.isFinite(resetValue)) {
      return new Date(
        resetValue < 1e12 ? resetValue * 1000 : resetValue,
      ).toISOString();
    }

    if (typeof resetValue === "string" && resetValue.trim().length > 0) {
      if (/^\d+$/.test(resetValue)) {
        const numericValue = Number(resetValue);
        return new Date(
          numericValue < 1e12 ? numericValue * 1000 : numericValue,
        ).toISOString();
      }
      return new Date(resetValue).toISOString();
    }
  } catch {
    return null;
  }

  return null;
}

function parseJwtExpiry(token: string | null | undefined) {
  const payload = decodeJwtPayload(token);
  if (typeof payload?.exp !== "number" || !Number.isFinite(payload.exp)) {
    return null;
  }

  try {
    return new Date(payload.exp * 1000).toISOString();
  } catch {
    return null;
  }
}

function humanizeDelimitedLabel(value: string | null | undefined) {
  const trimmed = String(value ?? "").trim();
  if (trimmed.length === 0) {
    return null;
  }

  return trimmed
    .split(/[_\s-]+/)
    .filter((part) => part.length > 0)
    .map((part) => part[0]!.toUpperCase() + part.slice(1))
    .join(" ");
}

function loadClaudeAuthStatus(homePath: string): ClaudeAuthStatus | null {
  const command = process.platform === "win32" ? "claude.cmd" : "claude";

  try {
    const raw = execFileSync(command, ["auth", "status", "--json"], {
      cwd: homePath,
      encoding: "utf8",
      env: {
        ...process.env,
        HOME: homePath,
        USERPROFILE: homePath,
      },
      stdio: ["ignore", "pipe", "ignore"],
    });
    const payload = JSON.parse(raw) as ClaudeAuthStatus;
    return payload.loggedIn ? payload : null;
  } catch {
    return null;
  }
}

export function buildClaudeUsageSummary(input: {
  account?: ClaudeOauthAccount | null;
  authStatus?: ClaudeAuthStatus | null;
  extraUsageDisabledReason?: string | null;
}) {
  const account = input.account ?? null;
  const authStatus = input.authStatus ?? null;
  const rawPlan =
    authStatus?.subscriptionType ??
    account?.organizationBillingType ??
    account?.billingType ??
    null;
  const rawDisabledReason = String(input.extraUsageDisabledReason ?? "").trim();
  const disabledReason =
    rawDisabledReason.length > 0
      ? humanizeDelimitedLabel(rawDisabledReason)?.toLowerCase() ?? null
      : null;

  let summary = humanizeDelimitedLabel(rawPlan);
  if (
    summary &&
    typeof account?.subscriptionCreatedAt === "string" &&
    account.subscriptionCreatedAt.length >= 10
  ) {
    summary += ` since ${account.subscriptionCreatedAt.slice(0, 10)}`;
  }

  if (disabledReason) {
    return summary
      ? `${summary} | extra usage unavailable (${disabledReason})`
      : `Extra usage unavailable (${disabledReason})`;
  }

  if (account?.hasExtraUsageEnabled) {
    return summary ? `${summary} | extra usage enabled` : "Extra usage enabled";
  }

  return summary;
}

function codexQuotaFromWindow(window: unknown) {
  if (!window || typeof window !== "object" || Array.isArray(window)) {
    return null;
  }

  const windowRecord = window as Record<string, unknown>;
  const usedPercentRaw =
    typeof windowRecord.used_percent === "number"
      ? windowRecord.used_percent
      : typeof windowRecord.percent_used === "number"
        ? windowRecord.percent_used
        : null;
  const windowSeconds =
    typeof windowRecord.limit_window_seconds === "number"
      ? windowRecord.limit_window_seconds
      : null;

  if (usedPercentRaw === null || windowSeconds === null) {
    return null;
  }

  return {
    windowSeconds,
    remainingPercent: Math.max(0, Math.min(100, 100 - usedPercentRaw)),
    resetAt: parseResetTime(
      windowRecord.reset_at ?? windowRecord.resets_at ?? null,
    ),
  };
}

async function codexMetadata(homePath: string): Promise<AccountMetadata> {
  const authPath = firstExistingPath([
    path.join(homePath, "auth.json"),
    path.join(homePath, ".codex", "auth.json"),
    path.join(homePath, "codex", "auth.json"),
  ]);
  if (!authPath) {
    return {
      authStorePath: homePath,
      accountEmail: null,
      accountName: null,
      usageSummary: null,
      remaining5hPercent: null,
      remaining7dPercent: null,
      remaining5hResetAt: null,
      remaining7dResetAt: null,
      usageSource: "unavailable",
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
      usageDetailLines: [],
    };
  }

  try {
    const auth = JSON.parse(fs.readFileSync(authPath, "utf8")) as {
      tokens?: {
        id_token?: string | null;
        access_token?: string | null;
      };
    };
    const payload = decodeJwtPayload(auth.tokens?.id_token);
    const openAiAuth =
      payload?.["https://api.openai.com/auth"] &&
      typeof payload["https://api.openai.com/auth"] === "object"
        ? (payload["https://api.openai.com/auth"] as Record<string, unknown>)
        : null;

    let usageSummary: string | null = null;
    const planType =
      typeof openAiAuth?.chatgpt_plan_type === "string"
        ? openAiAuth.chatgpt_plan_type
        : null;
    const activeUntil =
      typeof openAiAuth?.chatgpt_subscription_active_until === "string"
        ? openAiAuth.chatgpt_subscription_active_until
        : null;
    if (planType && activeUntil) {
      usageSummary = `${planType} until ${activeUntil.slice(0, 10)}`;
    } else if (planType) {
      usageSummary = planType;
    }

    let remaining5hPercent: number | null = null;
    let remaining7dPercent: number | null = null;
    let remaining5hResetAt: string | null = null;
    let remaining7dResetAt: string | null = null;
    let usageSource: "provider_api" | "unavailable" = "unavailable";
    const accessTokenExpiresAt = parseJwtExpiry(auth.tokens?.access_token);
    const usageDetailLines: AccountMetadata["usageDetailLines"] = [];

    if (
      typeof auth.tokens?.access_token === "string" &&
      auth.tokens.access_token
    ) {
      try {
        const response = await fetch(
          "https://chatgpt.com/backend-api/wham/usage",
          {
            method: "GET",
            headers: {
              Authorization: `Bearer ${auth.tokens.access_token}`,
              Accept: "application/json",
            },
            cache: "no-store",
          },
        );

        if (response.ok) {
          const usage = (await response.json()) as {
            rate_limit?: {
              primary_window?: unknown;
              secondary_window?: unknown;
            };
          };
          const primaryQuota = codexQuotaFromWindow(
            usage.rate_limit?.primary_window,
          );
          const secondaryQuota = codexQuotaFromWindow(
            usage.rate_limit?.secondary_window,
          );

          for (const quota of [primaryQuota, secondaryQuota]) {
            if (!quota) {
              continue;
            }
            if (quota.windowSeconds === 18000) {
              remaining5hPercent = quota.remainingPercent;
              remaining5hResetAt = quota.resetAt;
              usageDetailLines.push({
                label: "Remaining 5h",
                remainingPercent: quota.remainingPercent,
                resetAt: quota.resetAt,
              });
            } else if (quota.windowSeconds === 604800) {
              remaining7dPercent = quota.remainingPercent;
              remaining7dResetAt = quota.resetAt;
              usageDetailLines.push({
                label: "Remaining 7d",
                remainingPercent: quota.remainingPercent,
                resetAt: quota.resetAt,
              });
            }
          }

          if (remaining5hPercent !== null || remaining7dPercent !== null) {
            usageSource = "provider_api";
          }
        }
      } catch {
        usageSource = "unavailable";
      }
    }

    return {
      authStorePath: path.dirname(authPath),
      accountEmail: typeof payload?.email === "string" ? payload.email : null,
      accountName: typeof payload?.name === "string" ? payload.name : null,
      usageSummary,
      remaining5hPercent,
      remaining7dPercent,
      remaining5hResetAt,
      remaining7dResetAt,
      usageSource,
      accessTokenExpiresAt,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote:
        "Unknown. Codex refresh token expiry is not exposed in local auth data; re-login is required only when a refresh attempt fails.",
      usageDetailLines,
    };
  } catch {
    return {
      authStorePath: path.dirname(authPath),
      accountEmail: null,
      accountName: null,
      usageSummary: null,
      remaining5hPercent: null,
      remaining7dPercent: null,
      remaining5hResetAt: null,
      remaining7dResetAt: null,
      usageSource: "unavailable",
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
      usageDetailLines: [],
    };
  }
}

function claudeMetadata(homePath: string): AccountMetadata {
  const authPath = firstExistingPath([
    path.join(homePath, ".claude.json"),
    path.join(homePath, "claude", "auth.json"),
    path.join(homePath, ".config", "claude", "auth.json"),
  ]);
  if (!authPath) {
    return {
      authStorePath: path.join(homePath, ".claude"),
      accountEmail: null,
      accountName: null,
      usageSummary: null,
      remaining5hPercent: null,
      remaining7dPercent: null,
      remaining5hResetAt: null,
      remaining7dResetAt: null,
      usageSource: "unavailable",
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
      usageDetailLines: [],
    };
  }

  try {
    const auth = JSON.parse(fs.readFileSync(authPath, "utf8")) as {
      oauthAccount?: ClaudeOauthAccount;
      cachedExtraUsageDisabledReason?: string | null;
    };
    const authStatus = loadClaudeAuthStatus(homePath);
    const account = auth.oauthAccount;
    const usageSummary =
      buildClaudeUsageSummary({
        account,
        authStatus,
        extraUsageDisabledReason: auth.cachedExtraUsageDisabledReason,
      }) ?? null;

    return {
      authStorePath: path.dirname(authPath),
      accountEmail: authStatus?.email ?? account?.emailAddress ?? null,
      accountName: account?.displayName ?? null,
      usageSummary,
      remaining5hPercent: null,
      remaining7dPercent: null,
      remaining5hResetAt: null,
      remaining7dResetAt: null,
      usageSource: "unavailable",
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
      usageDetailLines: [],
    };
  } catch {
    return {
      authStorePath: path.join(homePath, ".claude"),
      accountEmail: null,
      accountName: null,
      usageSummary: null,
      remaining5hPercent: null,
      remaining7dPercent: null,
      remaining5hResetAt: null,
      remaining7dResetAt: null,
      usageSource: "unavailable",
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
      usageDetailLines: [],
    };
  }
}

async function refreshGeminiAccessToken(refreshToken: string) {
  const response = await fetch(GEMINI_CONFIG.tokenUrl, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Accept: "application/json",
    },
    body: new URLSearchParams({
      grant_type: "refresh_token",
      refresh_token: refreshToken,
      client_id: GEMINI_CONFIG.clientId,
      client_secret: GEMINI_CONFIG.clientSecret,
    }),
    cache: "no-store",
  });

  if (!response.ok) {
    return null;
  }

  const payload = (await response.json()) as { access_token?: string | null };
  return typeof payload.access_token === "string" && payload.access_token
    ? payload.access_token
    : null;
}

async function loadGeminiQuota(accessToken: string) {
  const metadata = {
    ideType: 9,
    platform: getOAuthPlatformEnum(),
    pluginType: 2,
  };
  const headers = {
    Authorization: `Bearer ${accessToken}`,
    "Content-Type": "application/json",
    "User-Agent": "google-api-nodejs-client/9.15.1",
    "X-Goog-Api-Client": "google-cloud-sdk vscode_cloudshelleditor/0.1",
    "Client-Metadata": JSON.stringify(metadata),
  };

  const loadResponse = await fetch(GEMINI_CONFIG.loadCodeAssistUrl, {
    method: "POST",
    headers,
    body: JSON.stringify({ metadata }),
    cache: "no-store",
  });
  if (!loadResponse.ok) {
    return null;
  }

  const loadPayload = (await loadResponse.json()) as {
    currentTier?: { name?: string | null };
    cloudaicompanionProject?: string | { id?: string | null } | null;
  };
  const projectId =
    typeof loadPayload.cloudaicompanionProject === "string"
      ? loadPayload.cloudaicompanionProject
      : (loadPayload.cloudaicompanionProject?.id ?? null);

  if (!projectId) {
    return {
      usageSummary: loadPayload.currentTier?.name ?? null,
      usageDetailLines: [] as AccountMetadata["usageDetailLines"],
      usageSource: "unavailable" as const,
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
    };
  }

  const quotaResponse = await fetch(GEMINI_CONFIG.retrieveQuotaUrl, {
    method: "POST",
    headers,
    body: JSON.stringify({ project: projectId }),
    cache: "no-store",
  });
  if (!quotaResponse.ok) {
    return {
      usageSummary: loadPayload.currentTier?.name ?? null,
      usageDetailLines: [] as AccountMetadata["usageDetailLines"],
      usageSource: "unavailable" as const,
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
    };
  }

  const quotaPayload = (await quotaResponse.json()) as {
    buckets?: GeminiQuotaBucket[];
  };

  const usageDetailLines = mapGeminiQuotaUsageDetailLines(quotaPayload.buckets);

  return {
    usageSummary: loadPayload.currentTier?.name ?? null,
    usageDetailLines,
    usageSource:
      usageDetailLines.length > 0
        ? ("provider_api" as const)
        : ("unavailable" as const),
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
  };
}

export function mapGeminiQuotaUsageDetailLines(
  buckets: GeminiQuotaBucket[] | null | undefined,
): AccountMetadata["usageDetailLines"] {
  return (buckets ?? [])
    .filter(
      (bucket) =>
        typeof bucket.modelId === "string" &&
        typeof bucket.remainingFraction === "number",
    )
    .sort((a, b) => String(a.modelId).localeCompare(String(b.modelId)))
    .map((bucket) => ({
      label: bucket.modelId as string,
      remainingPercent: Math.max(
        0,
        Math.min(100, Math.round((bucket.remainingFraction as number) * 100)),
      ),
      resetAt: parseResetTime(bucket.resetTime ?? null),
    }));
}

async function geminiMetadata(homePath: string): Promise<AccountMetadata> {
  const authPath = firstExistingPath([
    path.join(homePath, ".gemini", "oauth_creds.json"),
    path.join(homePath, "gemini", "oauth_creds.json"),
    path.join(homePath, "oauth_creds.json"),
  ]);
  const googleAccountsPath = firstExistingPath([
    path.join(homePath, ".gemini", "google_accounts.json"),
    path.join(homePath, "google_accounts.json"),
  ]);

  let accountEmail: string | null = null;
  let accountName: string | null = null;
  if (googleAccountsPath) {
    try {
      const accounts = JSON.parse(
        fs.readFileSync(googleAccountsPath, "utf8"),
      ) as {
        active?: string | null;
      };
      accountEmail = accounts.active ?? null;
    } catch {
      accountEmail = null;
    }
  }

  if (authPath) {
    try {
      const auth = JSON.parse(fs.readFileSync(authPath, "utf8")) as {
        id_token?: string | null;
        refresh_token?: string | null;
      };
      const payload = decodeJwtPayload(auth.id_token);
      if (!accountEmail && typeof payload?.email === "string") {
        accountEmail = payload.email;
      }
      if (typeof payload?.name === "string") {
        accountName = payload.name;
      }

      let usageSummary: string | null = null;
      let usageDetailLines: AccountMetadata["usageDetailLines"] = [];
      let usageSource: "provider_api" | "unavailable" = "unavailable";

      if (
        typeof auth.refresh_token === "string" &&
        auth.refresh_token.trim().length > 0
      ) {
        try {
          const accessToken = await refreshGeminiAccessToken(
            auth.refresh_token,
          );
          if (accessToken) {
            const quota = await loadGeminiQuota(accessToken);
            usageSummary = quota?.usageSummary ?? null;
            usageDetailLines = quota?.usageDetailLines ?? [];
            usageSource = quota?.usageSource ?? "unavailable";
          }
        } catch {
          usageSource = "unavailable";
        }
      }

      return {
        authStorePath: path.join(homePath, ".gemini"),
        accountEmail,
        accountName,
        usageSummary,
        remaining5hPercent: null,
        remaining7dPercent: null,
        remaining5hResetAt: null,
        remaining7dResetAt: null,
        usageSource,
        accessTokenExpiresAt: null,
        refreshTokenExpiresAt: null,
        refreshTokenExpiryNote: null,
        usageDetailLines,
      };
    } catch {
      // Ignore parse errors and keep partial metadata.
    }
  }

  return {
    authStorePath: path.join(homePath, ".gemini"),
    accountEmail,
    accountName,
    usageSummary: null,
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "unavailable",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
  };
}

// grokAuthEntry mirrors one value in ~/.grok/auth.json (live-verified shape,
// CP-46/Task-210 authoring). Task-216 corrects CP-46 Q-5/Task-210 Q-1's "no
// machine-readable quota endpoint" finding: `key` is the same cached OAuth
// bearer token Grok Build's own `/usage` TUI command uses, and
// loadGrokQuota below calls the real billing endpoint behind it
// (live-verified 2026-07-10).
type GrokAuthEntry = {
  email?: string | null;
  first_name?: string | null;
  last_name?: string | null;
  team_id?: string | null;
  key?: string | null;
};

// GROK_CLI_CHAT_PROXY_BASE_URL is Grok Build's own backend base URL
// (live-verified string in the installed `grok` binary, Task-216).
const GROK_CLI_CHAT_PROXY_BASE_URL = "https://cli-chat-proxy.grok.com/v1";

type GrokBillingValue = {
  val?: number | null;
};

// GrokBillingConfig mirrors GET {GROK_CLI_CHAT_PROXY_BASE_URL}/billing's
// `config` object (live-verified response shape, Task-216). Grok bills
// either a monthly or a weekly cycle depending on plan; only one of
// monthlyLimit/weeklyLimit is expected to be present on any given account.
type GrokBillingConfig = {
  monthlyLimit?: GrokBillingValue | null;
  weeklyLimit?: GrokBillingValue | null;
  used?: GrokBillingValue | null;
  billingPeriodEnd?: string | null;
};

type GrokBillingResponse = {
  config?: GrokBillingConfig | null;
};

// grokQuotaFromBilling is the pure mapping half of loadGrokQuota, split out
// so tests can exercise it against a captured response fixture without a
// live network call (mirrors mapGeminiQuotaUsageDetailLines's split).
//
// Labeled "Team Credits", not "Weekly limit": this endpoint reports the
// account's shared billing/credit pool (Task-216). Live-testing against a
// SuperGrok account (2026-07-10) showed the CLI's own `/usage`-style status
// line ("Weekly limit: 1%", resets weekly) is a DIFFERENT, still-unlocated
// metric — the personal SuperGrok included-usage allowance — not derivable
// from this /billing response. Do not rename this label to "Weekly limit"
// until that second endpoint is found and confirmed (tracked as a Task-216
// follow-up); doing so would misrepresent which quota is being shown.
export function grokQuotaFromBilling(
  billing: GrokBillingResponse | null | undefined,
): AccountMetadata["usageDetailLines"][number] | null {
  const config = billing?.config;
  if (!config) {
    return null;
  }

  let limit = config.monthlyLimit;
  let label = "Team Credits (Monthly)";
  if (!limit) {
    limit = config.weeklyLimit;
    label = "Team Credits (Weekly)";
  }

  const limitVal = limit?.val;
  const usedVal = config.used?.val;
  if (typeof limitVal !== "number" || limitVal <= 0 || typeof usedVal !== "number") {
    return null;
  }

  return {
    label,
    remainingPercent: Math.max(
      0,
      Math.min(100, Math.trunc(100 - (usedVal / limitVal) * 100)),
    ),
    resetAt: parseResetTime(config.billingPeriodEnd ?? null),
  };
}

async function loadGrokQuota(bearerToken: string | null | undefined) {
  const token = String(bearerToken ?? "").trim();
  if (!token) {
    return null;
  }

  try {
    const response = await fetch(`${GROK_CLI_CHAT_PROXY_BASE_URL}/billing`, {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: "application/json",
      },
      cache: "no-store",
    });
    if (!response.ok) {
      return null;
    }
    const billing = (await response.json()) as GrokBillingResponse;
    return grokQuotaFromBilling(billing);
  } catch {
    return null;
  }
}

async function grokMetadata(homePath: string): Promise<AccountMetadata> {
  const authPath = firstExistingPath([path.join(homePath, "auth.json")]);
  const base: AccountMetadata = {
    authStorePath: homePath,
    accountEmail: null,
    accountName: null,
    usageSummary: null,
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "unavailable",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
  };
  if (!authPath) {
    return base;
  }

  try {
    const raw = JSON.parse(fs.readFileSync(authPath, "utf8")) as Record<
      string,
      GrokAuthEntry
    >;
    const entry = Object.values(raw)[0];
    if (!entry) {
      return base;
    }

    const accountName = `${entry.first_name ?? ""} ${entry.last_name ?? ""}`.trim();
    const usageSummary = entry.team_id ? `Team ${entry.team_id}` : "Personal";
    const quotaLine = await loadGrokQuota(entry.key);

    return {
      ...base,
      accountEmail: entry.email ?? null,
      accountName: accountName.length > 0 ? accountName : null,
      usageSummary,
      usageSource: quotaLine ? "provider_api" : "unavailable",
      usageDetailLines: quotaLine ? [quotaLine] : [],
    };
  } catch {
    return base;
  }
}

function metadataForAccount(providerKey: ProviderKey, homePath: string) {
  switch (providerKey) {
    case "codex":
      return codexMetadata(homePath);
    case "claude":
      return claudeMetadata(homePath);
    case "gemini":
      return geminiMetadata(homePath);
    case "grok":
      return grokMetadata(homePath);
  }
}

export async function enrichProviderAccounts(accounts: ProviderAccountRow[]) {
  return Promise.all(
    accounts.map(async (account) => {
      const metadata = await metadataForAccount(
        account.provider_key as ProviderKey,
        account.home_path,
      );
      const displayLabel =
        metadata.accountEmail ?? metadata.accountName ?? account.display_name;

      return {
        ...account,
        auth_store_path: metadata.authStorePath,
        account_email: metadata.accountEmail,
        account_name: metadata.accountName,
        usage_summary: metadata.usageSummary,
        remaining_5h_percent: metadata.remaining5hPercent,
        remaining_7d_percent: metadata.remaining7dPercent,
        remaining_5h_reset_at: metadata.remaining5hResetAt,
        remaining_7d_reset_at: metadata.remaining7dResetAt,
        usage_source: metadata.usageSource,
        access_token_expires_at: metadata.accessTokenExpiresAt,
        refresh_token_expires_at: metadata.refreshTokenExpiresAt,
        refresh_token_expiry_note: metadata.refreshTokenExpiryNote,
        usage_detail_lines: metadata.usageDetailLines.map((line) => ({
          label: line.label,
          remaining_percent: line.remainingPercent,
          reset_at: line.resetAt,
        })),
        display_label: displayLabel,
      } satisfies EnrichedProviderAccountRow;
    }),
  );
}
