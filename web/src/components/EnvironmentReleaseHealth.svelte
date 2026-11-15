<script lang="ts">
  import type { ServiceResponse, AuthUser } from '../types';
  import ServiceIcon from './ServiceIcon.svelte';
  import UserMenu from './UserMenu.svelte';
  import { formatShortVersion, getTopEnvStatus } from '../lib/envPriority';

  type Props = {
    services: ServiceResponse[];
    currentUser?: AuthUser | null;
    isMobile?: boolean;
    isDark?: boolean;
    syncing?: boolean;
    onSelectService: (serviceName: string) => void;
    onViewChange?: (view: 'list' | 'overview') => void;
    onToggleTheme?: () => void;
    onLogout?: () => void;
    onSync?: () => void;
  };

  let {
    services,
    currentUser,
    isMobile = false,
    isDark = false,
    syncing = false,
    onSelectService,
    onViewChange,
    onToggleTheme,
    onLogout,
    onSync
  }: Props = $props();

  let filterQuery = $state<string>('');

  let filteredServices = $derived(
    services.filter(s => {
      if(!filterQuery.trim()) return true;
      const q = filterQuery.toLowerCase().trim();
      if(s.service.toLowerCase().includes(q)) return true;
      if(s.display_name && s.display_name.toLowerCase().includes(q)) return true;
      const st = getTopEnvStatus(s);
      if(st && st.environment && st.environment.toLowerCase().includes(q)) return true;
      return false;
    })
  );

  let upToDateCount = $derived(
    services.filter(s => getTopEnvStatus(s)?.is_up_to_date).length
  );

  let behindCount = $derived(
    services.filter(s => {
      const st = getTopEnvStatus(s);
      return st && !st.is_up_to_date;
    }).length
  );

  let pendingPRCount = $derived(
    services.filter(s => getTopEnvStatus(s)?.pending_pr).length
  );
</script>

