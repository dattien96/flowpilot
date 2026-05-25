import { getLocalRunnerBaseUrl } from "@/lib/env/browser-env";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";

type DirectoryValidationResult = {
  path: string;
  usable: boolean;
  reason: string;
};

async function validateBindingPath(path: string): Promise<DirectoryValidationResult> {
  const baseUrl = getLocalRunnerBaseUrl();
  let response: Response;

  try {
    response = await fetch(new URL("/directories/validate", baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({ path }),
    });
  } catch (error) {
    throw new Error(
      `Local runner is unreachable at ${baseUrl}. Start it with 'just runner-dev' or 'just dev', then retry workflow launch.`,
    );
  }

  if (!response.ok) {
    const message = await response.text();
    throw new Error(
      `Directory validation failed: ${message || `${response.status} ${response.statusText}`}`,
    );
  }

  return (await response.json()) as DirectoryValidationResult;
}

export function buildProjectBindingLaunchFailureMessage(
  bindings: DirectoryValidationResult[],
  directoryBindingPath: string,
) {
  const lines = bindings.map((binding) => `- ${binding.path}: ${binding.reason || "unusable path"}`);
  return [
    "Unable to start execution because the local runner could not open any bound directory for this project.",
    "",
    "Checked bindings:",
    ...lines,
    "",
    `Open the Directory Binding tab at ${directoryBindingPath} and add or update a valid local path for this machine.`,
  ].join("\n");
}

export async function ensureProjectHasUsableBinding(
  projectId: string,
  bindings: ProjectWorkspaceBinding[],
) {
  const directoryBindingPath = `/projects/${projectId}/directory-bindings`;
  if (bindings.length === 0) {
    throw new Error(
      `This project has no directory bindings. Open the Directory Binding tab at ${directoryBindingPath} and add a valid local path for this machine.`,
    );
  }

  const results = await Promise.all(
    bindings.map((binding) => validateBindingPath(binding.localPath)),
  );
  const usableBinding = results.find((binding) => binding.usable);
  if (usableBinding) {
    return usableBinding.path;
  }

  throw new Error(
    buildProjectBindingLaunchFailureMessage(results, directoryBindingPath),
  );
}
