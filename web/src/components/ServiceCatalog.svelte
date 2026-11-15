<script lang="ts">
  import type { ServiceResponse } from "../types";

  interface Props {
    services: ServiceResponse[];
    onPromote: (service: ServiceResponse, envName: string) => void;
    onViewHistory: (serviceName: string) => void;
  }

  let { services, onPromote, onViewHistory }: Props = $props();

  function formatRelative(isoStr?: string): string {
    if (!isoStr) return "Never deployed";
    const d = new Date(isoStr);
    const now = new Date();
    const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);
    if (diffSec < 60) return "just now";
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
    return `${Math.floor(diffSec / 86400)}d ago`;
  }

  function getEnvStatus(envView: any) {
    if(envView.pending_pr) {
      return {
        key: 'pending',
        label: 'Pending',
        tooltip: `PR #${envView.pending_pr.pr_number || 'open'} pending for ${envView.pending_pr.target_version}`,
      };
    }
    const cs = envView.cluster_status;
    if(!cs) return null;
    if(cs.status === 'ready') {
      return {
        key: 'ready',
        label: 'Ready',
        tooltip: cs.message || 'All replicas ready',
      };
    }
    if(cs.status === 'progressing') {
      return {
        key: 'deploying',
        label: 'Deploying',
        tooltip: cs.message || 'Rollout in progress',
      };
    }
    if(cs.status === 'degraded' || cs.status === 'error' || cs.status === 'not_found') {
      return {
        key: 'failed',
        label: 'Failed',
        tooltip: cs.message || 'Deployment degraded or failed',
      };
    }
    return null;
  }
</script>