<div class="empty-canvas-container" class:mobile-mode={isMobile}>
  <!-- Header: Mobile top bar or Desktop overview header -->
  {#if isMobile}
    <div class="mobile-overview-header">
      <div class="mobile-brand-bar">
        <div class="brand-left">
          <div class="brand-icon" title="DeployStackr GitOps">
            <svg width="24" height="24" viewBox="0 0 100 100" fill="none">
              <defs>
                <linearGradient id="dolphinGradHealth" x1="85%" y1="15%" x2="15%" y2="85%">
                  <stop offset="0%" stop-color="#f8fafc" />
                  <stop offset="100%" stop-color="#94a3b8" />
                </linearGradient>
                <linearGradient id="dolphinGradLightHealth" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stop-color="#ffffff" stop-opacity="0.95" />
                  <stop offset="100%" stop-color="#f8fafc" stop-opacity="0.65" />
                </linearGradient>
              </defs>
              <path d="M 44 28 C 42 16 37 7 30 5 C 27 13 26 23 28 34 Z" fill="url(#dolphinGradHealth)" />
              <path d="M 16 62 C 19 44 32 28 54 27 C 68 26 77 34 81 40 C 84 42.5 89 45 94 47.5 C 95 48.5 94 50 92 51 C 86 52.5 78 54 70 54.5 C 54 55.5 38 56 16 62 Z" fill="url(#dolphinGradHealth)" />
              <path d="M 16 62 C 38 56 54 55.5 70 54.5 C 78 54 86 52.5 92 51 C 90 53.5 84 55.5 78 57 C 71 59.5 63 65 58 75 C 56 79 53 79 54 73 C 55 66 52 64 45 66 C 34 69 24 68 16 62 Z" fill="url(#dolphinGradLightHealth)" />
            </svg>
          </div>
          <span class="brand-title">DeployStackr</span>
        </div>

        <div class="mobile-header-actions">
          {#if onSync}
            <button
              type="button"
              class="header-icon-btn"
              class:spinning={syncing}
              onclick={onSync}
              title="Sync manifests and releases"
              aria-label="Sync manifests"
            >
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21.5 2v6h-6M2.5 22v-6h6M2 11.5a10 10 0 0 1 18.8-4.3M22 12.5a10 10 0 0 1-18.8 4.3"/>
              </svg>
            </button>
          {/if}

          {#if onToggleTheme}
            <button
              type="button"
              class="header-icon-btn"
              onclick={onToggleTheme}
              title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
              aria-label="Toggle theme"
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

          {#if currentUser && onLogout}
            <UserMenu user={currentUser} {services} {onLogout} />
          {/if}
        </div>
      </div>

      <!-- Segmented View Tabs -->
      {#if onViewChange}
        <div class="mobile-nav-tabs">
          <button
            type="button"
            class="mobile-tab-btn"
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
            class="mobile-tab-btn active"
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
    </div>
  {:else}
    <header class="empty-canvas-header">
      <div class="overview-header-info">
        <h2 class="overview-title">Environment Release Health</h2>
        <p class="overview-subtitle">Fleet-wide status comparing highest-priority environments to latest releases</p>
      </div>
      {#if currentUser && onLogout}
        <div class="empty-canvas-user">
          <UserMenu user={currentUser} {services} {onLogout} />
        </div>
      {/if}
    </header>
  {/if}

  {#if services.length > 0}
    <!-- Fleet Stats Grid -->
    <div class="overview-stats-grid">
      <div class="stat-card">
        <span class="stat-label">Total Services</span>
        <span class="stat-value">{services.length}</span>
      </div>
      <div class="stat-card up-to-date">
        <span class="stat-label">Up to Date</span>
        <span class="stat-value">{upToDateCount}</span>
      </div>
      <div class="stat-card behind">
        <span class="stat-label">Behind Latest</span>
        <span class="stat-value">{behindCount}</span>
      </div>
      <div class="stat-card pending">
        <span class="stat-label">Pending PRs</span>
        <span class="stat-value">{pendingPRCount}</span>
      </div>
    </div>

    <!-- Filter input on mobile if more than 3 services -->
    {#if isMobile && services.length > 3}
      <div class="mobile-search-bar">
        <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.3-4.3"/>
        </svg>
        <input
          type="text"
          class="mobile-filter-input"
          placeholder="Filter release health..."
          bind:value={filterQuery}
        />
        {#if filterQuery}
          <button class="clear-search-btn" onclick={() => (filterQuery = '')} aria-label="Clear filter">✕</button>
        {/if}
      </div>
    {/if}

    <!-- Table / Card List of Services -->
    <div class="overview-table-card">
      {#if !isMobile}
        <div class="overview-table-header">
          <span class="col-service">Service</span>
          <span class="col-top-env">Top Priority Env</span>
          <span class="col-active">Active Version</span>
          <span class="col-latest">Latest Version</span>
          <span class="col-status">Release Delta</span>
        </div>
      {/if}

      <div class="overview-table-body">
        {#if filteredServices.length < 1}
          <div class="empty-filter-state">
            <p>No services match "{filterQuery}"</p>
          </div>
        {:else}
          {#each filteredServices as svc}
            {@const status = getTopEnvStatus(svc)}
            <button
              class="overview-row"
              class:mobile-row={isMobile}
              onclick={() => onSelectService(svc.service)}
              type="button"
            >
              {#if isMobile}
                <!-- Mobile Card Layout -->
                <div class="mobile-card-top">
                  <div class="mobile-service-identity">
                    <ServiceIcon icon={svc.icon} name={svc.service} size={28} />
                    <div class="service-name-text">
                      <span class="primary-name">{svc.display_name || svc.service}</span>
                      {#if svc.display_name && svc.display_name !== svc.service}
                        <span class="slug-name">{svc.service}</span>
                      {/if}
                    </div>
                  </div>

                  <div class="mobile-top-badges">
                    {#if status}
                      <span class="env-pill">{status.environment}</span>
                    {/if}
                    {#if status}
                      {#if status.pending_pr}
                        <span class="status-badge pending" title={`PR #${status.pending_pr.pr_number || ''} open`}>
                          <span class="pulse-dot"></span>
                          PR #{status.pending_pr.pr_number || 'open'}
                        </span>
                      {:else if status.is_up_to_date}
                        <span class="status-badge up-to-date">
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                            <polyline points="20 6 9 17 4 12"/>
                          </svg>
                          Up to date
                        </span>
                      {:else if status.versions_behind > 0}
                        <span class="status-badge behind">
                          {status.versions_behind} {status.versions_behind === 1 ? 'version' : 'versions'} behind
                        </span>
                      {:else}
                        <span class="status-badge neutral">Differing</span>
                      {/if}
                    {/if}
                  </div>
                </div>

                <div class="mobile-card-bottom">
                  <div class="mobile-versions-row">
                    <span class="version-label">Active:</span>
                    {#if status && status.active_version !== 'unknown'}
                      <code class="version-code active">{formatShortVersion(status.active_version)}</code>
                    {:else}
                      <span class="text-muted">unknown</span>
                    {/if}

                    <span class="version-arrow">→</span>

                    <span class="version-label">Latest:</span>
                    {#if status && status.latest_version}
                      <code class="version-code latest">{formatShortVersion(status.latest_version)}</code>
                    {:else}
                      <span class="text-muted">—</span>
                    {/if}
                  </div>

                  <svg class="chevron-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="9 18 15 12 9 6"/>
                  </svg>
                </div>
              {:else}
                <!-- Desktop 5-Column Grid Layout -->
                <div class="col-service row-service-name">
                  <ServiceIcon icon={svc.icon} name={svc.service} size={28} />
                  <div class="service-name-text">
                    <span class="primary-name" title={svc.display_name || svc.service}>{svc.display_name || svc.service}</span>
                    {#if svc.display_name && svc.display_name !== svc.service}
                      <span class="slug-name" title={svc.service}>{svc.service}</span>
                    {/if}
                  </div>
                </div>

                <div class="col-top-env">
                  {#if status}
                    <span class="env-pill">{status.environment}</span>
                  {:else}
                    <span class="text-muted">—</span>
                  {/if}
                </div>

                <div class="col-active" title={status?.active_version || ''}>
                  {#if status && status.active_version !== 'unknown'}
                    <code class="version-code active">{formatShortVersion(status.active_version)}</code>
                  {:else}
                    <span class="text-muted">unknown</span>
                  {/if}
                </div>

                <div class="col-latest" title={status?.latest_version || ''}>
                  {#if status && status.latest_version}
                    <code class="version-code latest">{formatShortVersion(status.latest_version)}</code>
                  {:else}
                    <span class="text-muted">—</span>
                  {/if}
                </div>

                <div class="col-status">
                  {#if status}
                    {#if status.pending_pr}
                      <span class="status-badge pending" title={`PR #${status.pending_pr.pr_number || ''} open`}>
                        <span class="pulse-dot"></span>
                        PR #{status.pending_pr.pr_number || 'open'} pending
                      </span>
                    {:else if status.is_up_to_date}
                      <span class="status-badge up-to-date">
                        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                          <polyline points="20 6 9 17 4 12"/>
                        </svg>
                        Up to date
                      </span>
                    {:else if status.versions_behind > 0}
                      <span class="status-badge behind">
                        {status.versions_behind} {status.versions_behind === 1 ? 'version' : 'versions'} behind
                      </span>
                    {:else}
                      <span class="status-badge neutral">
                        Differing
                      </span>
                    {/if}
                  {:else}
                    <span class="text-muted">—</span>
                  {/if}
                </div>
              {/if}
            </button>
          {/each}
        {/if}
      </div>
    </div>
  {:else}
    <div class="empty-selection">
      <div class="empty-selection-card">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="empty-selection-icon">
          <rect width="18" height="18" x="3" y="3" rx="2" />
          <path d="M9 3v18" />
          <path d="m14 9-3 3 3 3" />
        </svg>
        <p class="empty-selection-text">No services configured</p>
      </div>
    </div>
  {/if}
</div>

<style>
  .empty-canvas-container {
    flex: 1;
    display: flex;
    flex-direction: column;
    height: 100%;
    max-height: 100%;
    min-height: 0;
    overflow-y: auto;
    padding-bottom: 2rem;
    gap: 0.75rem;
  }

  .empty-canvas-container.mobile-mode {
    min-height: auto;
    padding-bottom: 2rem;
  }

  /* Desktop Header */
  .empty-canvas-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0 0.25rem 0.25rem;
    min-height: 40px;
    gap: 1rem;
    flex-shrink: 0;
  }

  .overview-header-info {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .overview-title {
    margin: 0;
    font-size: 1.2rem;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--text-main, #0f172a);
  }

  .overview-subtitle {
    margin: 0;
    font-size: 0.78rem;
    color: var(--text-muted, #94a3b8);
  }

  /* Mobile Top Navigation & Brand */
  .mobile-overview-header {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    margin-bottom: 1.25rem;
  }

  .mobile-brand-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.25rem 0.25rem;
  }

  .brand-left {
    display: flex;
    align-items: center;
    gap: 0.65rem;
  }

  .brand-icon {
    width: 32px;
    height: 32px;
    border-radius: var(--radius-md, 8px);
    background: #0f172a;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    box-shadow: 0 4px 12px rgba(15, 23, 42, 0.2);
  }

  :global(html.dark) .brand-icon {
    background: #090d16;
    border: 1px solid rgba(255, 255, 255, 0.12);
  }

  .brand-title {
    font-size: 1.05rem;
    font-weight: 700;
    color: var(--text-main, #0f172a);
    letter-spacing: -0.02em;
  }

  :global(html.dark) .brand-title {
    color: #f8fafc;
  }

  .mobile-header-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .header-icon-btn {
    width: 34px;
    height: 34px;
    border-radius: var(--radius-md, 8px);
    background: rgba(148, 163, 184, 0.12);
    border: 1px solid var(--border-color, rgba(148, 163, 184, 0.2));
    color: var(--text-main, #334155);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  :global(html.dark) .header-icon-btn {
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(255, 255, 255, 0.1);
    color: #cbd5e1;
  }

  .header-icon-btn.spinning svg {
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  /* Segmented Nav Tabs on Mobile */
  .mobile-nav-tabs {
    display: flex;
    background: rgba(148, 163, 184, 0.14);
    padding: 0.25rem;
    border-radius: var(--radius-lg, 12px);
    gap: 0.35rem;
    width: 100%;
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

  /* Stats Grid */
  .overview-stats-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 0.65rem;
    margin-bottom: 0.15rem;
    flex-shrink: 0;
  }

  @media (max-width: 960px) {
    .overview-stats-grid {
      grid-template-columns: repeat(2, 1fr);
      gap: 0.5rem;
      margin-bottom: 0.15rem;
    }
  }

  .stat-card {
    padding: 0.65rem 0.95rem;
    background: var(--glass-bg, rgba(255, 255, 255, 0.7));
    -webkit-backdrop-filter: blur(20px);
    backdrop-filter: blur(20px);
    border-radius: var(--radius-lg, 12px);
    border: 1px solid var(--border-color, rgba(148, 163, 184, 0.15));
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.03);
  }

  :global(html.dark) .stat-card {
    background: rgba(15, 23, 42, 0.65);
    border-color: rgba(255, 255, 255, 0.08);
  }

  .stat-label {
    font-size: 0.68rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted, #94a3b8);
  }

  .stat-value {
    font-size: 1.35rem;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--text-main, #0f172a);
  }

  :global(html.dark) .stat-value {
    color: #f8fafc;
  }

  .stat-card.up-to-date .stat-value { color: #10b981; }
  .stat-card.behind .stat-value { color: #f59e0b; }
  .stat-card.pending .stat-value { color: #3b82f6; }

  /* Mobile Search Bar */
  .mobile-search-bar {
    position: relative;
    display: flex;
    align-items: center;
    margin-bottom: 1rem;
    width: 100%;
  }

  .search-icon {
    position: absolute;
    left: 0.85rem;
    color: var(--text-muted, #94a3b8);
    pointer-events: none;
  }

  .mobile-filter-input {
    width: 100%;
    height: 38px;
    background: var(--glass-bg, rgba(255, 255, 255, 0.7));
    -webkit-backdrop-filter: blur(16px);
    backdrop-filter: blur(16px);
    border: 1px solid var(--border-color, rgba(148, 163, 184, 0.2));
    border-radius: var(--radius-md, 8px);
    padding: 0 2.2rem 0 2.2rem;
    font-size: 0.85rem;
    color: var(--text-main, #0f172a);
    outline: none;
    box-sizing: border-box;
  }

  :global(html.dark) .mobile-filter-input {
    background: rgba(15, 23, 42, 0.65);
    border-color: rgba(255, 255, 255, 0.1);
    color: #f8fafc;
  }

  .mobile-filter-input:focus {
    border-color: #3b82f6;
  }

  .clear-search-btn {
    position: absolute;
    right: 0.65rem;
    background: transparent;
    border: none;
    color: var(--text-muted, #94a3b8);
    font-size: 0.85rem;
    cursor: pointer;
  }

  /* Table & Card Container */
  .overview-table-card {
    background: var(--glass-bg, rgba(255, 255, 255, 0.75));
    -webkit-backdrop-filter: blur(24px);
    backdrop-filter: blur(24px);
    border-radius: var(--radius-xl, 16px);
    border: 1px solid var(--border-color, rgba(148, 163, 184, 0.18));
    overflow: hidden;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
    flex-shrink: 0;
  }

  :global(html.dark) .overview-table-card {
    background: rgba(15, 23, 42, 0.7);
    border-color: rgba(255, 255, 255, 0.08);
  }

  .overview-table-header {
    display: grid;
    grid-template-columns: minmax(180px, 2.2fr) 130px minmax(110px, 1.4fr) minmax(110px, 1.4fr) 150px;
    column-gap: 1.25rem;
    align-items: center;
    padding: 0.55rem 1rem;
    border-bottom: 1px solid var(--border-color, rgba(148, 163, 184, 0.15));
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted, #94a3b8);
    background: rgba(248, 250, 252, 0.4);
  }

  :global(html.dark) .overview-table-header {
    background: rgba(15, 23, 42, 0.4);
    border-bottom-color: rgba(255, 255, 255, 0.06);
  }

  .col-service, .col-top-env { min-width: 0; }
  .col-active, .col-latest {
    min-width: 0;
    display: flex;
    align-items: center;
    overflow: hidden;
  }
  .col-status {
    min-width: 0;
    display: flex;
    justify-content: flex-end;
    align-items: center;
  }

  .overview-table-body {
    display: flex;
    flex-direction: column;
  }

  .empty-filter-state {
    padding: 2.5rem 1.5rem;
    text-align: center;
    color: var(--text-muted, #94a3b8);
    font-size: 0.88rem;
  }

  /* Overview Row (Desktop) */
  .overview-row {
    width: 100%;
    display: grid;
    grid-template-columns: minmax(180px, 2.2fr) 130px minmax(110px, 1.4fr) minmax(110px, 1.4fr) 150px;
    column-gap: 1.25rem;
    align-items: center;
    padding: 0.6rem 1rem;
    border: none;
    border-bottom: 1px solid var(--border-color, rgba(148, 163, 184, 0.1));
    background: transparent;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    color: inherit;
    transition: background 0.12s ease;
  }

  .overview-row:last-child {
    border-bottom: none;
  }

  .overview-row:hover {
    background: rgba(59, 130, 246, 0.06);
  }

  :global(html.dark) .overview-row:hover {
    background: rgba(59, 130, 246, 0.1);
  }

  /* Overview Row (Mobile Card Adaptation) */
  .overview-row.mobile-row {
    display: flex;
    flex-direction: column;
    padding: 1rem 1.15rem;
    gap: 0.75rem;
    border-bottom: 1px solid var(--border-color, rgba(148, 163, 184, 0.15));
  }

  :global(html.dark) .overview-row.mobile-row {
    border-bottom-color: rgba(255, 255, 255, 0.06);
  }

  .mobile-card-top {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .mobile-service-identity {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    min-width: 0;
  }

  .mobile-top-badges {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  .mobile-card-bottom {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-left: 2.65rem;
    gap: 0.5rem;
  }

  .mobile-versions-row {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    flex-wrap: wrap;
    font-size: 0.78rem;
  }

  .version-label {
    font-size: 0.72rem;
    color: var(--text-muted, #94a3b8);
    font-weight: 500;
  }

  .version-arrow {
    color: var(--text-muted, #94a3b8);
    font-size: 0.78rem;
    padding: 0 0.1rem;
  }

  .chevron-icon {
    color: var(--text-muted, #94a3b8);
    flex-shrink: 0;
  }

  .row-service-name {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    min-width: 0;
  }

  .service-name-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
    overflow: hidden;
  }

  .primary-name {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--text-main, #0f172a);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  :global(html.dark) .primary-name {
    color: #f8fafc;
  }

  .slug-name {
    font-size: 0.72rem;
    color: var(--text-muted, #94a3b8);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .env-pill {
    display: inline-block;
    padding: 0.2rem 0.55rem;
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    border-radius: var(--radius-sm, 6px);
    background: rgba(148, 163, 184, 0.14);
    color: var(--text-main, #334155);
    white-space: nowrap;
  }

  :global(html.dark) .env-pill {
    background: rgba(148, 163, 184, 0.18);
    color: #cbd5e1;
  }

  .version-code {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.78rem;
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
    background: rgba(100, 116, 139, 0.08);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 130px;
    display: inline-block;
  }

  :global(html.dark) .version-code {
    background: rgba(255, 255, 255, 0.06);
  }

  .version-code.active {
    color: var(--text-main, #0f172a);
    font-weight: 500;
  }

  :global(html.dark) .version-code.active {
    color: #f8fafc;
  }

  .version-code.latest {
    color: #3b82f6;
    font-weight: 500;
  }

  .status-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.22rem 0.6rem;
    border-radius: 9999px;
    font-size: 0.74rem;
    font-weight: 600;
    letter-spacing: -0.01em;
    white-space: nowrap;
  }

  .status-badge.up-to-date {
    background: rgba(16, 185, 129, 0.12);
    color: #10b981;
    border: 1px solid rgba(16, 185, 129, 0.25);
  }

  .status-badge.behind {
    background: rgba(245, 158, 11, 0.12);
    color: #f59e0b;
    border: 1px solid rgba(245, 158, 11, 0.25);
  }

  .status-badge.pending {
    background: rgba(59, 130, 246, 0.12);
    color: #3b82f6;
    border: 1px solid rgba(59, 130, 246, 0.25);
  }

  .status-badge.neutral {
    background: rgba(148, 163, 184, 0.12);
    color: var(--text-muted, #94a3b8);
    border: 1px solid rgba(148, 163, 184, 0.2);
  }

  .pulse-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #3b82f6;
    box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.7);
    animation: pulseRing 1.8s infinite;
  }

  @keyframes pulseRing {
    0% {
      box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.7);
    }
    70% {
      box-shadow: 0 0 0 6px rgba(59, 130, 246, 0);
    }
    100% {
      box-shadow: 0 0 0 0 rgba(59, 130, 246, 0);
    }
  }

  .text-muted {
    color: var(--text-muted, #94a3b8);
    font-size: 0.8rem;
  }

  .empty-selection {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
  }

  .empty-selection-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.85rem;
    padding: 2.5rem 3rem;
    background: var(--glass-bg);
    -webkit-backdrop-filter: blur(24px);
    backdrop-filter: blur(24px);
    border-radius: var(--radius-xl);
    border: none;
    text-align: center;
    max-width: 440px;
  }

  .empty-selection-icon {
    color: var(--text-sub);
    opacity: 0.8;
  }

  .empty-selection-text {
    font-size: 1rem;
    font-weight: 500;
    color: var(--text-main);
    letter-spacing: -0.01em;
    line-height: 1.4;
  }
</style>