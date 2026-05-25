export interface CreateProjectWorkspaceBindingPayload {
  localPath: string;
  label?: string | null;
}

export interface UpdateProjectWorkspaceBindingPayload {
  localPath?: string;
  label?: string | null;
}
