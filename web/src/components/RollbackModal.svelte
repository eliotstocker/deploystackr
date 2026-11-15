<script lang="ts">
  import { onMount } from "svelte";
  import { promote, fetchHistory } from "../lib/api";
  import type {
    ServiceResponse,
    DeploymentEvent,
    AuthUser,
    PromotionResult,
  } from "../types";
  import UserAvatar from "./UserAvatar.svelte";

  interface Props {
    service: ServiceResponse;
    envName: string;
    events: DeploymentEvent[];
    user?: AuthUser | null;
    onClose: () => void;
    onPromote: (
      service: ServiceResponse,
      envName?: string,
      targetVersion?: string,
    ) => void;
    onSuccess: () => void;
  }

  let {
    service,
    envName,
    events,
    user,
    onClose,
    onPromote,
    onSuccess,
  }: Props = $props();

  let localEvents = $state<DeploymentEvent[]>([]);
  let loadingHistory = $state(false);

  onMount(async () => {
    if (events && events.length > 0) {
      localEvents = events;
    } else {
      loadingHistory = true;
      try {
        const hist = await fetchHistory(service.service);
        localEvents = hist?.[envName] || [];
      } catch (err) {
        console.warn("Failed to fetch history in RollbackModal", err);
      } finally {
        loadingHistory = false;
      }
    }
  });

  $effect(() => {
    if (events && events.length > 0 && localEvents.length === 0) {
      localEvents = events;
    }
  });

  let envView = $derived(service.environments?.[envName]);
  let currentActiveVersion = $derived(envView?.active_version || "");
  let writeStrategy = $derived(envView?.write_strategy || "direct");
  let canDeploy = $derived(
    service.user_permissions?.[envName]?.can_deploy ?? true,
  );
  let permReason = $derived(
    service.user_permissions?.[envName]?.reason || "",
  );

  // Take the last ~5 deployments for this environment
  let displayEvents = $derived(localEvents.slice(0, 5));

  let confirmingEvent = $state<DeploymentEvent | null>(null);
  let comment = $state("");
  let submitting = $state(false);
  let error = $state("");
  let result = $state<PromotionResult | null>(null);

  function formatShortVersion(v?: string): string {
    if (!v || v === "unknown") return "none";
    if (v.includes(":")) {
      const parts = v.split(":");
      return parts[parts.length - 1];
    }
    return v;
  }

  function formatFullDate(isoStr?: string): string {
    if (!isoStr) return "Unknown";
    try {
      const d = new Date(isoStr);
      return d.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return isoStr;
    }
  }

  function formatRelative(isoStr?: string): string {
    if (!isoStr) return "Never";
    const d = new Date(isoStr);
    const now = new Date();
    const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);
    if (diffSec < 60) return "just now";
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
    return `${Math.floor(diffSec / 86400)}d ago`;
  }

  function getEnvBadgeClass(name: string): string {
    const lower = name.toLowerCase();
    if (lower.includes("pre")) return "badge-preprod";
    if (lower.includes("prod")) return "badge-prod";
    if (lower.includes("stag")) return "badge-staging";
    if (lower.includes("dev")) return "badge-dev";
    return "badge-default";
  }

  function isCurrentActive(event: DeploymentEvent): boolean {
    if (event.is_active) return true;
    const cleanActive = formatShortVersion(currentActiveVersion);
    const cleanEventVer = formatShortVersion(event.version);
    return cleanActive === cleanEventVer;
  }

  function startRollback(event: DeploymentEvent) {
    confirmingEvent = event;
    comment = `Rollback ${envName} to ${formatShortVersion(event.version)}`;
    error = "";
  }

  function cancelRollback() {
    confirmingEvent = null;
    comment = "";
    error = "";
  }

  async function handleExecuteRollback() {
    if (!confirmingEvent) return;
    submitting = true;
    error = "";
    try {
      const res = await promote({
        service: service.service,
        environment: envName,
        target_version: confirmingEvent.version,
        mode: writeStrategy,
        comment:
          comment.trim() ||
          `Rollback ${envName} to ${formatShortVersion(confirmingEvent.version)}`,
        user: user?.name || "DeployStackr User",
      });
      result = res;
      onSuccess();
    } catch (err: any) {
      error = err.message || "Rollback failed";
    } finally {
      submitting = false;
    }
  }
</script>

