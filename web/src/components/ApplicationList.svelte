<script lang="ts">
  import { onMount } from 'svelte';
  import type { ServiceResponse, AuthUser } from '../types';
  import ServiceIcon from './ServiceIcon.svelte';
  import SourceIcon from './SourceIcon.svelte';
  import UserMenu from './UserMenu.svelte';
  import { sortEnvironments, getTopEnvStatus } from '../lib/envPriority';

  interface Props {
    services: ServiceResponse[];
    selectedService: string;
    searchQuery: string;
    isCollapsed?: boolean;
    isDark?: boolean;
    isMobile?: boolean;
    currentView?: 'list' | 'overview';
    user?: AuthUser | null;
    onLogout?: () => void;
    onToggleTheme?: () => void;
    onSelectService: (serviceName: string) => void;
    onSearchChange: (query: string) => void;
    onToggleCollapse?: () => void;
    onViewChange?: (view: 'list' | 'overview') => void;
  }

  let {
    services,
    selectedService,
    searchQuery,
    isCollapsed = false,
    isDark = false,
    isMobile = false,
    currentView = 'list',
    user,
    onLogout,
    onToggleTheme,
    onSelectService,
    onSearchChange,
    onToggleCollapse,
    onViewChange
  }: Props = $props();

  let behindCount = $derived(
    services.filter(s => {
      const st = getTopEnvStatus(s);
      return st && !st.is_up_to_date;
    }).length
  );

  let filtered = $derived(
    services.filter((s) => {
      if (!searchQuery) return true;
      const q = searchQuery.toLowerCase();
      if (s.service.toLowerCase().includes(q)) return true;
      if (s.display_name && s.display_name.toLowerCase().includes(q)) return true;
      if (s.source?.repository?.toLowerCase().includes(q)) return true;
      const envs = Object.keys(s.environments || {});
      return envs.some((e) => e.toLowerCase().includes(q));
    })
  );

  function getPrimaryEnvInfo(svc: ServiceResponse): { env: string; version: string } {
    const sorted = sortEnvironments(svc.environments || {}, svc.environment_priority);
    //find first environment with an active version
    const withActive = sorted.find(([_, view]) => view.active_version && view.active_version !== 'unknown' && view.active_version !== 'none');
    if(withActive) {
      return { env: withActive[0], version: withActive[1].active_version };
    }
    if(sorted.length > 0) {
      return { env: sorted[0][0], version: sorted[0][1].active_version || 'none' };
    }
    return { env: 'env', version: 'none' };
  }

  function getOtherEnvs(svc: ServiceResponse, primaryEnv: string) {
    const sorted = sortEnvironments(svc.environments || {}, svc.environment_priority);
    return sorted.filter(([name]) => name !== primaryEnv);
  }

  function getEnvBadgeClass(envName: string): string {
    const lower = envName.toLowerCase();
    if (lower.includes('pre')) return 'badge-preprod';
    if (lower.includes('prod')) return 'badge-prod';
    if (lower.includes('stag')) return 'badge-staging';
    if (lower.includes('dev')) return 'badge-dev';
    return 'badge-default';
  }

  function formatShortVersion(v: string): string {
    if (!v || v === 'unknown') return 'n/a';
    if (v.includes(':')) {
      const parts = v.split(':');
      return parts[parts.length - 1];
    }
    return v;
  }

  type TooltipData = {
    text: string;
    version?: string;
    badgeClass?: string;
    top: number;
    left: number;
  };

  let tooltipData = $state<TooltipData | null>(null);
  let hoverTimer: any = null;

  function portal(node: HTMLElement) {
    document.body.appendChild(node);
    return {
      destroy() {
        if(node.parentNode) {
          node.parentNode.removeChild(node);
        }
      }
    };
  }

  function showTooltip(e: MouseEvent, text: string, version?: string, badgeClass?: string) {
    if(!isCollapsed) return;
    clearTimeout(hoverTimer);
    const current = e.currentTarget as HTMLElement;
    const icon = current.querySelector('.app-icon-circle') || current.querySelector('svg') || current;
    const rect = icon.getBoundingClientRect();
    const top = rect.top + rect.height / 2;
    const left = rect.right + 14;
    hoverTimer = setTimeout(() => {
      tooltipData = {text, version, badgeClass, top, left};
    }, 1000);
  }

  function hideTooltip() {
    clearTimeout(hoverTimer);
    if(tooltipData) {
      tooltipData = null;
    }
  }

  let deferredInstallPrompt = $state<any>(null);
  let isStandalone = $state(false);

  function handleBeforeInstallPrompt(e: Event) {
    e.preventDefault();
    deferredInstallPrompt = e;
  }

  function handleAppInstalled() {
    deferredInstallPrompt = null;
    isStandalone = true;
  }

  onMount(() => {
    // Check if already in standalone display mode
    if (window.matchMedia('(display-mode: standalone)').matches || (window.navigator as any).standalone === true) {
      isStandalone = true;
    }
    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
    window.addEventListener('appinstalled', handleAppInstalled);
    return () => {
      window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
      window.removeEventListener('appinstalled', handleAppInstalled);
    };
  });

  async function triggerInstallPrompt() {
    if (!deferredInstallPrompt) {
      // Fallback instruction for browsers like Safari iOS/Desktop
      alert('To install DeployStackr, tap Share and choose "Add to Home Screen" or use your browser\'s Install option.');
      return;
    }
    deferredInstallPrompt.prompt();
    const { outcome } = await deferredInstallPrompt.userChoice;
    if (outcome === 'accepted') {
      deferredInstallPrompt = null;
    }
  }

  $effect(() => {
    if(!isCollapsed) {
      hideTooltip();
    }
    const handleDismiss = () => hideTooltip();
    window.addEventListener('scroll', handleDismiss, { passive: true });
    window.addEventListener('resize', handleDismiss, { passive: true });
    return () => {
      clearTimeout(hoverTimer);
      window.removeEventListener('scroll', handleDismiss);
      window.removeEventListener('resize', handleDismiss);
    };
  });
