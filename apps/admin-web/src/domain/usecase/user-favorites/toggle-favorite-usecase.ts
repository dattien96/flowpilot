import type { UserFavoriteGateway } from "@/domain/gateway/user-favorite-gateway";

export class ToggleFavoriteUseCase {
  constructor(private readonly userFavoriteGateway: UserFavoriteGateway) {}

  execute(targetType: "workflow" | "step", targetId: string, isFavorite: boolean): Promise<void> {
    return this.userFavoriteGateway.toggleFavorite(targetType, targetId, isFavorite);
  }
}
