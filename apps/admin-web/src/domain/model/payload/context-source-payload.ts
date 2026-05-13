export interface CreateContextSourcePayload {
  projectId: string;
  featureId: string | null;
  type: "manual_text" | "url" | "api_note" | "file";
  title: string;
  rawContent: string;
}
