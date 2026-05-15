export interface CreateProjectPayload {
  name: string;
  description: string;
  platform: "android" | "ios" | "web" | "multi";
  repositoryUrl: string;
}
