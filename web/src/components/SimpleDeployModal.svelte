<script lang="ts">
  import { onMount } from "svelte";
  import { fetchVersions, promote } from "../lib/api";
  import type { ServiceResponse, VersionInfo, PromotionResult } from "../types";

  interface Props {
    service: ServiceResponse;
    initialEnv?: string;
    initialVersion?: string;
    onClose: () => void;
    onSuccess?: (res: PromotionResult) => void;
  }

  let { service, initialEnv, initialVersion, onClose, onSuccess }: Props =
    $props();

  let environments = $derived(Object.keys(service.environments || {}));
  let selectedEnv = $derived(
    initialEnv || (environments.length > 0 ? environments[0] : ""),
  );

  let activeEnvView = $derived(service.environments?.[selectedEnv]);
  let currentActiveVersion = $derived(activeEnvView?.active_version || "");
  let defaultStrategy = $derived(activeEnvView?.write_strategy || "direct");

  let versions = $state<VersionInfo[]>([]);
  let loadingVersions = $state(false);
  let internalSelectedVersion = $state("");
  let selectedVersion = $derived(initialVersion || internalSelectedVersion);

  let comment = $state("");
  let submitting = $state(false);
  let error = $state("");
  let result = $state<PromotionResult | null>(null);

  let canDeploy = $derived(
    service.user_permissions?.[selectedEnv]?.can_deploy ?? true,
  );
  let permReason = $derived(
    service.user_permissions?.[selectedEnv]?.reason || "",
  );

  onMount(async () => {
    if (!initialVersion) {
      try {
        loadingVersions = true;
        const vers = await fetchVersions(service.service);
        versions = vers;
        if (!selectedVersion && vers.length > 0) {
          const next =
            vers.find((v) => v.version !== currentActiveVersion) || vers[0];
          selectedVersion = next.version;
        }
      } catch (err: any) {
        error = err.message;
      } finally {
        loadingVersions = false;
      }
    }
  });

  function formatShort(v?: string): string {
    if (!v || v === "unknown") return "none";
    if (v.includes(":")) {
      const parts = v.split(":");
      return parts[parts.length - 1];
    }
    return v;
  }

  async function handleDeploy() {
    if (!selectedVersion || !selectedEnv) return;
    submitting = true;
    error = "";
    result = null;

    try {
      const res = await promote({
        service: service.service,
        environment: selectedEnv,
        target_version: selectedVersion,
        mode: defaultStrategy,
        comment: comment.trim(),
        user: "DeployStackr",
      });
      result = res;
      if (res.mode === "pull_request" && service.environments?.[selectedEnv]) {
        service.environments[selectedEnv].pending_pr = {
          service: service.service,
          environment: selectedEnv,
          target_version: selectedVersion,
          branch: res.branch,
          pr_number: res.pr_number,
          pr_url: res.pr_url,
          user: "DeployStackr",
          created_at: new Date().toISOString(),
        };
      } else if (res.mode === "direct" && service.environments?.[selectedEnv]) {
        const prevVersion = service.environments[selectedEnv].active_version;
        service.environments[selectedEnv].active_version = selectedVersion;
        service.environments[selectedEnv].cluster_status = {
          status: 'pending',
          ready_replicas: 0,
          updated_replicas: 0,
          desired_replicas: 1,
          current_version: prevVersion,
          message: 'Waiting for GitOps rollout to start...',
        };
      }
      if (onSuccess) onSuccess(res);
    } catch (err: any) {
      error = err.message;
    } finally {
      submitting = false;
    }
  }
</script>

<div
  class="popover-backdrop"
  onclick={(e) => {
    if (e.target === e.currentTarget) onClose();
  }}
  onkeydown={(e) => e.key === "Escape" && onClose()}
  role="presentation"
