<script lang="ts">
  import { onDestroy } from "svelte";
  import type { AuthUser, ServiceResponse } from "../types";
  import { logoutUser, loginDev } from "../lib/api";

  type Props = {
    user: AuthUser;
    service?: ServiceResponse;
    services?: ServiceResponse[];
    onLogout?: () => void;
  };

  let { user, service, services, onLogout }: Props = $props();

  function cleanRepo(str: string): string {
    if(!str) return '';
    let s = str.toLowerCase().trim();
    s = s.replace(/^(https?:\/\/|git@|ssh:\/\/git@)/, '');
    if(s.includes('github.com/')) {
      s = s.split('github.com/')[1];
    } else if(s.includes('github.com:')) {
      s = s.split('github.com:')[1];
    }
    s = s.replace(/\.git\/?$/, '').replace(/^\/+|\/+$/g, '');
    return s;
  }

  function matchRepoKeys(repoKey: string, candidate: string): boolean {
    if(!repoKey || !candidate) return false;
    const a = cleanRepo(repoKey);
    const b = cleanRepo(candidate);
    if(a === b) return true;
    const aName = a.split('/').pop() || a;
    const bName = b.split('/').pop() || b;
    if(a === bName || b === aName) return true;
    if(a.endsWith('/' + b) || b.endsWith('/' + a)) return true;
    return false;
  }

  function isRepoRelevant(
    repoKey: string,
    svc?: ServiceResponse,
    allServices?: ServiceResponse[]
  ): boolean {
    //always filter in any repo written to for any deploy
    const deployTargets = new Set<string>();
    if(user.deploy_repos) {
      user.deploy_repos.forEach(r => deployTargets.add(r));
    }
    if(svc?.gitops_repo) deployTargets.add(svc.gitops_repo);
    if(svc?.deploy_repos) svc.deploy_repos.forEach(r => deployTargets.add(r));
    if(svc?.environments) {
      Object.values(svc.environments).forEach(env => {
        if(env.repo) deployTargets.add(env.repo);
      });
    }
    if(allServices) {
      allServices.forEach(s => {
        if(s.gitops_repo) deployTargets.add(s.gitops_repo);
        if(s.deploy_repos) s.deploy_repos.forEach(r => deployTargets.add(r));
        if(s.environments) {
          Object.values(s.environments).forEach(env => {
            if(env.repo) deployTargets.add(env.repo);
          });
        }
      });
    }

    for(const target of deployTargets) {
      if(matchRepoKeys(repoKey, target)) return true;
    }

    //check repos relevant to the active service
    if(svc) {
      if(matchRepoKeys(repoKey, svc.service)) return true;
      if(svc.source?.repository && matchRepoKeys(repoKey, svc.source.repository)) return true;
      if(svc.context?.repo && matchRepoKeys(repoKey, svc.context.repo)) return true;
      return false;
    }

    //when no service selected, match any configured service repo
    if(allServices && allServices.length > 0) {
      for(const s of allServices) {
        if(matchRepoKeys(repoKey, s.service)) return true;
        if(s.source?.repository && matchRepoKeys(repoKey, s.source.repository)) return true;
        if(s.context?.repo && matchRepoKeys(repoKey, s.context.repo)) return true;
      }
      return false;
    }

    return deployTargets.size < 1;
  }

  let relevantRepos = $derived(
    Object.entries(user.repos || {}).filter(([repo]) => isRepoRelevant(repo, service, services))
  );

  let menuOpen = $state(false);
  let closeTimer: ReturnType<typeof setTimeout> | undefined;

  function getInitials(name?: string, username?: string): string {
    const target = name || username || "U";
    const parts = target.trim().split(/\s+/);
    if (parts.length >= 2) {
      return (parts[0][0] + parts[1][0]).toUpperCase();
    }
    return target.slice(0, 2).toUpperCase();
  }

  function handleMouseEnter() {
    if (closeTimer) {
      clearTimeout(closeTimer);
      closeTimer = undefined;
    }
    menuOpen = true;
  }

  function handleMouseLeave() {
    closeTimer = setTimeout(() => {
      menuOpen = false;
    }, 180);
  }

  function handleWindowClick(e: MouseEvent) {
    const target = e.target as HTMLElement | null;
    if (menuOpen && !target?.closest(".user-menu-container")) {
      menuOpen = false;
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Escape" && menuOpen) {
      menuOpen = false;
    }
  }

  async function handleSignOut() {
    try {
      await logoutUser();
      if (onLogout) {
        onLogout();
      } else {
        window.location.reload();
      }
    } catch (err) {
      console.error("Logout error", err);
      window.location.reload();
    }
  }

  async function switchDevPersona(personaId: string) {
    try {
      await loginDev(personaId);
      window.location.reload();
    } catch (err) {
      console.error("Persona switch error", err);
      window.location.reload();
    }
  }

  onDestroy(() => {
    if (closeTimer) {
      clearTimeout(closeTimer);
    }
  });
</script>

<svelte:window onclick={handleWindowClick} onkeydown={handleKeyDown} />

