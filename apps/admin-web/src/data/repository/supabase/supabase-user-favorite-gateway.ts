import type { SupabaseClient } from "@supabase/supabase-js";
import type { UserFavoriteGateway } from "@/domain/gateway/user-favorite-gateway";
import type { UserFavorite } from "@/domain/model/entity/user-favorite";

type SupabaseRow = Record<string, unknown>;

function mapUserFavorite(row: SupabaseRow): UserFavorite {
  return {
    id: String(row.id),
    userId: String(row.user_id),
    targetType: row.target_type as "workflow" | "step",
    targetId: String(row.target_id),
    defaultProjectId: row.default_project_id ? String(row.default_project_id) : null,
    createdAt: String(row.created_at),
  };
}

export class SupabaseUserFavoriteGateway implements UserFavoriteGateway {
  constructor(private readonly supabase: SupabaseClient) {}

  async listFavorites(): Promise<UserFavorite[]> {
    const { data: userData } = await this.supabase.auth.getUser();
    if (!userData.user) return [];

    const { data, error } = await this.supabase
      .from("user_favorites")
      .select("*")
      .eq("user_id", userData.user.id)
      .order("created_at", { ascending: false });

    if (error) throw new Error(error.message);
    return (data ?? []).map(mapUserFavorite);
  }

  async toggleFavorite(targetType: "workflow" | "step", targetId: string, isFavorite: boolean): Promise<void> {
    const { data: userData } = await this.supabase.auth.getUser();
    if (!userData.user) throw new Error("Not authenticated");

    if (isFavorite) {
      const { error } = await this.supabase
        .from("user_favorites")
        .upsert(
          {
            user_id: userData.user.id,
            target_type: targetType,
            target_id: targetId,
          },
          { onConflict: "user_id,target_type,target_id" }
        );
      if (error) throw new Error(error.message);
    } else {
      const { error } = await this.supabase
        .from("user_favorites")
        .delete()
        .eq("user_id", userData.user.id)
        .eq("target_type", targetType)
        .eq("target_id", targetId);
      if (error) throw new Error(error.message);
    }
  }

  async updateFavoriteProject(targetType: "workflow" | "step", targetId: string, projectId: string | null): Promise<void> {
    const { data: userData } = await this.supabase.auth.getUser();
    if (!userData.user) throw new Error("Not authenticated");

    const { error } = await this.supabase
      .from("user_favorites")
      .update({ default_project_id: projectId })
      .eq("user_id", userData.user.id)
      .eq("target_type", targetType)
      .eq("target_id", targetId);

    if (error) throw new Error(error.message);
  }
}