>
  <div class="popover-panel" role="dialog" aria-modal="true">
    <div class="popover-header">
      <div class="header-titles">
        <h2 class="popover-title">
          Deploy to <span class="env-target-badge">{selectedEnv}</span>
        </h2>
        <span class="popover-sub"
          >{service.display_name || service.service} • One-click GitOps promotion</span
        >
      </div>
      <button
        class="close-btn"
        onclick={onClose}
        type="button"
        aria-label="Close"
      >
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
          <line x1="18" y1="6" x2="6" y2="18" />
          <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>
    </div>

    {#if result}
      <div class="result-view">
        <div class="success-icon">
          <svg
            width="24"
            height="24"
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
        <h3 class="success-heading">Promotion Complete!</h3>
        <p class="success-msg">{result.message}</p>

        {#if result.commit_sha}
          <div class="result-meta-row">
            <span class="meta-label">Commit SHA:</span>
            <code class="meta-sha">{result.commit_sha.slice(0, 7)}</code>
          </div>
        {/if}

        {#if result.pr_url}
          <div class="result-meta-row">
            <span class="meta-label">Pull Request:</span>
            <a
              href={result.pr_url}
              target="_blank"
              rel="noopener noreferrer"
              class="meta-pr-link"
            >
              #{result.pr_number} - Open on GitHub ↗
            </a>
          </div>
        {/if}

        <button class="done-btn" onclick={onClose} type="button">Done</button>
      </div>
    {:else}
      <div class="popover-body">
        {#if error}
          <div class="error-banner">
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              style="display: inline-block; vertical-align: -2px; margin-right: 6px;"
            >
              <path
                d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"
              />
              <line x1="12" y1="9" x2="12" y2="13" />
              <line x1="12" y1="17" x2="12.01" y2="17" />
            </svg>{error}
          </div>
        {/if}

        {#if !canDeploy}
          <div class="perm-warning-banner">
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
            </svg>
            <span>Deployment restricted for {selectedEnv}: {permReason}</span>
          </div>
        {/if}

        <!-- Release Version selection (only needed when going from the deploy to button) -->
        {#if !initialVersion}
          <div class="form-group">
            <label for="select-version" class="group-label">Release Version</label>
            {#if loadingVersions}
              <div class="loading-input-state">Loading candidate versions...</div>
            {:else}
              <select
                id="select-version"
                class="version-select"
                value={selectedVersion}
                onchange={(e) =>
                  (internalSelectedVersion = e.currentTarget.value)}
              >
                {#each versions as v}
                  <option value={v.version}>
                    {v.version}
                    {v.version === currentActiveVersion
                      ? "(Currently Deployed)"
                      : ""}
                  </option>
                {/each}
              </select>
            {/if}
          </div>
        {/if}

        <!-- Version Transition Banner -->
        <div class="transition-banner">
          <div class="version-stage">
            <span class="stage-label">Current ({selectedEnv})</span>
            <span class="stage-ver">{formatShort(currentActiveVersion)}</span>
          </div>
          <span class="arrow-sep">→</span>
          <div class="version-stage target">
            <span class="stage-label">Promoting to</span>
            <span class="stage-ver"
              >{formatShort(selectedVersion) || "Select"}</span
            >
          </div>
        </div>

        <!-- Strategy Pill -->
        <div class="strategy-banner">
          {#if defaultStrategy === "pull_request"}
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
              <circle cx="18" cy="18" r="3" />
              <circle cx="6" cy="6" r="3" />
              <path d="M13 6h3a2 2 0 0 1 2 2v7" />
              <line x1="6" y1="9" x2="6" y2="21" />
            </svg>
          {:else}
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
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
            </svg>
          {/if}
          <span class="strategy-text">
            {defaultStrategy === "pull_request"
              ? "Will create a branch & Pull Request"
              : "Will commit directly to branch"}
          </span>
        </div>

        <!-- Comment Field (Optional) -->
        <div class="form-group">
          <label for="deploy-comment" class="group-label">
            Comment / Release Notes
            <span class="label-hint">(optional)</span>
          </label>
          <textarea
            id="deploy-comment"
            class="comment-textarea"
            placeholder="Add context or rationale for this release promotion..."
            rows="2"
            bind:value={comment}
          ></textarea>
        </div>
      </div>

      <div class="popover-footer">
        <button
          class="cancel-btn"
          onclick={onClose}
          type="button"
          disabled={submitting}
        >
          Cancel
        </button>
        <button
          class="deploy-btn"
          onclick={handleDeploy}
          type="button"
          disabled={submitting ||
            !canDeploy ||
            !selectedVersion ||
            selectedVersion === currentActiveVersion}
        >
          {#if submitting}
            <span class="spinner-small"></span> Deploying...
          {:else if selectedVersion === currentActiveVersion}
            Active in {selectedEnv}
          {:else}
            Deploy to {selectedEnv}
          {/if}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  .popover-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(255, 255, 255, 0.45);
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1rem;
  }

  .popover-panel {
    background: rgba(255, 255, 255, 1);
    -webkit-backdrop-filter: blur(28px);
    backdrop-filter: blur(28px);
    border: none;
    border-radius: var(--radius-xl);
    box-shadow: none;
    width: 100%;
    max-width: 480px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    animation: popIn 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  }

  @keyframes popIn {
    from {
      opacity: 0;
      transform: scale(0.96) translateY(8px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  /* On Mobile: Full-Screen Popover */
  @media (max-width: 640px) {
    .popover-backdrop {
      padding: 0;
    }
    .popover-panel {
      position: fixed;
      inset: 0;
      width: 100vw;
      height: 100vh;
      max-width: 100vw;
      max-height: 100vh;
      border-radius: 0;
      border: none;
      background: #ffffff;
    }
  }

  .popover-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid rgba(220, 210, 235, 0.5);
    background: rgba(255, 255, 255, 0.5);
  }

  .header-titles {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .popover-title {
    font-size: 1.2rem;
    font-weight: 600;
    color: var(--text-main);
    letter-spacing: -0.01em;
  }

  .env-target-badge {
    color: var(--accent-coral);
    text-transform: capitalize;
  }

  .popover-sub {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .close-btn {
    background: transparent;
    border: none;
    font-size: 1.1rem;
    color: var(--text-sub);
    cursor: pointer;
    padding: 0.35rem;
    border-radius: var(--radius-pill);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .close-btn:hover {
    color: var(--text-main);
    background: rgba(100, 116, 139, 0.1);
  }

  .popover-body {
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.15rem;
    overflow-y: auto;
    flex: 1;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .group-label {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-main);
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .label-hint {
    font-size: 0.7rem;
    color: var(--text-sub);
    font-weight: 500;
  }



  .perm-warning-banner {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 0.85rem;
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.25);
    border-radius: var(--radius-md);
    color: #ef4444;
    font-size: 0.775rem;
    font-weight: 500;
  }

  .version-select {
    background: #ffffff;
    border: 1px solid rgba(203, 213, 225, 0.7);
    border-radius: var(--radius-md);
    padding: 0.65rem 0.85rem;
    font-size: 0.9rem;
    font-family: var(--font-mono);
    color: var(--text-main);
    outline: none;
    cursor: pointer;
  }

  .version-select:focus {
    border-color: #64748b;
    box-shadow: 0 0 0 3px rgba(100, 116, 139, 0.15);
  }

  .loading-input-state {
    padding: 0.65rem;
    background: rgba(240, 235, 248, 0.5);
    border-radius: var(--radius-md);
    color: var(--text-muted);
    font-size: 0.85rem;
  }

  .transition-banner {
    display: flex;
    align-items: center;
    justify-content: space-around;
    background: rgba(245, 240, 252, 0.6);
    border: 1px solid rgba(220, 210, 235, 0.6);
    border-radius: var(--radius-md);
    padding: 0.75rem;
  }

  .version-stage {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
  }

  .stage-label {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--text-sub);
  }

  .stage-ver {
    font-family: var(--font-mono);
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-main);
  }

  .version-stage.target .stage-ver {
    color: var(--accent-coral);
  }

  .arrow-sep {
    color: var(--text-sub);
    font-size: 1.1rem;
  }

  .strategy-banner {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: #ffffff;
    border: 1px solid rgba(203, 213, 225, 0.6);
    border-radius: var(--radius-sm);
    padding: 0.45rem 0.75rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    font-weight: 500;
  }

  .comment-textarea {
    background: #ffffff;
    border: 1px solid rgba(203, 213, 225, 0.7);
    border-radius: var(--radius-md);
    padding: 0.65rem 0.85rem;
    font-size: 0.85rem;
    color: var(--text-main);
    resize: vertical;
    outline: none;
    line-height: 1.4;
  }

  .comment-textarea:focus {
    border-color: #64748b;
    box-shadow: 0 0 0 3px rgba(100, 116, 139, 0.15);
  }

  .popover-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.65rem;
    padding: 1.15rem 1.5rem;
    border-top: 1px solid rgba(203, 213, 225, 0.5);
    background: rgba(255, 255, 255, 0.6);
  }

  .cancel-btn {
    background: transparent;
    border: 1px solid rgba(203, 213, 225, 0.6);
    color: var(--text-muted);
    padding: 0.55rem 1rem;
    border-radius: var(--radius-pill);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
  }

  .cancel-btn:hover:not(:disabled) {
    background: rgba(0, 0, 0, 0.05);
    color: var(--text-main);
  }

  .deploy-btn {
    background: var(--accent-coral);
    color: #ffffff;
    border: none;
    padding: 0.55rem 1.35rem;
    border-radius: var(--radius-pill);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    box-shadow: none;
    transition: all 0.15s ease;
  }

  .deploy-btn:hover:not(:disabled) {
    background: var(--accent-coral-hover);
    transform: translateY(-1px);
  }

  .deploy-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  :global(html.dark) .popover-backdrop {
    background: rgba(15, 23, 42, 0.75);
  }

  :global(html.dark) .popover-panel {
    background: #1e293b;
    border: 1px solid rgba(51, 65, 85, 0.7);
    box-shadow: 0 20px 40px -10px rgba(0, 0, 0, 0.7);
  }

  :global(html.dark) .popover-header {
    background: rgba(30, 41, 59, 0.7);
    border-bottom-color: rgba(51, 65, 85, 0.6);
  }

  :global(html.dark) .popover-footer {
    background: rgba(30, 41, 59, 0.7);
    border-top-color: rgba(51, 65, 85, 0.6);
  }

  :global(html.dark) .version-select {
    background: #0f172a;
    border-color: rgba(51, 65, 85, 0.8);
    color: #f1f5f9;
  }

  :global(html.dark) .transition-banner {
    background: rgba(15, 23, 42, 0.5);
    border-color: rgba(51, 65, 85, 0.6);
  }

  :global(html.dark) .strategy-banner {
    background: rgba(15, 23, 42, 0.4);
    border-color: rgba(51, 65, 85, 0.5);
  }

  :global(html.dark) .comment-textarea {
    background: #0f172a;
    border-color: rgba(51, 65, 85, 0.8);
    color: #f1f5f9;
  }

  :global(html.dark) .cancel-btn {
    background: rgba(51, 65, 85, 0.5);
    color: #cbd5e1;
    border-color: rgba(51, 65, 85, 0.7);
  }

  :global(html.dark) .cancel-btn:hover:not(:disabled) {
    background: rgba(51, 65, 85, 0.8);
    color: #ffffff;
  }

  :global(html.dark) .deploy-btn {
    color: #0f172a;
  }

  .spinner-small {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(255, 255, 255, 0.4);
    border-top-color: #ffffff;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .error-banner {
    background: #fee2e2;
    border: 1px solid #fecaca;
    color: #b91c1c;
    padding: 0.75rem;
    border-radius: var(--radius-md);
    font-size: 0.85rem;
  }

  /* Result view */
  .result-view {
    padding: 2.5rem 1.5rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
    text-align: center;
  }

  .success-icon {
    width: 48px;
    height: 48px;
    background: #dcfce7;
    border: 2px solid #86efac;
    color: #15803d;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    font-weight: bold;
  }

  .success-heading {
    font-size: 1.25rem;
    font-weight: 800;
    color: var(--text-main);
  }

  .success-msg {
    font-size: 0.9rem;
    color: var(--text-muted);
  }

  .result-meta-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.85rem;
  }

  .meta-label {
    color: var(--text-sub);
  }

  .meta-sha {
    font-family: var(--font-mono);
    background: rgba(241, 245, 249, 0.85);
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
    color: var(--text-main);
    font-weight: 700;
  }

  .meta-pr-link {
    color: var(--text-main);
    font-weight: 700;
    text-decoration: underline;
  }

  .meta-pr-link:hover {
    color: #000000;
  }

  .done-btn {
    margin-top: 1rem;
    background: var(--text-main);
    color: #ffffff;
    border: none;
    padding: 0.55rem 1.5rem;
    border-radius: var(--radius-pill);
    font-size: 0.85rem;
    font-weight: 700;
    cursor: pointer;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
