export interface ContextSource {
  id: string;
  projectId: string;
  type: "manual_text" | "url" | "api_note" | "file";
  title: string;
  rawContent: string;
  summarizedContent: string | null;
  archivedAt?: string | null;
  createdBy: string;
  createdAt: string;
}