<div class="catalog-grid">
  {#each services as svc}
    <div class="service-card">
      <div class="card-header">
        <div class="service-meta">
          <h3 class="service-name">{svc.service}</h3>
          <span
            class="source-tag"
            class:oci={svc.source.type === "oci"}
            class:gh={svc.source.type === "github_releases"}
          >
            {svc.source.type}
          </span>
        </div>
        <div class="repo-sub">{svc.source.repository}</div>
      </div>

      <div class="environments-container">
        {#each Object.entries(svc.environments || {}) as [envName, envView]}
          <div class="env-block">
            <div class="env-header">
              <span class="env-name">{envName}</span>
              <div class="env-header-badges">
                {#if getEnvStatus(envView)}
                  {@const st = getEnvStatus(envView)}
                  <span class="env-status-pill {st.key}" title={st.tooltip}>
                    <span class="status-pill-dot"></span>
                    <span>{st.label}</span>
                  </span>
                {/if}
                <span
                  class="strategy-pill"
                  class:pr={envView.write_strategy === "pull_request"}
                >
                  {envView.write_strategy === "pull_request" ? "PR" : "DIRECT"}
                </span>
              </div>
            </div>

            <div class="version-row">
              <span
                class="status-indicator {envView.cluster_status?.status || ''}"
                title={envView.cluster_status ? `${envView.cluster_status.status}: ${envView.cluster_status.message || ''}` : 'Status check not enabled'}
              ></span>
              <span class="version-tag"
                >{envView.active_version || "unknown"}</span
              >
              {#if envView.cluster_status}
                <span
                  class="cluster-badge {envView.cluster_status.status}"
                  title={envView.cluster_status.message || envView.cluster_status.status}
                >
                  {envView.cluster_status.ready_replicas}/{envView.cluster_status.desired_replicas}
                </span>
              {/if}
              {#if envView.pr_number}
                <span class="pr-pill">#{envView.pr_number}</span>
              {/if}
              {#if envView.pending_pr}
                {#if envView.pending_pr.pr_url}
                  <a
                    href={envView.pending_pr.pr_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="pending-pill is-link"
                    title="Open PR #{envView.pending_pr.pr_number || ''} on GitHub"
                    onclick={e => e.stopPropagation()}
                  >
                    <span class="pending-pill-ring"></span>
                    <span>PR #{envView.pending_pr.pr_number || 'open'} pending</span>
                    <svg
                      class="pending-pill-popout"
                      width="10"
                      height="10"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      aria-hidden="true"
                    >
                      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                      <polyline points="15 3 21 3 21 9"></polyline>
                      <line x1="10" y1="14" x2="21" y2="3"></line>
                    </svg>
                  </a>
                {:else}
                  <span
                    class="pending-pill"
                    title="Deploy PR #{envView.pending_pr.pr_number || ''} open for {envView.pending_pr.target_version}"
                  >
                    <span class="pending-pill-ring"></span>
                    <span>PR #{envView.pending_pr.pr_number || 'open'} pending</span>
                  </span>
                {/if}
              {/if}
            </div>

            <div class="env-meta">
              <span class="author-label">{envView.deployed_by || "system"}</span
              >
              <span class="dot">•</span>
              <span class="time-label"
                >{formatRelative(envView.last_deployed)}</span
              >
            </div>

            <div class="env-actions">
              <button
                class="promote-btn"
                onclick={() => onPromote(svc, envName)}
              >
                Promote →
              </button>
            </div>
          </div>
        {/each}
      </div>

      <div class="card-footer">
        <button class="history-link" onclick={() => onViewHistory(svc.service)}>
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            style="display: inline-block; vertical-align: -1px; margin-right: 4px;"
          >
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 6 12 12 16 14" />
          </svg>View Timeline & Audit History
        </button>
      </div>
    </div>
  {/each}
</div>

<style>
  .catalog-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
    gap: 1.5rem;
  }
  .service-card {
    background: #0f172a;
    border: 1px solid #1e293b;
    border-radius: 12px;
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    box-shadow: none;
    transition:
      transform 0.15s ease,
      border-color 0.15s ease;
  }
  .service-card:hover {
    border-color: #334155;
    transform: translateY(-2px);
  }
  .card-header {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid #1e293b;
  }
  .service-meta {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .service-name {
    margin: 0;
    font-size: 1.125rem;
    font-weight: 700;
    color: #f8fafc;
  }
  .source-tag {
    font-size: 0.6875rem;
    text-transform: uppercase;
    font-weight: 700;
    letter-spacing: 0.05em;
    padding: 0.2rem 0.5rem;
    border-radius: 9999px;
    background: #334155;
    color: #cbd5e1;
  }
  .source-tag.oci {
    background: #0284c725;
    color: #38bdf8;
  }
  .source-tag.gh {
    background: #a855f725;
    color: #c084fc;
  }
  .repo-sub {
    font-size: 0.8125rem;
    color: #64748b;
    font-family: monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .environments-container {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 0.75rem;
  }
  .env-block {
    background: #1e293b40;
    border: 1px solid #33415550;
    border-radius: 8px;
    padding: 0.875rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .env-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .env-header-badges {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .env-status-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.625rem;
    font-weight: 700;
    line-height: 1;
    padding: 0.12rem 0.38rem;
    border-radius: 9999px;
  }
  .env-status-pill .status-pill-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: currentColor;
  }
  .env-status-pill.ready {
    color: #34d399;
    background: rgba(16, 185, 129, 0.15);
    border: 1px solid rgba(16, 185, 129, 0.3);
  }
  .env-status-pill.deploying {
    color: #fbbf24;
    background: rgba(245, 158, 11, 0.15);
    border: 1px solid rgba(245, 158, 11, 0.35);
  }
  .env-status-pill.deploying .status-pill-dot {
    animation: statusDotPulse 1.8s infinite ease-in-out;
  }
  .env-status-pill.pending {
    color: #818cf8;
    background: rgba(99, 102, 241, 0.15);
    border: 1px solid rgba(99, 102, 241, 0.3);
  }
  .env-status-pill.failed {
    color: #f87171;
    background: rgba(239, 68, 68, 0.15);
    border: 1px solid rgba(239, 68, 68, 0.3);
  }
  @keyframes statusDotPulse {
    0%, 100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.35;
      transform: scale(0.8);
    }
  }
  .env-name {
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    color: #94a3b8;
    letter-spacing: 0.05em;
  }
  .strategy-pill {
    font-size: 0.65rem;
    font-weight: 700;
    padding: 0.1rem 0.35rem;
    border-radius: 4px;
    background: #0284c720;
    color: #38bdf8;
  }
  .strategy-pill.pr {
    background: #8b5cf620;
    color: #a78bfa;
  }
  .version-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0.2rem 0;
  }
  .status-indicator {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #10b981;
    transition: background 0.2s ease;
  }
  .status-indicator.progressing {
    background: #f59e0b;
    box-shadow: 0 0 6px rgba(245, 158, 11, 0.6);
  }
  .status-indicator.degraded {
    background: #ef4444;
    box-shadow: 0 0 6px rgba(239, 68, 68, 0.6);
  }
  .status-indicator.not_found {
    background: #64748b;
  }
  .status-indicator.error {
    background: #f87171;
  }
  .cluster-badge {
    font-size: 0.65rem;
    font-weight: 700;
    font-family: monospace;
    padding: 0.05rem 0.3rem;
    border-radius: 4px;
    background: rgba(16, 185, 129, 0.15);
    color: #34d399;
  }
  .cluster-badge.progressing {
    background: rgba(245, 158, 11, 0.15);
    color: #fbbf24;
  }
  .cluster-badge.pending {
    background: rgba(139, 92, 246, 0.15);
    color: #c084fc;
  }
  .cluster-badge.degraded {
    background: rgba(239, 68, 68, 0.15);
    color: #f87171;
  }
  .cluster-badge.not_found {
    background: rgba(100, 116, 139, 0.15);
    color: #94a3b8;
  }
  .version-tag {
    font-family: monospace;
    font-weight: 700;
    font-size: 0.95rem;
    color: #f8fafc;
  }
  .pr-pill {
    font-size: 0.6875rem;
    background: rgba(100, 116, 139, 0.2);
    color: #cbd5e1;
    padding: 0.1rem 0.35rem;
    border-radius: 4px;
    font-weight: 600;
  }
  .pending-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.68rem;
    font-weight: 600;
    color: #d97706;
    background: #fef3c7;
    padding: 0.12rem 0.45rem;
    border-radius: var(--radius-pill, 9999px);
    border: 1px solid rgba(245, 158, 11, 0.3);
  }
  :global(html.dark) .pending-pill {
    color: #fbbf24;
    background: rgba(245, 158, 11, 0.15);
    border-color: rgba(245, 158, 11, 0.35);
  }
  .pending-pill.is-link {
    text-decoration: none;
    cursor: pointer;
    transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  }
  .pending-pill.is-link:hover {
    background: #fde68a;
    border-color: rgba(245, 158, 11, 0.6);
    color: #b45309;
  }
  :global(html.dark) .pending-pill.is-link:hover {
    background: rgba(245, 158, 11, 0.25);
    border-color: rgba(245, 158, 11, 0.55);
    color: #fcd34d;
  }
  .pending-pill-popout {
    flex-shrink: 0;
    opacity: 0.75;
    transition: transform 0.15s ease, opacity 0.15s ease;
  }
  .pending-pill.is-link:hover .pending-pill-popout {
    opacity: 1;
    transform: translate(1px, -1px);
  }
  .pending-pill-ring {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    border: 2px solid #f59e0b;
    background: transparent;
  }
  .env-meta {
    font-size: 0.75rem;
    color: #64748b;
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .author-label {
    color: #94a3b8;
    max-width: 90px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .env-actions {
    margin-top: 0.5rem;
  }
  .promote-btn {
    width: 100%;
    background: #0284c7;
    color: #ffffff;
    border: none;
    border-radius: 6px;
    padding: 0.4rem 0.5rem;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s ease;
  }
  .promote-btn:hover {
    background: #0369a1;
  }
  .card-footer {
    padding-top: 0.5rem;
    border-top: 1px solid #1e293b;
    display: flex;
  }
  .history-link {
    background: transparent;
    border: none;
    color: #38bdf8;
    font-size: 0.8125rem;
    cursor: pointer;
    padding: 0;
    font-weight: 500;
  }
  .history-link:hover {
    text-decoration: underline;
  }
</style>
