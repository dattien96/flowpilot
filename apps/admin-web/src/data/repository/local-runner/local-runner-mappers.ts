import type {
  LocalRunnerProviderAccount,
  LocalRunnerProvider,
  LocalRunnerProviderModel,
} from "@/domain/model/entity/local-runner";

export interface RawProviderModel {
  id: string;
  display_name: string;
  available: boolean;
  source: string;
}

export interface RawProviderAccount {
  id: string;
  homePath: string;
  label: string;
}

export interface RawProvider {
  id?: string;
  key: string;
  label: string;
  supported: boolean;
  installed: boolean;
  install_status?: "NOT_INSTALLED" | "INSTALLED" | "FAILED" | "UNSUPPORTED_OS";
  auth_status?: "READY" | "AUTH_REQUIRED" | "UNKNOWN";
  detected_binary?: string | null;
  detected_version?: string | null;
  models?: RawProviderModel[];
  last_error?: string | null;
  version: string;
  binaryPath: string;
  installHint: string;
  accounts?: RawProviderAccount[];
}

export function mapProviderModel(raw: RawProviderModel): LocalRunnerProviderModel {
  return {
    id: raw.id,
    displayName: raw.display_name,
    available: raw.available,
    source: raw.source,
  };
}

export function mapProvider(raw: RawProvider): LocalRunnerProvider {
  return {
    key: raw.key,
    label: raw.label,
    installed: raw.installed,
    version: raw.version,
    binaryPath: raw.binaryPath,
    supported: raw.supported,
    installStatus: raw.install_status,
    authStatus: raw.auth_status,
    detectedBinary: raw.detected_binary,
    detectedVersion: raw.detected_version,
    models: raw.models?.map(mapProviderModel),
    lastError: raw.last_error,
    installHint: raw.installHint,
    accounts: raw.accounts?.map(mapProviderAccount),
  };
}

export function mapProviderAccount(raw: RawProviderAccount): LocalRunnerProviderAccount {
  return {
    id: raw.id,
    homePath: raw.homePath,
    label: raw.label,
  };
}
