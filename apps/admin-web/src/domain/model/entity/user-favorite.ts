export interface UserFavorite {
  id: string;
  userId: string;
  targetType: "workflow" | "step";
  targetId: string;
  defaultProjectId: string | null;
  createdAt: string;
}
