<script lang="ts">
  import { fetchHistory } from '../lib/api';
  import type { ServiceResponse, DeploymentEvent } from '../types';

  interface Props {
    services: ServiceResponse[];
    selectedService: string;
    onSelectService: (serviceName: string) => void;
    onPromoteVersion: (serviceName: string, env: string, version: string) => void;
  }

  let {
    services,
    selectedService,
    onSelectService,
    onPromoteVersion
  }: Props = $props();

  let historyMap = $state<Record<string, DeploymentEvent[]>>({});
  let loading = $state(false);
  let error = $state('');
  let currentEnvFilter = $state('all');

  $effect(() => {
    if (selectedService) {
      loadHistory(selectedService);
    }
  });

  async function loadHistory(svcName: string) {
    loading = true;
    error = '';
    try {
      const data = await fetchHistory(svcName);
      historyMap = data;
    } catch (err: any) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  let envList = $derived(Object.keys(historyMap || {}));

  let flatEvents = $derived((): DeploymentEvent[] => {
    let list: DeploymentEvent[] = [];
    for (const [env, evts] of Object.entries(historyMap || {})) {
      if (currentEnvFilter === 'all' || currentEnvFilter === env) {
        list.push(...evts);
      }
    }
    // Sort descending by deployed_at
    list.sort((a, b) => new Date(b.deployed_at).getTime() - new Date(a.deployed_at).getTime());
    return list;
  });

  function formatDate(isoStr?: string): string {
    if (!isoStr) return 'Present (Active)';
    const d = new Date(isoStr);
    return d.toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  }
</script>

<div class="timeline-view">
  <div class="view-header">
    <div class="header-left">
      <h2 class="title">Deployment History & Audit Timeline</h2>
      <p class="subtitle">Immutable GitOps audit trail extracted via git log history</p>
    </div>

    <div class="controls">
      <div class="control-group">
        <label for="service-select">Service</label>
        <select
          id="service-select"
          value={selectedService}
          onchange={(e) => onSelectService((e.target as HTMLSelectElement).value)}
        >
          {#each services as s}
            <option value={s.service}>{s.service}</option>
          {/each}
        </select>
      </div>

      <div class="env-tabs">
        <button
          class="tab-btn"
          class:active={currentEnvFilter === 'all'}
          onclick={() => (currentEnvFilter = 'all')}
        >
          All Envs
        </button>
        {#each envList as env}
          <button
            class="tab-btn"
            class:active={currentEnvFilter === env}
            onclick={() => (currentEnvFilter = env)}
          >
            {env}
          </button>
        {/each}
      </div>
    </div>
  </div>

  {#if loading}
    <div class="loading-state">
      <div class="spinner"></div>
      <span>Extracting deployment lifecycle from Git commits...</span>
    </div>
  {:else if error}
    <div class="error-banner">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display: inline-block; vertical-align: -2px; margin-right: 6px;">
        <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/>
        <line x1="12" y1="9" x2="12" y2="13"/>
        <line x1="12" y1="17" x2="12.01" y2="17"/>
      </svg>{error}
    </div>
  {:else if flatEvents().length === 0}
    <div class="empty-state">
      <p>No recorded deployment history for <strong>{selectedService}</strong> in the selected environment(s).</p>
      <p class="empty-sub">Deploy a version or run GitOps commits against the target manifests to hydrate the audit timeline.</p>
    </div>
  {:else}
    <div class="table-container">
      <table class="audit-table">
        <thead>
          <tr>
            <th>Status</th>
            <th>Version</th>
            <th>Environment</th>
            <th>Lifespan & Active Duration</th>
            <th>Deployed By</th>
            <th>Commit & PR</th>
            <th>Message</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          {#each flatEvents() as evt}
            <tr class="event-row" class:is-active={evt.is_active}>
              <td>
                {#if evt.is_active}
                  <span class="badge active-badge"><span class="status-indicator-dot active"></span>Active</span>
                {:else}
                  <span class="badge retired-badge"><span class="status-indicator-dot retired"></span>Superseded</span>
                {/if}
              </td>
              <td>
                <span class="version-tag">{evt.version}</span>
              </td>
              <td>
                <span class="env-tag">{evt.environment}</span>
              </td>
              <td class="duration-cell">
                <div class="duration-highlight">{evt.active_duration || 'active'}</div>
                <div class="date-range">
                  <span>{formatDate(evt.deployed_at)}</span>
                  <span class="range-sep">→</span>
                  <span>{formatDate(evt.retired_at)}</span>
                </div>
              </td>
              <td>
                <div class="author-name">{evt.author_name}</div>
                <div class="author-email">{evt.author_email}</div>
              </td>
              <td>
                <div class="commit-pill">
                  <span class="sha">{evt.commit_sha.slice(0, 7)}</span>
                  {#if evt.pr_number}
                    <span class="pr-badge">#{evt.pr_number}</span>
                  {/if}
                </div>
              </td>
              <td class="message-cell" title={evt.commit_message}>
                {evt.commit_message}
              </td>
              <td>
                <button
                  class="action-btn"
                  onclick={() => onPromoteVersion(selectedService, evt.environment, evt.version)}
                >
                  Promote
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .timeline-view {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }
  .view-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    flex-wrap: wrap;
    gap: 1rem;
    padding-bottom: 1rem;
    border-bottom: 1px solid #1e293b;
  }
  .title {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 600;
    color: #f8fafc;
  }
  .subtitle {
    margin: 0.25rem 0 0 0;
    font-size: 0.8125rem;
    color: #94a3b8;
  }
  .controls {
    display: flex;
    align-items: center;
    gap: 1.25rem;
    flex-wrap: wrap;
  }
  .control-group {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  label {
    font-size: 0.8125rem;
    color: #94a3b8;
    font-weight: 600;
  }
  select {
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 0.4rem 0.75rem;
    color: #f8fafc;
    font-size: 0.875rem;
  }
  .env-tabs {
    display: flex;
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 0.2rem;
  }
  .tab-btn {
    background: transparent;
    border: none;
    color: #94a3b8;
    padding: 0.3rem 0.75rem;
    border-radius: 4px;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    text-transform: capitalize;
  }
  .tab-btn:hover {
    color: #f8fafc;
  }
  .tab-btn.active {
    background: #1e293b;
    color: #38bdf8;
  }
  .table-container {
    background: #0f172a;
    border: 1px solid #1e293b;
    border-radius: 12px;
    overflow-x: auto;
    box-shadow: none;
  }
  .audit-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.875rem;
    text-align: left;
  }
  th {
    background: #0b1120;
    padding: 0.875rem 1rem;
    color: #94a3b8;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-bottom: 1px solid #1e293b;
  }
  td {
    padding: 1rem;
    border-bottom: 1px solid #1e293b80;
    color: #cbd5e1;
    vertical-align: middle;
  }
  .event-row:hover {
    background: #1e293b40;
  }
  .event-row.is-active {
    background: #0284c710;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    padding: 0.2rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 600;
  }
  .active-badge {
    background: #10b98120;
    color: #34d399;
    border: 1px solid #10b98140;
  }
  .retired-badge {
    background: #33415540;
    color: #94a3b8;
  }
  .status-indicator-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    margin-right: 0.375rem;
    display: inline-block;
  }
  .status-indicator-dot.active {
    background: #10b981;
  }
  .status-indicator-dot.retired {
    background: #64748b;
  }
  .version-tag {
    font-family: monospace;
    font-weight: 600;
    font-size: 0.95rem;
    color: #f8fafc;
  }
  .env-tag {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    background: #1e293b;
    padding: 0.2rem 0.45rem;
    border-radius: 4px;
    color: #94a3b8;
  }
  .duration-highlight {
    font-size: 0.8125rem;
    font-weight: 600;
    color: #38bdf8;
  }
  .date-range {
    font-size: 0.7rem;
    color: #64748b;
    margin-top: 0.15rem;
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .author-name {
    font-weight: 500;
    color: #f8fafc;
  }
  .author-email {
    font-size: 0.75rem;
    color: #64748b;
  }
  .commit-pill {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .sha {
    font-family: monospace;
    background: #1e293b;
    color: #38bdf8;
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
    font-size: 0.8rem;
  }
  .pr-badge {
    background: #475569;
    color: #ffffff;
    font-size: 0.7rem;
    font-weight: 600;
    padding: 0.1rem 0.4rem;
    border-radius: 10px;
  }
  .message-cell {
    max-width: 260px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: #94a3b8;
  }
  .action-btn {
    background: #1e293b;
    border: 1px solid #334155;
    color: #38bdf8;
    padding: 0.35rem 0.75rem;
    border-radius: 6px;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s ease;
  }
  .action-btn:hover {
    background: #0284c7;
    color: #ffffff;
    border-color: #0284c7;
  }
  .loading-state, .empty-state {
    padding: 3rem 1rem;
    text-align: center;
    background: #0f172a;
    border: 1px solid #1e293b;
    border-radius: 12px;
    color: #94a3b8;
  }
  .empty-sub {
    font-size: 0.8125rem;
    color: #64748b;
    margin-top: 0.5rem;
  }
  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid #334155;
    border-top-color: #38bdf8;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    margin: 0 auto 0.75rem auto;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
