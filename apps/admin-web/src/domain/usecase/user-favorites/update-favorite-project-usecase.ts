import type { UserFavoriteGateway } from "@/domain/gateway/user-favorite-gateway";

export class UpdateFavoriteProjectUseCase {
  constructor(private readonly userFavoriteGateway: UserFavoriteGateway) {}

  execute(targetType: "workflow" | "step", targetId: string, projectId: string | null): Promise<void> {
    return this.userFavoriteGateway.updateFavoriteProject(targetType, targetId, projectId);
  }
}
