import type { UserFavorite } from "@/domain/model/entity/user-favorite";

export interface UserFavoriteGateway {
  listFavorites(): Promise<UserFavorite[]>;
  toggleFavorite(targetType: "workflow" | "step", targetId: string, isFavorite: boolean): Promise<void>;
  updateFavoriteProject(targetType: "workflow" | "step", targetId: string, projectId: string | null): Promise<void>;
}