</script>

<aside class="app-list-pane" class:collapsed={isCollapsed}>
  <!-- Brand Header -->
  <div class="brand-top-row">
    <a
      href="/"
      class="brand-left"
      onclick={(e) => {
        if(e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {
          e.preventDefault();
          if(isMobile && onViewChange) {
            onViewChange('overview');
          } else {
            onSelectService('');
          }
        }
      }}
      title="DeployStackr Overview"
    >
      <div class="brand-icon" title="DeployStackr GitOps">
        <svg width="24" height="24" viewBox="0 0 100 100" fill="none">
          <defs>
            <linearGradient id="dolphinGrad" x1="85%" y1="15%" x2="15%" y2="85%">
              <stop offset="0%" stop-color="#f8fafc" />
              <stop offset="100%" stop-color="#94a3b8" />
            </linearGradient>
            <linearGradient id="dolphinGradLight" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="#ffffff" stop-opacity="0.95" />
              <stop offset="100%" stop-color="#f8fafc" stop-opacity="0.65" />
            </linearGradient>
          </defs>

          <!-- Falcate Dorsal Fin -->
          <path d="M 44 28 C 42 16 37 7 30 5 C 27 13 26 23 28 34 Z" 
                fill="url(#dolphinGrad)" />

          <!-- Upper Body / Dark Dorsal Cape (Melon, Snout Top, Back) -->
          <path d="M 16 62 
                   C 19 44 32 28 54 27 
                   C 68 26 77 34 81 40 
                   C 84 42.5 89 45 94 47.5 
                   C 95 48.5 94 50 92 51 
                   C 86 52.5 78 54 70 54.5 
                   C 54 55.5 38 56 16 62 Z" 
                fill="url(#dolphinGrad)" />

          <!-- Lighter Countershaded Underbelly & Pectoral Flipper -->
          <path d="M 16 62 
                   C 38 56 54 55.5 70 54.5 
                   C 78 54 86 52.5 92 51 
                   C 90 53.5 84 55.5 78 57 
                   C 71 59.5 63 65 58 75 
                   C 56 79 53 79 54 73 
                   C 55 66 52 64 45 66 
                   C 34 69 24 68 16 62 Z" 
                fill="url(#dolphinGradLight)" />
        </svg>
      </div>
      {#if !isCollapsed}
        <div class="brand-info">
          <span class="brand-name">DeployStackr</span>
        </div>
      {/if}
    </a>

    <div class="brand-top-actions">
      {#if onToggleTheme && !isCollapsed}
        <button
          type="button"
          class="theme-toggle-btn"
          onclick={onToggleTheme}
          title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
          aria-label={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {#if isDark}
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="4"/>
              <path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>
            </svg>
          {:else}
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>
            </svg>
          {/if}
        </button>
      {/if}

      {#if user && onLogout && isMobile}
        <UserMenu {user} {services} {onLogout} />
      {/if}
    </div>
  </div>

  <!-- Mobile View Switcher Tabs -->
  {#if isMobile && onViewChange}
    <div class="mobile-nav-tabs">
      <button
        type="button"
        class="mobile-tab-btn"
        class:active={currentView === 'list'}
        onclick={() => onViewChange('list')}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect width="7" height="7" x="3" y="3" rx="1"/>
          <rect width="7" height="7" x="14" y="3" rx="1"/>
          <rect width="7" height="7" x="14" y="14" rx="1"/>
          <rect width="7" height="7" x="3" y="14" rx="1"/>
        </svg>
        <span>Applications</span>
        <span class="tab-count-pill">{services.length}</span>
      </button>

      <button
        type="button"
        class="mobile-tab-btn"
        class:active={currentView === 'overview'}
        onclick={() => onViewChange('overview')}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M22 12h-4l-3 9L9 3l-3 9H2"/>
        </svg>
        <span>Release Health</span>
        {#if behindCount > 0}
          <span class="tab-badge-behind">{behindCount} behind</span>
        {:else if services.length > 0}
          <span class="tab-badge-ok">✓</span>
        {/if}
      </button>
    </div>
  {/if}

  <!-- Search & Title Header (Hidden when collapsed) -->
  {#if !isCollapsed}
    <div class="pane-header">
      <div class="header-title-row">
        <span class="pane-title">Applications</span>
        <span class="count-pill">{services.length}</span>
      </div>

      <div class="search-wrapper">
        <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.3-4.3"/>
        </svg>
        <input
          type="text"
          class="search-input"
          placeholder="Filter applications..."
          value={searchQuery}
          oninput={(e) => onSearchChange((e.target as HTMLInputElement).value)}
        />
      </div>
    </div>
  {/if}

  <!-- Application Items List -->
  <div class="app-items-container" class:collapsed-items={isCollapsed} onscroll={hideTooltip}>
    {#if filtered.length === 0}
      <div class="empty-list">{isCollapsed ? '—' : 'No applications match'}</div>
    {:else}
      {#each filtered as svc}
        {@const primary = getPrimaryEnvInfo(svc)}
        {@const otherEnvs = getOtherEnvs(svc, primary.env)}
        {@const isSelected = svc.service === selectedService}

        <a
          href="/{svc.service}"
          class="app-item-row"
          class:selected={isSelected}
          class:collapsed={isCollapsed}
          onmouseenter={(e) => {
            if(isCollapsed) {
              showTooltip(
                e,
                svc.display_name || svc.service,
                primary.version && primary.version !== 'none' ? primary.version : undefined,
                getEnvBadgeClass(primary.env)
              );
            }
          }}
          onmouseleave={hideTooltip}
          onclick={(e) => {
            hideTooltip();
            if (e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {
              e.preventDefault();
              onSelectService(svc.service);
            }
          }}
        >
          <!-- Circular Icon Container -->
          <div class="app-icon-circle" class:selected={isSelected}>
            <ServiceIcon icon={svc.icon} name={svc.service} size={18} />
          </div>

          <!-- Application details (only visible when expanded) -->
          {#if !isCollapsed}
            <div class="app-identity">
              <span class="app-name">{svc.display_name || svc.service}</span>
              <span class="source-tag" title="{svc.source.type.replace('_', ' ')} ({svc.source.repository})">
                <SourceIcon type={svc.source.type} size={11} />
              </span>
            </div>

            <!-- Primary version pill with hover cascade dropdown -->
            <div
              class="prod-chip-container"
              class:has-dropdown={otherEnvs.length > 0}
              onclick={(e) => {
                e.preventDefault();
                e.stopPropagation();
              }}
              role="presentation"
            >
              <!-- Sizer keeping flow dimension in the row -->
              <div class="primary-version-pill sizer" aria-hidden="true">
                <span class="pill-info">
                  <span class="env-abbr">{primary.env.slice(0, 4)}:</span>
                  <span class="v-num">{formatShortVersion(primary.version)}</span>
                </span>
                {#if otherEnvs.length > 0}
                  <svg class="dropdown-arrow" width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="6 9 12 15 18 9"/>
                  </svg>
                {/if}
              </div>

              <!-- Unified card that contains the original pill and the dropdown pills -->
              <div class="env-dropdown-card">
                <div
                  class="primary-version-pill {getEnvBadgeClass(primary.env)}"
                  title="View environment versions"
                >
                  <span class="pill-info">
                    <span class="env-abbr">{primary.env.slice(0, 4)}:</span>
                    <span class="v-num">{formatShortVersion(primary.version)}</span>
                  </span>
                  {#if otherEnvs.length > 0}
                    <svg class="dropdown-arrow" width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="6 9 12 15 18 9"/>
                    </svg>
                  {/if}
                </div>

                {#if otherEnvs.length > 0}
                  <div class="env-cascade-dropdown">
                    {#each otherEnvs as [envName, envView]}
                      <div class="other-env-pill {getEnvBadgeClass(envName)}">
                        <span class="pill-info">
                          <span class="env-abbr">{envName.slice(0, 4)}:</span>
                          <span class="v-num">{formatShortVersion(envView.active_version)}</span>
                        </span>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </a>
      {/each}
    {/if}
  </div>

  <!-- Bottom Footer with Install App Button & Collapse / Expand Toggle Button -->
  {#if (!isStandalone && !isCollapsed) || onToggleCollapse}
    <div class="sidebar-footer" class:collapsed={isCollapsed}>
      {#if !isStandalone && !isCollapsed}
        <button
          type="button"
          class="sidebar-install-btn"
          onclick={triggerInstallPrompt}
          title="Install DeployStackr as a standalone web app"
          aria-label="Install DeployStackr"
        >
          <svg
            width="17"
            height="17"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="7 10 12 15 17 10" />
            <line x1="12" y1="15" x2="12" y2="3" />
          </svg>
          <span class="install-btn-text">Install App</span>
        </button>
      {/if}

      {#if onToggleCollapse}
        <button
          type="button"
          class="sidebar-toggle-btn"
          class:collapsed={isCollapsed}
          onclick={() => {
            hideTooltip();
            onToggleCollapse && onToggleCollapse();
          }}
          onmouseenter={(e) => {
            if(isCollapsed) {
              showTooltip(e, 'Expand sidebar');
            }
          }}
          onmouseleave={hideTooltip}
          title={isCollapsed ? undefined : 'Collapse sidebar to icons'}
          aria-label={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            class="toggle-svg"
            class:flipped={!isCollapsed}
          >
            <polyline points="9 18 15 12 9 6" />
          </svg>
        </button>
      {/if}
    </div>
  {/if}

  {#if isCollapsed && tooltipData}
    <div
      use:portal
      class="collapsed-tooltip"
      style="top: {tooltipData.top}px; left: {tooltipData.left}px;"
      role="tooltip"
    >
      <span class="collapsed-tooltip-name">{tooltipData.text}</span>
      {#if tooltipData.version && tooltipData.version !== 'none'}
        <span class="collapsed-tooltip-pill {tooltipData.badgeClass}">
          {formatShortVersion(tooltipData.version)}
        </span>
      {/if}
    </div>
  {/if}
</aside>

<style>
  .app-list-pane {
    background: var(--glass-bg);
    -webkit-backdrop-filter: blur(24px);
    backdrop-filter: blur(24px);
    border: none;
    border-radius: var(--radius-xl);
    box-shadow: none;
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: visible; /* allows cascade dropdown to overflow smoothly */
    transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  }

  .brand-top-row {
    padding: 1.25rem 1rem 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .app-list-pane.collapsed .brand-top-row {
    padding: 1.25rem 0.5rem 1rem;
    justify-content: center;
    flex-direction: column;
    gap: 0.75rem;
  }

  .brand-left {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    min-width: 0;
    text-decoration: none;
    color: inherit;
    cursor: pointer;
    user-select: none;
  }

  .brand-icon {
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: linear-gradient(135deg, #334155, #0f172a);
    color: #ffffff;
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: none;
    flex-shrink: 0;
  }

  :global(html.dark) .brand-icon {
    background: linear-gradient(135deg, #475569, #1e293b);
  }

  .brand-info {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .brand-name {
    font-size: 1.1rem;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--text-main);
    white-space: nowrap;
  }

  .brand-tag {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.15rem 0.45rem;
    border-radius: var(--radius-pill);
    background: rgba(100, 116, 139, 0.12);
    color: var(--text-muted);
    white-space: nowrap;
  }

  .theme-toggle-btn {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(255, 255, 255, 0.75);
    border: 1px solid rgba(203, 213, 225, 0.6);
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.2s ease;
    flex-shrink: 0;
  }

  .brand-top-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .mobile-nav-tabs {
    display: flex;
    background: rgba(148, 163, 184, 0.14);
    padding: 0.25rem;
    border-radius: var(--radius-lg, 12px);
    gap: 0.35rem;
    margin: 0.35rem 1rem 0.5rem;
    box-sizing: border-box;
  }

  :global(html.dark) .mobile-nav-tabs {
    background: rgba(255, 255, 255, 0.06);
  }

  .mobile-tab-btn {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    padding: 0.55rem 0.65rem;
    border-radius: var(--radius-md, 8px);
    border: none;
    background: transparent;
    color: var(--text-muted, #64748b);
    font-size: 0.82rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  :global(html.dark) .mobile-tab-btn {
    color: #94a3b8;
  }

  .mobile-tab-btn.active {
    background: #ffffff;
    color: var(--text-main, #0f172a);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  }

  :global(html.dark) .mobile-tab-btn.active {
    background: #1e293b;
    color: #f8fafc;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
  }

  .tab-count-pill {
    font-size: 0.72rem;
    padding: 0.1rem 0.4rem;
    border-radius: 9999px;
    background: rgba(148, 163, 184, 0.18);
    font-weight: 600;
  }

  .tab-badge-behind {
    font-size: 0.68rem;
    padding: 0.12rem 0.45rem;
    border-radius: 9999px;
    background: rgba(245, 158, 11, 0.18);
    color: #d97706;
    font-weight: 700;
  }

  :global(html.dark) .tab-badge-behind {
    color: #fbbf24;
  }

  .tab-badge-ok {
    font-size: 0.72rem;
    padding: 0.1rem 0.4rem;
    border-radius: 9999px;
    background: rgba(16, 185, 129, 0.15);
    color: #10b981;
    font-weight: 700;
  }

  .theme-toggle-btn:hover {
    color: var(--text-main);
    transform: scale(1.08);
  }

  .sidebar-footer {
    padding: 0.85rem 1.15rem 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    margin-top: auto;
  }

  .sidebar-footer.collapsed {
    padding: 0.85rem 0.5rem 1rem;
    justify-content: center;
  }

  .sidebar-install-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.45rem 0.85rem;
    border-radius: var(--radius-pill);
    background: rgba(100, 116, 139, 0.08);
    border: 1px solid rgba(100, 116, 139, 0.18);
    color: var(--text-muted);
    font-size: 0.78rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .sidebar-install-btn:hover {
    background: rgba(100, 116, 139, 0.16);
    color: var(--text-main);
    border-color: rgba(100, 116, 139, 0.3);
  }

  :global(html.dark) .sidebar-install-btn {
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(255, 255, 255, 0.12);
    color: #cbd5e1;
  }

  :global(html.dark) .sidebar-install-btn:hover {
    background: rgba(255, 255, 255, 0.12);
    border-color: rgba(255, 255, 255, 0.24);
    color: #f8fafc;
  }

  .sidebar-toggle-btn {
    width: 42px;
    height: 42px;
    border-radius: 50%;
    background: #ffffff;
    border: none;
    color: var(--text-main);
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: none;
    cursor: pointer;
    transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
    flex-shrink: 0;
    position: relative;
  }

  .sidebar-toggle-btn:hover {
    transform: scale(1.08);
    color: var(--accent-coral);
  }

  .toggle-svg {
    transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  }

  .toggle-svg.flipped {
    transform: rotate(180deg);
  }

  .pane-header {
    padding: 1.1rem 1.25rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  .header-title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .pane-title {
    font-size: 1.15rem;
    font-weight: 600;
    color: var(--text-main);
    letter-spacing: -0.02em;
  }

  .count-pill {
    background: rgba(255, 255, 255, 0.85);
    border: 1px solid rgba(203, 213, 225, 0.6);
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    padding: 0.15rem 0.55rem;
    border-radius: var(--radius-pill);
  }

  .search-wrapper {
    position: relative;
    display: flex;
    align-items: center;
  }

  .search-icon {
    position: absolute;
    left: 0.85rem;
    font-size: 0.8rem;
    color: var(--text-sub);
    pointer-events: none;
  }

  .search-input {
    width: 100%;
    background: rgba(255, 255, 255, 0.8);
    border: 1px solid rgba(203, 213, 225, 0.6);
    border-radius: var(--radius-pill);
    padding: 0.55rem 0.85rem 0.55rem 2.25rem;
    font-size: 0.85rem;
    color: var(--text-main);
    outline: none;
    transition: all 0.2s ease;
  }

  .search-input:focus {
    background: #ffffff;
    border-color: #64748b;
    box-shadow: 0 0 0 3px rgba(100, 116, 139, 0.15);
  }

  .app-items-container {
    flex: 1;
    overflow-y: auto;
    overflow-x: visible; /* so hover cascades can pop out */
    padding: 0.85rem 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .app-items-container.collapsed-items {
    padding: 0.85rem 0.5rem;
    align-items: center;
    overflow-x: hidden;
    scrollbar-width: none;
  }

  .app-items-container.collapsed-items::-webkit-scrollbar {
    display: none;
  }

  .empty-list {
    text-align: center;
    padding: 2.5rem 1rem;
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  /* Menu Item Row - Background removed per request! */
  .app-item-row {
    background: transparent;
    border: none;
    box-shadow: none;
    border-radius: var(--radius-md);
    padding: 0.45rem 0.5rem;
    cursor: pointer;
    transition: all 0.15s ease;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    position: relative;
    text-decoration: none;
    color: inherit;
  }

  .app-item-row:hover {
    background: transparent;
  }

  .app-item-row:hover .app-name {
    color: var(--text-main);
  }

  .app-item-row.selected {
    background: transparent;
    box-shadow: none;
  }

  .app-item-row.collapsed {
    padding: 0.4rem;
    justify-content: center;
  }

  /* Collapsed sidebar tooltip - floating outside scroll container with 1-second hover delay */
  .collapsed-tooltip {
    position: fixed;
    transform: translateY(-50%);
    background: #ffffff;
    color: var(--text-main);
    padding: 0.35rem 0.65rem;
    border-radius: 6px;
    font-size: 0.75rem;
    line-height: 1.25;
    white-space: nowrap;
    pointer-events: none;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1), 0 1px 3px rgba(0, 0, 0, 0.05);
    border: 1px solid rgba(203, 213, 225, 0.8);
    z-index: 99999;
    display: flex;
    align-items: center;
    gap: 0.45rem;
    animation: tooltipFadeIn 0.15s ease-out forwards;
  }

  @keyframes tooltipFadeIn {
    from {
      opacity: 0;
      transform: translateY(-50%) translateX(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(-50%) translateX(0);
    }
  }

  .collapsed-tooltip::before {
    content: '';
    position: absolute;
    right: 100%;
    top: 50%;
    transform: translateY(-50%);
    border-width: 5px;
    border-style: solid;
    border-color: transparent #ffffff transparent transparent;
  }

  :global(html.dark) .collapsed-tooltip {
    background: #1e293b;
    color: #f8fafc;
    border-color: rgba(255, 255, 255, 0.12);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.35);
  }

  :global(html.dark) .collapsed-tooltip::before {
    border-color: transparent #1e293b transparent transparent;
  }

  .collapsed-tooltip-name {
    font-weight: 600;
    color: inherit;
    font-size: 0.75rem;
  }

  .collapsed-tooltip-pill {
    font-family: var(--font-mono, monospace);
    font-size: 0.68rem;
    font-weight: 600;
    padding: 0.08rem 0.42rem;
    border-radius: var(--radius-pill, 9999px);
    line-height: 1.25;
    white-space: nowrap;
    letter-spacing: -0.01em;
  }

  /* Circle around the icon */
  .app-icon-circle {
    width: 38px;
    height: 38px;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.75);
    border: none;
    color: var(--text-main);
    display: flex;
    align-items: center;
    justify-content: center;
    box-shadow: none;
    flex-shrink: 0;
    transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  }

  .app-item-row:hover .app-icon-circle {
    transform: scale(1.14);
    background: #ffffff;
  }

  .app-icon-circle.selected {
    background: #0f172a;
    color: #ffffff;
    box-shadow: none;
  }

  .app-item-row:hover .app-icon-circle.selected {
    transform: scale(1.04);
    background: #0f172a;
    color: #ffffff;
    box-shadow: none;
  }

  :global(html.dark) .app-item-row:hover .app-icon-circle {
    background: rgba(51, 65, 85, 0.9);
  }

  :global(html.dark) .app-icon-circle.selected {
    background: #f8fafc;
    color: #0f172a;
  }

  :global(html.dark) .app-item-row:hover .app-icon-circle.selected {
    transform: scale(1.04);
    background: #f8fafc;
    color: #0f172a;
    box-shadow: none;
  }

  .app-identity {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
    flex: 1;
  }

  .app-name {
    font-weight: 600;
    font-size: 0.92rem;
    color: var(--text-main);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    letter-spacing: -0.01em;
  }

  .app-item-row.selected .app-name {
    color: var(--text-main);
    font-weight: 600;
  }

  :global(html.dark) .app-item-row.selected .app-name {
    color: #f8fafc;
  }

  .source-tag {
    display: inline-flex;
    align-items: center;
    color: var(--text-sub);
    opacity: 0.75;
    transition: opacity 0.15s ease, color 0.15s ease;
  }

  .app-item-row:hover .source-tag {
    opacity: 1;
    color: var(--text-main);
  }

  /* Production version chip wrapper */
  .prod-chip-container {
    position: relative;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
  }

  .primary-version-pill {
    display: inline-flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    min-width: 104px;
    box-sizing: border-box;
    gap: 0.35rem;
    padding: 0.25rem 0.55rem;
    border-radius: var(--radius-pill);
    font-size: 0.72rem;
    font-weight: 600;
    font-family: var(--font-mono);
    cursor: default;
    transition: all 0.15s ease;
  }

  .primary-version-pill.sizer {
    visibility: hidden;
    pointer-events: none;
    opacity: 0;
  }

  /* Unified dropdown card enclosing original pill + cascade pills on hover */
  .env-dropdown-card {
    position: absolute;
    top: -4px;
    right: -4px;
    left: -4px;
    padding: 4px;
    border-radius: var(--radius-lg, 12px);
    background: transparent;
    border: none;
    box-shadow: none;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    transition: background 0.18s cubic-bezier(0.16, 1, 0.3, 1),
                box-shadow 0.18s cubic-bezier(0.16, 1, 0.3, 1);
    z-index: 50;
    pointer-events: auto;
  }

  .prod-chip-container.has-dropdown:hover .env-dropdown-card {
    background: #ffffff;
    border: none;
    box-shadow: 0 10px 25px -4px rgba(15, 23, 42, 0.12), 0 4px 8px -2px rgba(15, 23, 42, 0.05);
    z-index: 1000;
  }

  :global(html.dark) .prod-chip-container.has-dropdown:hover .env-dropdown-card {
    background: #1e293b;
    border: none;
    box-shadow: 0 10px 25px -4px rgba(0, 0, 0, 0.45), 0 4px 8px -2px rgba(0, 0, 0, 0.25);
  }

  .pill-info {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
  }

  .env-abbr {
    text-transform: uppercase;
    font-size: 0.62rem;
    opacity: 0.8;
  }

  .dropdown-arrow {
    margin-left: auto;
    opacity: 0.7;
    transition: transform 0.2s ease;
    flex-shrink: 0;
  }

  .prod-chip-container.has-dropdown:hover .dropdown-arrow {
    transform: rotate(180deg);
  }

  /* Dropdown cascading directly below the primary pill within the card */
  .env-cascade-dropdown {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0.25rem;
    background: transparent;
    border: none;
    box-shadow: none;
    padding: 0;
    opacity: 0;
    visibility: hidden;
    pointer-events: none;
    transform: translateY(-4px);
    transition: opacity 0.18s cubic-bezier(0.16, 1, 0.3, 1),
                transform 0.2s cubic-bezier(0.16, 1, 0.3, 1),
                visibility 0.18s;
  }

  .prod-chip-container.has-dropdown:hover .env-cascade-dropdown {
    opacity: 1;
    visibility: visible;
    pointer-events: auto;
    transform: translateY(0);
  }

  .other-env-pill {
    display: inline-flex;
    align-items: center;
    width: 100%;
    box-sizing: border-box;
    gap: 0.25rem;
    padding: 0.25rem 0.55rem;
    border-radius: var(--radius-pill);
    font-size: 0.72rem;
    font-weight: 600;
    font-family: var(--font-mono);
    white-space: nowrap;
    box-shadow: none;
    opacity: 0;
    transform: translateY(-8px);
    transition: transform 0.12s ease, opacity 0.1s ease;
  }

  .prod-chip-container.has-dropdown:hover .other-env-pill {
    opacity: 1;
    transform: translateY(0);
  }

  .prod-chip-container.has-dropdown:hover .other-env-pill:nth-child(1) {
    transition: transform 0.22s cubic-bezier(0.16, 1, 0.3, 1) 0.03s,
                opacity 0.18s cubic-bezier(0.16, 1, 0.3, 1) 0.03s;
  }

  .prod-chip-container.has-dropdown:hover .other-env-pill:nth-child(2) {
    transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1) 0.07s,
                opacity 0.2s cubic-bezier(0.16, 1, 0.3, 1) 0.07s;
  }

  .prod-chip-container.has-dropdown:hover .other-env-pill:nth-child(3) {
    transition: transform 0.28s cubic-bezier(0.16, 1, 0.3, 1) 0.11s,
                opacity 0.22s cubic-bezier(0.16, 1, 0.3, 1) 0.11s;
  }

  .prod-chip-container.has-dropdown:hover .other-env-pill:nth-child(4) {
    transition: transform 0.31s cubic-bezier(0.16, 1, 0.3, 1) 0.15s,
                opacity 0.24s cubic-bezier(0.16, 1, 0.3, 1) 0.15s;
  }

  /* Environment Badges */
  .badge-prod {
    background: var(--accent-green-bg);
    color: var(--accent-green);
    border: 1px solid var(--accent-green-border);
  }

  :global(html.dark) .badge-prod {
    background: rgba(34, 197, 94, 0.18);
    color: #4ade80;
    border-color: rgba(34, 197, 94, 0.35);
  }

  .badge-preprod {
    background: #fef3c7;
    color: #b45309;
    border: 1px solid #fde68a;
  }

  :global(html.dark) .badge-preprod {
    background: rgba(245, 158, 11, 0.18);
    color: #fbbf24;
    border-color: rgba(245, 158, 11, 0.35);
  }

  .badge-staging {
    background: var(--accent-blue-bg);
    color: var(--accent-blue);
    border: 1px solid var(--accent-blue-border);
  }

  :global(html.dark) .badge-staging {
    background: rgba(14, 165, 233, 0.18);
    color: #38bdf8;
    border-color: rgba(14, 165, 233, 0.35);
  }

  .badge-dev {
    background: #f1f5f9;
    color: #475569;
    border: 1px solid #cbd5e1;
  }

  :global(html.dark) .badge-dev {
    background: rgba(51, 65, 85, 0.4);
    color: #cbd5e1;
    border-color: rgba(100, 116, 139, 0.4);
  }

  .badge-default {
    background: #f3e8ff;
    color: #7c3aed;
    border: 1px solid #e9d5ff;
  }

  :global(html.dark) .badge-default {
    background: rgba(139, 92, 246, 0.18);
    color: #c084fc;
    border-color: rgba(139, 92, 246, 0.35);
  }

  @media (max-width: 860px) {
    .app-list-pane {
      flex-grow: 1;
    }
  }
</style>