<div
  class="user-menu-container"
  role="group"
  aria-label="User profile and menu"
  onmouseenter={handleMouseEnter}
  onmouseleave={handleMouseLeave}
>
  <button
    class="user-avatar-btn"
    class:active={menuOpen}
    onclick={() => (menuOpen = !menuOpen)}
    type="button"
    aria-expanded={menuOpen}
    aria-haspopup="dialog"
    aria-label="User profile and menu for {user.name || user.username}"
    title="{user.name || user.username} ({user.provider})"
  >
    {#if user.avatar_url}
      <img
        src={user.avatar_url}
        alt={user.name || user.username}
        class="avatar-img"
      />
    {:else}
      <span class="avatar-initials"
        >{getInitials(user.name, user.username)}</span
      >
    {/if}
  </button>

  {#if menuOpen}
    <div class="user-dropdown-popover" role="dialog" aria-label="User Menu">
      <div class="popover-user-card">
        <div class="popover-avatar">
          {#if user.avatar_url}
            <img
              src={user.avatar_url}
              alt={user.name || user.username}
              class="avatar-img-lg"
            />
          {:else}
            <span class="avatar-initials-lg"
              >{getInitials(user.name, user.username)}</span
            >
          {/if}
        </div>
        <div class="popover-details">
          <span class="popover-name">{user.name || user.username}</span>
          <span class="popover-email">{user.email || `@${user.username}`}</span>
          <span class="popover-provider-badge">Provider: {user.provider}</span>
        </div>
      </div>

      <div class="popover-section">
        <div class="section-label">Permissions &amp; Groups</div>

        <!-- GitHub Teams -->
        {#if user.teams && user.teams.length > 0}
          <div class="perm-group">
            <span class="perm-type">GitHub Teams</span>
            <div class="pill-wrap">
              {#each user.teams as team}
                <span class="perm-pill team-pill">{team}</span>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Google Groups -->
        {#if user.groups && user.groups.length > 0}
          <div class="perm-group">
            <span class="perm-type">Google Groups</span>
            <div class="pill-wrap">
              {#each user.groups as group}
                <span class="perm-pill group-pill">{group}</span>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Source Repos -->
        {#if relevantRepos.length > 0}
          <div class="perm-group">
            <span class="perm-type">Repo Permissions</span>
            <div class="pill-wrap">
              {#each relevantRepos as [repo, level]}
                <span class="perm-pill repo-pill" title="{repo}: {level}">
                  {repo.split("/").pop()}: <strong>{level}</strong>
                </span>
              {/each}
            </div>
          </div>
        {/if}

        {#if (!user.teams || user.teams.length === 0) && (!user.groups || user.groups.length === 0) && relevantRepos.length === 0}
          <span class="empty-perms">Standard authenticated access</span>
        {/if}
      </div>

      {#if user.provider === "dev"}
        <div class="popover-section dev-switch-section">
          <div class="section-label">Dev Persona Switcher</div>
          <div class="dev-switch-buttons">
            <button
              class="switch-btn"
              class:selected={user.id === "dev:admin"}
              onclick={() => switchDevPersona("dev_admin")}
              type="button"
            >
              Admin (Full)
            </button>
            <button
              class="switch-btn"
              class:selected={user.id === "dev:billing"}
              onclick={() => switchDevPersona("dev_billing")}
              type="button"
            >
              Billing Lead
            </button>
            <button
              class="switch-btn"
              class:selected={user.id === "dev:guest"}
              onclick={() => switchDevPersona("dev_guest")}
              type="button"
            >
              Viewer (Guest)
            </button>
          </div>
        </div>
      {/if}

      <div class="popover-footer">
        <button class="logout-btn" onclick={handleSignOut} type="button">
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
            <polyline points="16 17 21 12 16 7"></polyline>
            <line x1="21" y1="12" x2="9" y2="12"></line>
          </svg>
          <span>Sign Out</span>
        </button>
      </div>
    </div>
  {/if}
</div>

<style>
  .user-menu-container {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .user-avatar-btn {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    padding: 0;
    margin: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #0f172a;
    color: #f8fafc;
    border: 1.5px solid rgba(100, 116, 139, 0.25);
    cursor: pointer;
    overflow: hidden;
    transition: all 0.15s ease;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.08);
  }

  :global(html.dark) .user-avatar-btn {
    background: #1e293b;
    border-color: rgba(255, 255, 255, 0.15);
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
  }

  .user-avatar-btn:hover,
  .user-avatar-btn.active {
    border-color: var(--accent-color, #3b82f6);
    box-shadow: 0 0 0 2.5px rgba(59, 130, 246, 0.2);
    transform: scale(1.04);
  }

  .avatar-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .avatar-initials {
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: -0.02em;
    user-select: none;
  }

  .user-dropdown-popover {
    position: absolute;
    top: calc(100% + 8px);
    right: 0;
    width: 290px;
    background: rgba(255, 255, 255, 0.95);
    -webkit-backdrop-filter: blur(20px);
    backdrop-filter: blur(20px);
    border: 1px solid rgba(100, 116, 139, 0.2);
    border-radius: var(--radius-lg, 12px);
    box-shadow:
      0 20px 40px -10px rgba(0, 0, 0, 0.18),
      0 0 1px rgba(0, 0, 0, 0.15);
    padding: 0.85rem;
    z-index: 100;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    box-sizing: border-box;
    animation: popoverFadeIn 0.15s ease-out;
  }

  /* Invisible hover bridge to prevent cursor gap drop */
  .user-dropdown-popover::before {
    content: "";
    position: absolute;
    top: -10px;
    left: 0;
    right: 0;
    height: 10px;
  }

  :global(html.dark) .user-dropdown-popover {
    background: rgba(15, 23, 42, 0.94);
    border-color: rgba(255, 255, 255, 0.1);
    box-shadow:
      0 20px 40px -10px rgba(0, 0, 0, 0.6),
      0 0 1px rgba(255, 255, 255, 0.1);
  }

  @keyframes popoverFadeIn {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .popover-user-card {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid rgba(100, 116, 139, 0.15);
  }

  :global(html.dark) .popover-user-card {
    border-bottom-color: rgba(255, 255, 255, 0.08);
  }

  .popover-avatar {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    overflow: hidden;
    background: #0f172a;
    color: #f8fafc;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  :global(html.dark) .popover-avatar {
    background: #334155;
  }

  .avatar-img-lg {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .avatar-initials-lg {
    font-size: 0.95rem;
    font-weight: 700;
  }

  .popover-details {
    display: flex;
    flex-direction: column;
    min-width: 0;
    text-align: left;
  }

  .popover-name {
    font-size: 0.9rem;
    font-weight: 700;
    color: var(--text-main);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .popover-email {
    font-size: 0.75rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .popover-provider-badge {
    display: inline-block;
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--accent-color, #3b82f6);
    margin-top: 0.15rem;
  }

  .popover-section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    text-align: left;
  }

  .section-label {
    font-size: 0.675rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-sub, #64748b);
  }

  .perm-group {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .perm-type {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
  }

  .pill-wrap {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }

  .perm-pill {
    font-size: 0.7rem;
    padding: 0.15rem 0.45rem;
    border-radius: var(--radius-sm, 4px);
    background: rgba(100, 116, 139, 0.1);
    color: var(--text-main);
    border: 1px solid rgba(100, 116, 139, 0.2);
  }

  .team-pill {
    background: rgba(59, 130, 246, 0.1);
    border-color: rgba(59, 130, 246, 0.25);
    color: #2563eb;
  }

  :global(html.dark) .team-pill {
    color: #93c5fd;
  }

  .group-pill {
    background: rgba(16, 185, 129, 0.1);
    border-color: rgba(16, 185, 129, 0.25);
    color: #059669;
  }

  :global(html.dark) .group-pill {
    color: #6ee7b7;
  }

  .repo-pill {
    background: rgba(245, 158, 11, 0.1);
    border-color: rgba(245, 158, 11, 0.25);
    color: #d97706;
  }

  :global(html.dark) .repo-pill {
    color: #fcd34d;
  }

  .empty-perms {
    font-size: 0.75rem;
    color: var(--text-muted);
    font-style: italic;
  }

  .dev-switch-buttons {
    display: flex;
    gap: 0.35rem;
  }

  .switch-btn {
    flex: 1;
    padding: 0.35rem 0.2rem;
    font-size: 0.675rem;
    font-weight: 600;
    border-radius: var(--radius-sm, 6px);
    border: 1px solid rgba(100, 116, 139, 0.2);
    background: rgba(15, 23, 42, 0.03);
    color: var(--text-main);
    cursor: pointer;
    transition: all 0.12s ease;
    text-align: center;
  }

  :global(html.dark) .switch-btn {
    background: rgba(255, 255, 255, 0.04);
    border-color: rgba(255, 255, 255, 0.1);
  }

  .switch-btn:hover {
    background: rgba(15, 23, 42, 0.08);
  }

  :global(html.dark) .switch-btn:hover {
    background: rgba(255, 255, 255, 0.08);
  }

  .switch-btn.selected {
    background: var(--accent-color, #3b82f6);
    color: #ffffff;
    border-color: var(--accent-color, #3b82f6);
  }

  .popover-footer {
    padding-top: 0.6rem;
    border-top: 1px solid rgba(100, 116, 139, 0.15);
  }

  :global(html.dark) .popover-footer {
    border-top-color: rgba(255, 255, 255, 0.08);
  }

  .logout-btn {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    padding: 0.45rem;
    border-radius: var(--radius-md, 8px);
    border: 1px solid rgba(239, 68, 68, 0.25);
    background: rgba(239, 68, 68, 0.08);
    color: #ef4444;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.12s ease;
  }

  .logout-btn:hover {
    background: rgba(239, 68, 68, 0.15);
    border-color: rgba(239, 68, 68, 0.4);
  }
</style>
