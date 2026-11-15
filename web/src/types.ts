export interface SourceConfig {
  type: 'oci' | 'github_releases' | 'git_tags' | string;
  repository: string;
  filter?: string;
}

export interface ContextConfig {
  repo: string;
  changelog_strategy?: string;
  tag_prefix?: string;
  prefix?: string;
}

export interface DeploymentEvent {
  service: string;
  environment: string;
  version: string;
  commit_sha: string;
  commit_message: string;
  author_name: string;
  author_email: string;
  pr_number?: number;
  deployed_at: string;
  retired_at?: string;
  active_duration?: string;
  is_active: boolean;
}

export interface PendingPR {
  service: string;
  environment: string;
  target_version: string;
  branch?: string;
  pr_number?: number;
  pr_url?: string;
  user?: string;
  created_at: string;
}

export type ClusterStatus = {
    status: 'ready' | 'progressing' | 'pending' | 'degraded' | 'not_found' | 'error';
    ready_replicas: number;
    updated_replicas: number;
    desired_replicas: number;
    current_version?: string;
    message?: string;
};

export interface EnvironmentView {
  repo?: string;
  active_version: string;
  last_deployed?: string;
  deployed_by?: string;
  commit_sha?: string;
  pr_number?: number;
  write_strategy: 'direct' | 'pull_request' | string;
  target_file: string;
  target_path: string;
  recent_events?: DeploymentEvent[];
  pending_pr?: PendingPR;
  cluster_status?: ClusterStatus;
}

export interface ServicePermissions {
  github_teams?: string[];
  google_groups?: string[];
  require_repo_access?: 'read' | 'write' | 'admin' | string;
}

export interface EnvironmentPermission {
  can_deploy: boolean;
  reason?: string;
}

export interface TopEnvStatus {
  environment: string;
  active_version: string;
  latest_version: string;
  versions_behind: number;
  is_up_to_date: boolean;
  pending_pr?: PendingPR;
}

export interface ServiceResponse {
  service: string;
  display_name?: string;
  icon?: string;
  gitops_repo?: string;
  deploy_repos?: string[];
  source: SourceConfig;
  context: ContextConfig;
  permissions?: ServicePermissions;
  environment_priority?: string[];
  environments: Record<string, EnvironmentView>;
  user_permissions?: Record<string, EnvironmentPermission>;
  top_env_status?: TopEnvStatus;
  latest_version?: string;
}

export interface AuthUser {
  id: string;
  username: string;
  name: string;
  email: string;
  avatar_url?: string;
  provider: string;
  teams: string[];
  groups: string[];
  repos: Record<string, string>;
  deploy_repos?: string[];
}

export interface AuthMeResponse {
  authenticated: boolean;
  user?: AuthUser;
}

export interface DevPersona {
  id: string;
  name: string;
  role: string;
  description: string;
}

export interface AuthProvidersResponse {
  providers: string[];
  connected_provider?: string;
  dev_mode: boolean;
  personas?: DevPersona[];
}

export interface VersionInfo {
  version: string;
  released_at?: string;
  prerelease?: boolean;
  description?: string;
  changelog?: string;
}

export interface DiffEntry {
  sha: string;
  author_name: string;
  author_email: string;
  timestamp: string;
  subject: string;
  pr_number?: number;
}

export interface ChangelogDiff {
  from_version: string;
  to_version: string;
  commits: DiffEntry[];
  diff_stat?: string;
  raw_diff?: string;
  is_rollback?: boolean;
}

export interface PromotionRequest {
  service: string;
  environment: string;
  target_version: string;
  mode?: string;
  user?: string;
  email?: string;
  comment?: string;
}

export interface PromotionResult {
  success: boolean;
  mode: string;
  commit_sha?: string;
  branch?: string;
  pr_number?: number;
  pr_url?: string;
  message: string;
  target_file: string;
  old_version?: string;
  new_version: string;
}
