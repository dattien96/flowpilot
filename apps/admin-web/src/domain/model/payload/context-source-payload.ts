export interface CreateContextSourcePayload {
  projectId: string;
  featureId: string | null;
  type: "manual_text" | "url" | "api_note" | "file";
  title: string;
  rawContent: string;
}

export interface UpdateContextSourcePayload {
  contextSourceId: string;
  title: string;
  type: "manual_text" | "url" | "api_note" | "file";
  rawContent: string;
  summarizedContent?: string | null;
}
