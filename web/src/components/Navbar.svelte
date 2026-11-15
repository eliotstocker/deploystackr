<script lang="ts">
  import { triggerSync } from '../lib/api';

  interface Props {
    currentView: 'catalog' | 'timeline' | string;
    onViewChange: (view: 'catalog' | 'timeline' | string) => void;
    searchQuery: string;
    onSearchChange: (query: string) => void;
    serviceCount: number;
    onSyncDone?: () => void;
  }

  let {
    currentView,
    onViewChange,
    searchQuery,
    onSearchChange,
    serviceCount,
    onSyncDone
  }: Props = $props();

  let syncing = $state(false);
  let syncSuccess = $state(false);

  async function handleSync() {
    syncing = true;
    syncSuccess = false;
    try {
      await triggerSync();
      syncSuccess = true;
      if (onSyncDone) onSyncDone();
      setTimeout(() => (syncSuccess = false), 2500);
    } catch (err) {
      console.error(err);
    } finally {
      syncing = false;
    }
  }
</script>

<header class="navbar">
  <div class="nav-left">
    <button class="brand" onclick={() => onViewChange('catalog')} type="button">
      <div class="brand-icon">
        <svg width="22" height="22" viewBox="0 0 100 100" fill="none">
          <defs>
            <linearGradient id="navDolphinGrad" x1="85%" y1="15%" x2="15%" y2="85%">
              <stop offset="0%" stop-color="#f8fafc" />
              <stop offset="100%" stop-color="#94a3b8" />
            </linearGradient>
            <linearGradient id="navDolphinGradLight" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="#ffffff" stop-opacity="0.95" />
              <stop offset="100%" stop-color="#f8fafc" stop-opacity="0.65" />
            </linearGradient>
          </defs>
          <path d="M 44 28 C 42 16 37 7 30 5 C 27 13 26 23 28 34 Z" fill="url(#navDolphinGrad)" />
          <path d="M 16 62 C 19 44 32 28 54 27 C 68 26 77 34 81 40 C 84 42.5 89 45 94 47.5 C 95 48.5 94 50 92 51 C 86 52.5 78 54 70 54.5 C 54 55.5 38 56 16 62 Z" fill="url(#navDolphinGrad)" />
          <path d="M 16 62 C 38 56 54 55.5 70 54.5 C 78 54 86 52.5 92 51 C 90 53.5 84 55.5 78 57 C 71 59.5 63 65 58 75 C 56 79 53 79 54 73 C 55 66 52 64 45 66 C 34 69 24 68 16 62 Z" fill="url(#navDolphinGradLight)" />
        </svg>
      </div>
      <div class="brand-text">
        <span class="name">DeployStackr</span>
        <span class="badge">GitOps Engine</span>
      </div>
    </button>

    <nav class="nav-links">
      <button
        class="nav-item"
        class:active={currentView === 'catalog'}
        onclick={() => onViewChange('catalog')}
      >
        Catalog
        {#if serviceCount > 0}
          <span class="count-pill">{serviceCount}</span>
        {/if}
      </button>
      <button
        class="nav-item"
        class:active={currentView === 'timeline'}
        onclick={() => onViewChange('timeline')}
      >
        Timeline & Audit
      </button>
    </nav>
  </div>

  <div class="nav-right">
    <div class="search-box">
      <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="11" cy="11" r="8"/>
        <path d="m21 21-4.3-4.3"/>
      </svg>
      <input
        type="text"
        placeholder="Filter services or envs..."
        value={searchQuery}
        oninput={(e) => onSearchChange(e.target.value)}
      />
    </div>

    <button class="sync-btn" onclick={handleSync} disabled={syncing}>
      {#if syncing}
        <span class="spinner-small"></span> Syncing...
      {:else if syncSuccess}
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"/>
        </svg> Synced
      {:else}
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>
          <path d="M3 3v5h5"/>
          <path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/>
          <path d="M16 16h5v5"/>
        </svg> Sync GitOps
      {/if}
    </button>
  </div>
</header>

<style>
  .navbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.875rem 2rem;
    background: #0f172a;
    border-bottom: 1px solid #1e293b;
    position: sticky;
    top: 0;
    z-index: 100;
  }
  .nav-left {
    display: flex;
    align-items: center;
    gap: 2.5rem;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    cursor: pointer;
    background: transparent;
    border: none;
    padding: 0;
    text-align: left;
    font: inherit;
  }
  .brand-icon {
    font-size: 1.5rem;
    color: #38bdf8;
    background: #0284c720;
    width: 38px;
    height: 38px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 8px;
    border: 1px solid #0284c740;
  }
  .brand-text {
    display: flex;
    flex-direction: column;
  }
  .name {
    font-size: 1.125rem;
    font-weight: 800;
    color: #f8fafc;
    letter-spacing: -0.02em;
  }
  .badge {
    font-size: 0.65rem;
    color: #38bdf8;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .nav-links {
    display: flex;
    gap: 0.5rem;
  }
  .nav-item {
    background: transparent;
    border: none;
    color: #94a3b8;
    font-size: 0.875rem;
    font-weight: 600;
    padding: 0.5rem 0.875rem;
    border-radius: 6px;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    transition: all 0.15s ease;
  }
  .nav-item:hover {
    color: #f8fafc;
    background: #1e293b50;
  }
  .nav-item.active {
    color: #38bdf8;
    background: #1e293b;
  }
  .count-pill {
    background: #334155;
    color: #cbd5e1;
    font-size: 0.7rem;
    font-weight: 700;
    padding: 0.1rem 0.4rem;
    border-radius: 9999px;
  }
  .nav-right {
    display: flex;
    align-items: center;
    gap: 1rem;
  }
  .search-box {
    position: relative;
    display: flex;
    align-items: center;
  }
  .search-icon {
    position: absolute;
    left: 0.75rem;
    font-size: 0.8rem;
    color: #64748b;
  }
  .search-box input {
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 0.45rem 0.75rem 0.45rem 2.2rem;
    color: #f8fafc;
    font-size: 0.8125rem;
    width: 220px;
    outline: none;
    transition: width 0.2s ease, border-color 0.2s ease;
  }
  .search-box input:focus {
    width: 280px;
    border-color: #38bdf8;
  }
  .sync-btn {
    background: #1e293b;
    border: 1px solid #334155;
    color: #e2e8f0;
    padding: 0.45rem 0.875rem;
    border-radius: 6px;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    transition: all 0.15s ease;
  }
  .sync-btn:hover:not(:disabled) {
    background: #334155;
    color: #38bdf8;
  }
  .sync-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .spinner-small {
    width: 12px;
    height: 12px;
    border: 2px solid #ffffff40;
    border-top-color: #ffffff;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
