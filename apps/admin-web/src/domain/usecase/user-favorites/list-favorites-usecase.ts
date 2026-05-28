import type { UserFavoriteGateway } from "@/domain/gateway/user-favorite-gateway";
import type { UserFavorite } from "@/domain/model/entity/user-favorite";

export class ListFavoritesUseCase {
  constructor(private readonly userFavoriteGateway: UserFavoriteGateway) {}

  execute(): Promise<UserFavorite[]> {
    return this.userFavoriteGateway.listFavorites();
  }
}