<div
  class="modal-backdrop"
  onclick={(e) => {
    if (e.target === e.currentTarget && !submitting) onClose();
  }}
  onkeydown={(e) => e.key === "Escape" && !submitting && onClose()}
  role="presentation"
>
  <div class="modal-panel" role="dialog" aria-modal="true" aria-labelledby="rollback-title">
    <!-- Modal Header -->
    <div class="modal-header">
      <div class="header-titles">
        <div class="header-badge-row">
          <span class="status-dot {getEnvBadgeClass(envName)}"></span>
          <span class="env-chip {getEnvBadgeClass(envName)}">{envName}</span>
          <span class="service-name-label">{service.display_name || service.service}</span>
        </div>
        <h2 id="rollback-title" class="modal-title">
          Rollback Environment
        </h2>
        <p class="modal-sub">
          Select a historical deployment to roll back <strong>{envName}</strong>.
        </p>
      </div>

      <button
        class="close-btn"
        onclick={onClose}
        disabled={submitting}
        type="button"
        aria-label="Close"
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <line x1="18" y1="6" x2="6" y2="18" />
          <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>
    </div>

    <!-- Modal Body -->
    <div class="modal-body">
      {#if result}
        <!-- Success State -->
        <div class="result-view">
          <div class="success-icon">
            <svg
              width="28"
              height="28"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="20 6 9 17 4 12" />
            </svg>
          </div>
          <h3 class="success-heading">Rollback Initiated!</h3>
          <p class="success-msg">{result.message}</p>

          {#if result.commit_sha}
            <div class="result-meta-row">
              <span class="meta-label">Git Commit:</span>
              <code class="meta-sha">{result.commit_sha.slice(0, 7)}</code>
            </div>
          {/if}

          {#if result.pr_url}
            <div class="result-meta-row">
              <span class="meta-label">Pull Request:</span>
              <a
                href={result.pr_url}
                target="_blank"
                rel="noreferrer"
                class="pr-link"
              >
                View Pull Request #{result.pr_number || ""} ↗
              </a>
            </div>
          {/if}

          <div class="modal-footer" style="padding: 1.5rem 0 0 0; justify-content: center;">
            <button class="primary-btn" onclick={onClose} type="button">
              Done
            </button>
          </div>
        </div>
      {:else}
        <!-- Permission warning if user cannot deploy -->
        {#if !canDeploy}
          <div class="permission-notice">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span>You do not have permission to deploy to <strong>{envName}</strong>{permReason ? ` (${permReason})` : ''}. Rollback is disabled.</span>
          </div>
        {/if}

        {#if error}
          <div class="error-notice">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polygon points="7.86 2 16.14 2 22 7.86 22 16.14 16.14 22 7.86 22 2 16.14 2 7.86 7.86 2" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span>{error}</span>
          </div>
        {/if}

        <!-- Active Version Info Banner -->
        <div class="current-env-summary">
          <div class="summary-col">
            <span class="summary-k">Currently Active</span>
            <span class="summary-v version-pill-active">
              {formatShortVersion(currentActiveVersion)}
            </span>
          </div>
          <div class="summary-col">
            <span class="summary-k">Write Strategy</span>
            <span class="summary-v strategy-tag">
              {writeStrategy === "pull_request" ? "Pull Request" : "Direct Commit"}
            </span>
          </div>
          <div class="summary-col">
            <span class="summary-k">Target File</span>
            <span class="summary-v mono-text" title={envView?.target_file}>
              {envView?.target_file || "config.yaml"}
            </span>
          </div>
        </div>

        <!-- Inline Rollback Confirmation Panel -->
        {#if confirmingEvent}
          <div class="confirmation-panel">
            <div class="confirm-header">
              <div class="confirm-icon">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="1 4 1 10 7 10"></polyline>
                  <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path>
                </svg>
              </div>
              <div class="confirm-text">
                <h4 class="confirm-title">
                  Confirm Rollback to {formatShortVersion(confirmingEvent.version)}
                </h4>
                <p class="confirm-desc">
                  This will change <strong>{envName}</strong> from
                  <code>{formatShortVersion(currentActiveVersion)}</code> back to
                  <code>{formatShortVersion(confirmingEvent.version)}</code> via
                  <strong>{writeStrategy === "pull_request" ? "Pull Request" : "Direct Commit"}</strong>.
                </p>
              </div>
            </div>

            <div class="confirm-form">
              <label for="rollback-comment" class="confirm-label">
                Deployment note / comment (optional)
              </label>
              <input
                id="rollback-comment"
                type="text"
                class="confirm-input"
                bind:value={comment}
                placeholder="e.g. Reverting due to incident #104"
                disabled={submitting}
              />
            </div>

            <div class="confirm-actions">
              <button
                class="secondary-btn"
                onclick={cancelRollback}
                disabled={submitting}
                type="button"
              >
                Cancel
              </button>

              <button
                class="text-link-btn"
                onclick={() => {
                  if (!confirmingEvent) return;
                  const v = confirmingEvent.version;
                  onClose();
                  onPromote(service, envName, v);
                }}
                disabled={submitting}
                type="button"
              >
                Advanced Deploy Options →
              </button>

              <button
                class="danger-rollback-btn"
                onclick={handleExecuteRollback}
                disabled={submitting || !canDeploy}
                type="button"
              >
                {#if submitting}
                  <span class="spinner-sm"></span>
                  Rolling back...
                {:else}
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="1 4 1 10 7 10"></polyline>
                    <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path>
                  </svg>
                  Confirm Rollback
                {/if}
              </button>
            </div>
          </div>
        {/if}

        <!-- Historical Deployments List -->
        <div class="history-section">
          <div class="history-section-header">
            <h3 class="section-title">Deployment History (Last {displayEvents.length || 5})</h3>
            <span class="section-sub">Chronological events in {envName}</span>
          </div>

          {#if loadingHistory}
            <div class="loading-state">
              <div class="spinner-sm"></div>
              <span>Loading deployment events...</span>
            </div>
          {:else if displayEvents.length === 0}
            <div class="empty-state">
              <p>No historical deployment events recorded for this environment.</p>
            </div>
          {:else}
            <div class="timeline-list">
              {#each displayEvents as event, idx}
                {@const active = isCurrentActive(event)}
                <div
                  class="timeline-item"
                  class:is-active={active}
                  class:is-selected={confirmingEvent?.version === event.version}
                >
                  <!-- Timeline track & marker -->
                  <div class="timeline-track">
                    <span class="timeline-marker" class:active-marker={active}></span>
                    {#if idx < displayEvents.length - 1}
                      <span class="timeline-line"></span>
                    {/if}
                  </div>

                  <!-- Event details card -->
                  <div class="event-card">
                    <div class="event-card-top">
                      <div class="version-tag-row">
                        <span class="version-badge" class:active-version={active}>
                          {formatShortVersion(event.version)}
                        </span>

                        {#if active}
                          <span class="status-pill active-pill">
                            <span class="pulse-dot"></span>
                            Currently Active
                          </span>
                        {:else}
                          <span class="status-pill retired-pill">
                            Retired
                          </span>
                        {/if}
                      </div>

                      <div class="event-card-action">
                        {#if active}
                          <span class="active-tag-label">Active</span>
                        {:else}
                          <button
                            class="rollback-trigger-btn"
                            class:selected={confirmingEvent?.version === event.version}
                            onclick={() => startRollback(event)}
                            disabled={!canDeploy || submitting}
                            type="button"
                            title={!canDeploy ? permReason || 'No deploy permission' : `Roll back ${envName} to ${formatShortVersion(event.version)}`}
                          >
                            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                              <polyline points="1 4 1 10 7 10"></polyline>
                              <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path>
                            </svg>
                            Rollback
                          </button>
                        {/if}
                      </div>
                    </div>

                    <!-- Timestamps: Deployed and Retired -->
                    <div class="lifecycle-grid">
                      <!-- Deployed At -->
                      <div class="lifecycle-col">
                        <span class="col-heading">Deployed</span>
                        <div class="lifecycle-val">
                          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="meta-icon">
                            <circle cx="12" cy="12" r="10"></circle>
                            <polyline points="12 6 12 12 16 14"></polyline>
                          </svg>
                          <span class="time-main">{formatFullDate(event.deployed_at)}</span>
                          <span class="time-rel">({formatRelative(event.deployed_at)})</span>
                        </div>
                        <div class="author-row">
                          <UserAvatar name={event.author_name} size={15} />
                          <span class="author-name">{event.author_name || "System"}</span>
                        </div>
                      </div>

                      <!-- Retired At -->
                      <div class="lifecycle-col">
                        <span class="col-heading">Retired</span>
                        <div class="lifecycle-val">
                          {#if active}
                            <span class="still-active-val">
                              <span class="mini-green-dot"></span>
                              Still running {event.active_duration ? `(${event.active_duration})` : ''}
                            </span>
                          {:else if event.retired_at}
                            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="meta-icon">
                              <circle cx="12" cy="12" r="10"></circle>
                              <line x1="10" y1="15" x2="10" y2="9"></line>
                              <line x1="14" y1="15" x2="14" y2="9"></line>
                            </svg>
                            <span class="time-main">{formatFullDate(event.retired_at)}</span>
                            <span class="time-rel">({formatRelative(event.retired_at)})</span>
                          {:else if event.active_duration}
                            <span class="duration-label">Ran for {event.active_duration}</span>
                          {:else}
                            <span class="retired-label">Replaced</span>
                          {/if}
                        </div>

                        {#if !active && event.active_duration}
                          <div class="duration-badge-row">
                            <span class="duration-tag">
                              Active duration: {event.active_duration}
                            </span>
                          </div>
                        {/if}
                      </div>
                    </div>

                    <!-- Commit / PR Metadata if available -->
                    {#if event.commit_message || event.commit_sha || event.pr_number}
                      <div class="commit-meta-strip">
                        {#if event.commit_sha}
                          <code class="sha-pill" title={event.commit_sha}>
                            {event.commit_sha.slice(0, 7)}
                          </code>
                        {/if}
                        {#if event.pr_number}
                          <span class="pr-pill">PR #{event.pr_number}</span>
                        {/if}
                        {#if event.commit_message}
                          <span class="commit-msg-text" title={event.commit_message}>
                            {event.commit_message}
                          </span>
                        {/if}
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.45);
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1.25rem;
  }

  .modal-panel {
    background: #ffffff;
    border-radius: var(--radius-xl);
    box-shadow: 0 20px 45px -10px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(226, 232, 240, 0.8);
    width: 100%;
    max-width: 660px;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    animation: modalPopIn 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  }

  :global(html.dark) .modal-panel {
    background: #111827;
    box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(55, 65, 81, 0.7);
  }

  @keyframes modalPopIn {
    from {
      opacity: 0;
      transform: scale(0.97) translateY(8px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  @media (max-width: 640px) {
    .modal-backdrop {
      padding: 0;
    }
    .modal-panel {
      position: fixed;
      inset: 0;
      width: 100vw;
      height: 100vh;
      max-width: 100vw;
      max-height: 100vh;
      border-radius: 0;
    }
  }

  .modal-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid rgba(226, 232, 240, 0.8);
    background: rgba(248, 250, 252, 0.7);
  }

  :global(html.dark) .modal-header {
    background: rgba(17, 24, 39, 0.8);
    border-bottom-color: rgba(55, 65, 81, 0.7);
  }

  .header-titles {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .header-badge-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.15rem;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #10b981;
    flex-shrink: 0;
  }

  .status-dot.badge-prod {
    background: #10b981;
  }
  .status-dot.badge-preprod {
    background: #f59e0b;
  }
  .status-dot.badge-staging {
    background: #3b82f6;
  }
  .status-dot.badge-dev {
    background: #64748b;
  }
  .status-dot.badge-default {
    background: #8b5cf6;
  }

  .env-chip {
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.15rem 0.5rem;
    border-radius: var(--radius-pill);
    background: rgba(226, 232, 240, 0.7);
    color: var(--text-main);
  }

  .env-chip.badge-default {
    background: #f3e8ff;
    color: #7c3aed;
  }

  :global(html.dark) .env-chip.badge-default {
    background: rgba(139, 92, 246, 0.2);
    color: #c084fc;
  }

  :global(html.dark) .env-chip {
    background: rgba(51, 65, 85, 0.8);
    color: #f1f5f9;
  }

  .service-name-label {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-weight: 500;
  }

  .modal-title {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text-main);
    letter-spacing: -0.01em;
  }

  .modal-sub {
    font-size: 0.82rem;
    color: var(--text-muted);
    margin: 0;
  }

  .close-btn {
    background: transparent;
    border: none;
    color: var(--text-sub);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: var(--radius-pill);
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.15s ease, color 0.15s ease;
  }

  .close-btn:hover {
    color: var(--text-main);
    background: rgba(148, 163, 184, 0.15);
  }

  .modal-body {
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    overflow-y: auto;
    flex: 1;
  }

  .permission-notice,
  .error-notice {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    font-size: 0.82rem;
    padding: 0.65rem 0.9rem;
    border-radius: var(--radius-md);
  }

  .permission-notice {
    background: rgba(245, 158, 11, 0.12);
    border: 1px solid rgba(245, 158, 11, 0.35);
    color: #b45309;
  }

  :global(html.dark) .permission-notice {
    background: rgba(245, 158, 11, 0.18);
    border-color: rgba(245, 158, 11, 0.4);
    color: #fbbf24;
  }

  .error-notice {
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.3);
    color: #b91c1c;
  }

  :global(html.dark) .error-notice {
    background: rgba(239, 68, 68, 0.18);
    border-color: rgba(239, 68, 68, 0.4);
    color: #f87171;
  }

  /* Current environment summary */
  .current-env-summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    background: rgba(241, 245, 249, 0.7);
    border: 1px solid rgba(226, 232, 240, 0.8);
    padding: 0.75rem 1.1rem;
    border-radius: var(--radius-lg);
  }

  :global(html.dark) .current-env-summary {
    background: rgba(30, 41, 59, 0.55);
    border-color: rgba(51, 65, 85, 0.7);
  }

  .summary-col {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .summary-k {
    font-size: 0.7rem;
    text-transform: uppercase;
    font-weight: 600;
    letter-spacing: 0.03em;
    color: var(--text-muted);
  }

  .summary-v {
    font-size: 0.86rem;
    font-weight: 600;
    color: var(--text-main);
  }

  .version-pill-active {
    font-family: var(--font-mono);
    color: #059669;
    font-weight: 700;
  }

  :global(html.dark) .version-pill-active {
    color: #34d399;
  }

  .strategy-tag {
    font-size: 0.78rem;
    color: var(--text-sub);
  }

  .mono-text {
    font-family: var(--font-mono);
    font-size: 0.78rem;
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Confirmation Panel */
  .confirmation-panel {
    background: rgba(254, 242, 242, 0.8);
    border: 1px solid rgba(254, 202, 202, 0.9);
    border-radius: var(--radius-lg);
    padding: 1.15rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    animation: fadeIn 0.15s ease;
  }

  :global(html.dark) .confirmation-panel {
    background: rgba(69, 10, 10, 0.35);
    border-color: rgba(185, 28, 28, 0.5);
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .confirm-header {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
  }

  .confirm-icon {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: #fee2e2;
    color: #dc2626;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  :global(html.dark) .confirm-icon {
    background: rgba(185, 28, 28, 0.4);
    color: #f87171;
  }

  .confirm-text {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .confirm-title {
    font-size: 0.95rem;
    font-weight: 700;
    color: var(--text-main);
  }

  .confirm-desc {
    font-size: 0.82rem;
    color: var(--text-muted);
    line-height: 1.4;
  }

  .confirm-desc code {
    background: rgba(255, 255, 255, 0.8);
    padding: 0.1rem 0.35rem;
    border-radius: 4px;
    font-family: var(--font-mono);
    font-size: 0.8rem;
  }

  :global(html.dark) .confirm-desc code {
    background: rgba(30, 41, 59, 0.8);
  }

  .confirm-form {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .confirm-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-main);
  }

  .confirm-input {
    width: 100%;
    padding: 0.45rem 0.75rem;
    border-radius: var(--radius-md);
    border: 1px solid rgba(203, 213, 225, 0.9);
    background: #ffffff;
    font-size: 0.85rem;
    color: var(--text-main);
  }

  :global(html.dark) .confirm-input {
    background: rgba(17, 24, 39, 0.9);
    border-color: rgba(75, 85, 99, 0.8);
    color: #f8fafc;
  }

  .confirm-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.65rem;
    margin-top: 0.25rem;
  }

  .secondary-btn {
    background: transparent;
    border: 1px solid rgba(203, 213, 225, 0.8);
    color: var(--text-main);
    padding: 0.45rem 0.85rem;
    border-radius: var(--radius-pill);
    font-size: 0.82rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s ease;
  }

  :global(html.dark) .secondary-btn {
    border-color: rgba(75, 85, 99, 0.8);
    color: #e2e8f0;
  }

  .secondary-btn:hover:not(:disabled) {
    background: rgba(203, 213, 225, 0.3);
  }

  .text-link-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    padding: 0.45rem 0.5rem;
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .text-link-btn:hover:not(:disabled) {
    color: var(--text-main);
  }

  .danger-rollback-btn {
    background: #dc2626;
    color: #ffffff;
    border: none;
    border-radius: var(--radius-pill);
    padding: 0.45rem 1.1rem;
    font-size: 0.85rem;
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    cursor: pointer;
    transition: background 0.15s ease;
  }

  .danger-rollback-btn:hover:not(:disabled) {
    background: #b91c1c;
  }

  .danger-rollback-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  /* History list */
  .history-section {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .history-section-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }

  .section-title {
    font-size: 0.88rem;
    font-weight: 700;
    color: var(--text-main);
    letter-spacing: -0.01em;
  }

  .section-sub {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .timeline-list {
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
  }

  .timeline-item {
    display: flex;
    gap: 0.85rem;
    position: relative;
  }

  .timeline-track {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 14px;
    padding-top: 0.65rem;
    flex-shrink: 0;
  }

  .timeline-marker {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: #cbd5e1;
    border: 2px solid #ffffff;
    box-shadow: 0 0 0 1px #94a3b8;
    z-index: 2;
  }

  :global(html.dark) .timeline-marker {
    background: #475569;
    border-color: #111827;
    box-shadow: 0 0 0 1px #64748b;
  }

  .timeline-marker.active-marker {
    background: #10b981;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.4);
  }

  .timeline-line {
    width: 2px;
    flex: 1;
    background: #e2e8f0;
    margin-top: 4px;
  }

  :global(html.dark) .timeline-line {
    background: #334155;
  }

  .event-card {
    flex: 1;
    background: #ffffff;
    border: 1px solid rgba(226, 232, 240, 0.8);
    border-radius: var(--radius-lg);
    padding: 0.85rem 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
    transition: all 0.15s ease;
  }

  :global(html.dark) .event-card {
    background: rgba(30, 41, 59, 0.45);
    border-color: rgba(51, 65, 85, 0.7);
  }

  .timeline-item.is-active .event-card {
    border-color: rgba(16, 185, 129, 0.4);
    background: rgba(240, 253, 244, 0.4);
  }

  :global(html.dark) .timeline-item.is-active .event-card {
    background: rgba(6, 78, 59, 0.15);
    border-color: rgba(16, 185, 129, 0.35);
  }

  .timeline-item.is-selected .event-card {
    border-color: rgba(239, 68, 68, 0.5);
    background: rgba(254, 242, 242, 0.4);
  }

  :global(html.dark) .timeline-item.is-selected .event-card {
    border-color: rgba(239, 68, 68, 0.4);
    background: rgba(69, 10, 10, 0.2);
  }

  .event-card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .version-tag-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .version-badge {
    font-family: var(--font-mono);
    font-size: 0.92rem;
    font-weight: 700;
    color: var(--text-main);
  }

  .version-badge.active-version {
    color: #059669;
  }

  :global(html.dark) .version-badge.active-version {
    color: #34d399;
  }

  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.7rem;
    font-weight: 600;
    padding: 0.1rem 0.45rem;
    border-radius: var(--radius-pill);
  }

  .status-pill.active-pill {
    background: #dcfce7;
    color: #15803d;
  }

  :global(html.dark) .status-pill.active-pill {
    background: rgba(16, 185, 129, 0.2);
    color: #34d399;
  }

  .status-pill.retired-pill {
    background: rgba(226, 232, 240, 0.7);
    color: var(--text-muted);
  }

  :global(html.dark) .status-pill.retired-pill {
    background: rgba(51, 65, 85, 0.6);
    color: #94a3b8;
  }

  .pulse-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #10b981;
    display: inline-block;
  }

  .active-tag-label {
    font-size: 0.78rem;
    font-weight: 600;
    color: #059669;
    padding: 0.2rem 0.5rem;
  }

  :global(html.dark) .active-tag-label {
    color: #34d399;
  }

  .rollback-trigger-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    background: transparent;
    border: 1px solid rgba(203, 213, 225, 0.9);
    border-radius: var(--radius-pill);
    padding: 0.25rem 0.65rem;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--text-main);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  :global(html.dark) .rollback-trigger-btn {
    border-color: rgba(75, 85, 99, 0.8);
    color: #e2e8f0;
  }

  .rollback-trigger-btn:hover:not(:disabled) {
    background: #fee2e2;
    border-color: #fca5a5;
    color: #dc2626;
  }

  :global(html.dark) .rollback-trigger-btn:hover:not(:disabled) {
    background: rgba(185, 28, 28, 0.3);
    border-color: rgba(239, 68, 68, 0.5);
    color: #fca5a5;
  }

  .rollback-trigger-btn.selected {
    background: #dc2626;
    border-color: #dc2626;
    color: #ffffff;
  }

  .rollback-trigger-btn:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  /* Lifecycle grid: Deployed & Retired */
  .lifecycle-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
    background: rgba(248, 250, 252, 0.6);
    border-radius: var(--radius-md);
    padding: 0.55rem 0.75rem;
  }

  :global(html.dark) .lifecycle-grid {
    background: rgba(15, 23, 42, 0.4);
  }

  @media (max-width: 500px) {
    .lifecycle-grid {
      grid-template-columns: 1fr;
      gap: 0.5rem;
    }
  }

  .lifecycle-col {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .col-heading {
    font-size: 0.68rem;
    text-transform: uppercase;
    font-weight: 700;
    letter-spacing: 0.04em;
    color: var(--text-sub);
  }

  .lifecycle-val {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem;
    font-size: 0.78rem;
    color: var(--text-main);
  }

  .meta-icon {
    color: var(--text-sub);
    flex-shrink: 0;
  }

  .time-main {
    font-weight: 500;
  }

  .time-rel {
    font-size: 0.72rem;
    color: var(--text-muted);
  }

  .author-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-top: 0.15rem;
  }

  .author-name {
    font-size: 0.74rem;
    color: var(--text-muted);
  }

  .still-active-val {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-weight: 600;
    color: #059669;
    font-size: 0.76rem;
  }

  :global(html.dark) .still-active-val {
    color: #34d399;
  }

  .mini-green-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #10b981;
  }

  .duration-badge-row {
    margin-top: 0.15rem;
  }

  .duration-tag {
    font-size: 0.7rem;
    color: var(--text-sub);
    font-weight: 500;
  }

  .retired-label,
  .duration-label {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  /* Commit metadata strip */
  .commit-meta-strip {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    font-size: 0.75rem;
    padding-top: 0.3rem;
    border-top: 1px solid rgba(226, 232, 240, 0.6);
  }

  :global(html.dark) .commit-meta-strip {
    border-top-color: rgba(51, 65, 85, 0.5);
  }

  .sha-pill {
    background: rgba(226, 232, 240, 0.8);
    color: var(--text-main);
    font-family: var(--font-mono);
    font-size: 0.7rem;
    font-weight: 600;
    padding: 0.05rem 0.35rem;
    border-radius: 4px;
  }

  :global(html.dark) .sha-pill {
    background: rgba(51, 65, 85, 0.8);
    color: #cbd5e1;
  }

  .pr-pill {
    background: rgba(237, 233, 254, 0.9);
    color: #6d28d9;
    font-family: var(--font-mono);
    font-size: 0.7rem;
    font-weight: 600;
    padding: 0.05rem 0.35rem;
    border-radius: 4px;
  }

  :global(html.dark) .pr-pill {
    background: rgba(109, 40, 217, 0.25);
    color: #c4b5fd;
  }

  .commit-msg-text {
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 380px;
  }

  /* Result view */
  .result-view {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    padding: 2rem 1rem;
    gap: 0.85rem;
  }

  .success-icon {
    width: 52px;
    height: 52px;
    border-radius: 50%;
    background: #dcfce7;
    color: #16a34a;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  :global(html.dark) .success-icon {
    background: rgba(22, 163, 74, 0.25);
    color: #4ade80;
  }

  .success-heading {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text-main);
  }

  .success-msg {
    font-size: 0.9rem;
    color: var(--text-muted);
    max-width: 420px;
  }

  .result-meta-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
  }

  .meta-label {
    color: var(--text-sub);
    font-weight: 500;
  }

  .meta-sha {
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--text-main);
    background: rgba(226, 232, 240, 0.7);
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
  }

  .pr-link {
    color: #2563eb;
    font-weight: 600;
    text-decoration: underline;
  }

  .primary-btn {
    background: var(--accent-coral);
    color: #ffffff;
    border: none;
    border-radius: var(--radius-pill);
    padding: 0.55rem 1.4rem;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    box-shadow: none;
  }

  :global(html.dark) .primary-btn {
    background: #f8fafc;
    color: #0f172a;
  }

  .loading-state,
  .empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    padding: 2rem;
    color: var(--text-muted);
    font-size: 0.85rem;
  }

  .spinner-sm {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(255, 255, 255, 0.3);
    border-top-color: currentColor;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
    display: inline-block;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
