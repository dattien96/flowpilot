import type { UserFavoriteGateway } from "@/domain/gateway/user-favorite-gateway";
import type { UserFavorite } from "@/domain/model/entity/user-favorite";

export class InMemoryUserFavoriteGateway implements UserFavoriteGateway {
  private favorites: UserFavorite[] = [];

  async listFavorites(): Promise<UserFavorite[]> {
    return [...this.favorites];
  }

  async toggleFavorite(targetType: "workflow" | "step", targetId: string, isFavorite: boolean): Promise<void> {
    if (isFavorite) {
      if (!this.favorites.find((f) => f.targetType === targetType && f.targetId === targetId)) {
        this.favorites.push({
          id: crypto.randomUUID(),
          userId: "demo-user",
          targetType,
          targetId,
          defaultProjectId: null,
          createdAt: new Date().toISOString(),
        });
      }
    } else {
      this.favorites = this.favorites.filter((f) => !(f.targetType === targetType && f.targetId === targetId));
    }
  }

  async updateFavoriteProject(targetType: "workflow" | "step", targetId: string, projectId: string | null): Promise<void> {
    const favorite = this.favorites.find((f) => f.targetType === targetType && f.targetId === targetId);
    if (favorite) {
      favorite.defaultProjectId = projectId;
    }
  }
}
