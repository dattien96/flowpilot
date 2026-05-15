export interface Project {
  id: string;
  name: string;
  description: string;
  platform: "android" | "ios" | "web" | "multi";
  repositoryUrl: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}
