<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchVersions, fetchDiff, promote } from '../lib/api';
  import type { ServiceResponse, VersionInfo, ChangelogDiff, PromotionResult } from '../types';
  import DiffViewer from './DiffViewer.svelte';

  interface Props {
    service: ServiceResponse;
    initialEnv?: string;
    onClose: () => void;
    onSuccess?: (res: PromotionResult) => void;
  }

  let { service, initialEnv, onClose, onSuccess }: Props = $props();

  let environments = $derived(Object.keys(service.environments || {}));
  let selectedEnv = $state('');

  $effect(() => {
    if (!selectedEnv) {
      selectedEnv = initialEnv || (environments.length > 0 ? environments[0] : '');
    }
  });

  let activeEnvView = $derived(service.environments?.[selectedEnv]);
  let currentActiveVersion = $derived(activeEnvView?.active_version || '');
  let defaultStrategy = $derived(activeEnvView?.write_strategy || 'direct');

  let versions = $state<VersionInfo[]>([]);
  let loadingVersions = $state(true);
  let selectedVersion = $state('');

  let diff = $state<ChangelogDiff | null>(null);
  let loadingDiff = $state(false);

  let userName = $state('GitOps Engineer');
  let userEmail = $state('engineer@company.com');
  let customMode = $state('');

  let submitting = $state(false);
  let error = $state('');
  let result = $state<PromotionResult | null>(null);

  function getContextRepoUrl(repo?: string): string {
    if(!repo) return '';
    let r = repo.trim();
    if(r.startsWith('git@github.com:')) {
      return `https://github.com/${r.replace('git@github.com:', '').replace(/\.git$/, '')}`;
    }
    if(r.startsWith('git@gitlab.com:')) {
      return `https://gitlab.com/${r.replace('git@gitlab.com:', '').replace(/\.git$/, '')}`;
    }
    if(r.startsWith('http://') || r.startsWith('https://')) {
      return r.replace(/\.git$/, '');
    }
    if(!r.includes('://') && r.split('/').filter(Boolean).length === 2) {
      return `https://github.com/${r.replace(/\.git$/, '')}`;
    }
    return r;
  }

  onMount(async () => {
    try {
      loadingVersions = true;
      const vers = await fetchVersions(service.service);
      versions = vers;
      if (vers.length > 0) {
        // Default to newest candidate different from current, or first
        const next = vers.find(v => v.version !== currentActiveVersion) || vers[0];
        selectedVersion = next.version;
      }
    } catch (err: any) {
      error = err.message;
    } finally {
      loadingVersions = false;
    }
  });

  $effect(() => {
    if(selectedVersion && currentActiveVersion && selectedVersion !== currentActiveVersion) {
      diff = null;
      loadDiff(currentActiveVersion, selectedVersion);
    } else if(selectedVersion && currentActiveVersion && selectedVersion === currentActiveVersion) {
      diff = {
        from_version: currentActiveVersion,
        to_version: selectedVersion,
        commits: [],
        diff_stat: '0 files changed'
      };
    } else {
      diff = null;
    }
  });

  async function loadDiff(from: string, to: string) {
    diff = null;
    loadingDiff = true;
    try {
      diff = await fetchDiff(service.service, from, to);
    } catch(e) {
      console.warn('Failed to load diff', e);
      diff = null;
    } finally {
      loadingDiff = false;
    }
  }

  async function handlePromote() {
    if (!selectedVersion) return;
    submitting = true;
    error = '';
    result = null;

    try {
      const mode = customMode || defaultStrategy;
      const res = await promote({
        service: service.service,
        environment: selectedEnv,
        target_version: selectedVersion,
        mode: mode,
        user: userName,
        email: userEmail
      });
      result = res;
      if (onSuccess) {
        onSuccess(res);
      }
    } catch (err: any) {
      error = err.message;
    } finally {
      submitting = false;
    }
  }
</script>

<div
  class="modal-backdrop"
  onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}
  onkeydown={(e) => e.key === 'Escape' && onClose()}
  role="presentation"
>
  <div class="modal-content" role="dialog" aria-modal="true">
    <div class="modal-header">
      <div>
        <h2 class="title">Promote {service.service}</h2>
        <p class="subtitle">Update target deployment manifest via GitOps engine</p>
      </div>
      <button class="close-btn" onclick={onClose} aria-label="Close">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>

    {#if result}
      <div class="result-box">
        <div class="result-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"/>
          </svg>
        </div>
        <div class="result-details">
          <h3>Promotion Triggered Successfully!</h3>
          <p>{result.message}</p>
          {#if result.commit_sha}
            <div class="meta-row">
              <span class="label">Commit SHA:</span>
              <code>{result.commit_sha}</code>
            </div>
          {/if}
          {#if result.pr_url}
            <div class="meta-row">
              <span class="label">Pull Request:</span>
              <a href={result.pr_url} target="_blank" rel="noopener noreferrer">
                #{result.pr_number} - Open on GitHub ↗
              </a>
            </div>
          {/if}
        </div>
        <button class="primary-btn" onclick={onClose}>Done</button>
      </div>
    {:else}
      <div class="form-body">
        {#if error}
          <div class="error-banner">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display: inline-block; vertical-align: -2px; margin-right: 6px;">
              <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/>
              <line x1="12" y1="9" x2="12" y2="13"/>
              <line x1="12" y1="17" x2="12.01" y2="17"/>
            </svg>{error}
          </div>
        {/if}

        <div class="form-row grid-2">
          <div class="field">
            <label for="target-env">Target Environment</label>
            <select id="target-env" bind:value={selectedEnv}>
              {#each environments as env}
                <option value={env}>{env}</option>
              {/each}
            </select>
          </div>

          <div class="field">
            <label for="release-version">Release Candidate</label>
            {#if loadingVersions}
              <div class="input-placeholder">Loading available versions...</div>
            {:else}
              <select id="release-version" bind:value={selectedVersion}>
                {#each versions as v}
                  <option value={v.version}>
                    {v.version} {v.version === currentActiveVersion ? '(Active)' : ''}
                  </option>
                {/each}
              </select>
            {/if}
          </div>
        </div>

        <div class="version-transition">
          <div class="state-pill current">
            <span class="pill-label">CURRENT IN {selectedEnv.toUpperCase()}</span>
            <span class="version-num">{currentActiveVersion || 'None'}</span>
          </div>
          <div class="transition-arrow">→</div>
          <div class="state-pill target">
            <span class="pill-label">TARGET VERSION</span>
            <span class="version-num">{selectedVersion || 'Select'}</span>
          </div>
        </div>

        <div class="field">
          <div class="field-label">Target Manifest & In-Place Selector</div>
          <div class="manifest-info">
            <span class="file-tag">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display: inline-block; vertical-align: -1px; margin-right: 4px;">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
              </svg>{activeEnvView?.target_file || 'manifest'}
            </span>
            <code class="path-tag">{activeEnvView?.target_path || 'path'}</code>
            <span class="strategy-badge" class:pr={defaultStrategy === 'pull_request'}>
              {defaultStrategy === 'pull_request' ? 'Pull Request' : 'Direct Commit'}
            </span>
          </div>
        </div>

        <div class="field">
          <div class="field-label">Changelog & Commits Diff</div>
          <DiffViewer
            {diff}
            loading={loadingDiff}
            repoUrl={getContextRepoUrl(service.context?.repo || service.source?.repository)}
          />
        </div>

        <div class="form-row grid-2">
          <div class="field">
            <label for="user-name">Author Name</label>
            <input id="user-name" type="text" bind:value={userName} placeholder="Your name" />
          </div>
          <div class="field">
            <label for="user-email">Author Email</label>
            <input id="user-email" type="email" bind:value={userEmail} placeholder="your@email.com" />
          </div>
        </div>
      </div>

      <div class="modal-footer">
        <button class="secondary-btn" onclick={onClose} disabled={submitting}>Cancel</button>
        <button
          class="primary-btn"
          onclick={handlePromote}
          disabled={submitting || !selectedVersion || selectedVersion === currentActiveVersion}
        >
          {#if submitting}
            <span class="spinner-small"></span> Mutating & Deploying...
          {:else if defaultStrategy === 'pull_request'}
            Create Pull Request
          {:else}
            Execute Direct Commit
          {/if}
        </button>
      </div>
    {/if}
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.75);
    -webkit-backdrop-filter: blur(4px);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1rem;
  }
  .modal-content {
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 12px;
    width: 100%;
    max-width: 680px;
    box-shadow: none;
    display: flex;
    flex-direction: column;
    max-height: 90vh;
    overflow-y: auto;
  }
  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid #1e293b;
  }
  .title {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 700;
    color: #f8fafc;
  }
  .subtitle {
    margin: 0.25rem 0 0 0;
    font-size: 0.8125rem;
    color: #94a3b8;
  }
  .close-btn {
    background: transparent;
    border: none;
    color: #94a3b8;
    font-size: 1.25rem;
    cursor: pointer;
    padding: 0.25rem;
  }
  .close-btn:hover {
    color: #f8fafc;
  }
  .form-body {
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }
  .grid-2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  label, .field-label {
    font-size: 0.8125rem;
    font-weight: 600;
    color: #cbd5e1;
  }
  select, input {
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 6px;
    padding: 0.6rem 0.75rem;
    color: #f8fafc;
    font-size: 0.875rem;
    outline: none;
  }
  select:focus, input:focus {
    border-color: #38bdf8;
  }
  .input-placeholder {
    padding: 0.6rem;
    background: #1e293b;
    border-radius: 6px;
    color: #64748b;
    font-size: 0.875rem;
  }
  .version-transition {
    display: flex;
    align-items: center;
    justify-content: space-around;
    background: #1e293b50;
    border: 1px solid #334155;
    border-radius: 8px;
    padding: 0.75rem;
  }
  .state-pill {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    align-items: center;
  }
  .pill-label {
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.05em;
    color: #64748b;
  }
  .version-num {
    font-family: monospace;
    font-size: 1rem;
    font-weight: 700;
    color: #f8fafc;
  }
  .state-pill.target .version-num {
    color: #38bdf8;
  }
  .transition-arrow {
    font-size: 1.25rem;
    color: #64748b;
  }
  .manifest-info {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.8125rem;
    background: #1e293b60;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    border: 1px solid #334155;
    flex-wrap: wrap;
  }
  .file-tag {
    color: #cbd5e1;
    font-weight: 500;
  }
  .path-tag {
    background: #0f172a;
    padding: 0.2rem 0.4rem;
    border-radius: 4px;
    color: #a7f3d0;
  }
  .strategy-badge {
    margin-left: auto;
    font-size: 0.75rem;
    font-weight: 600;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    background: #0284c725;
    color: #38bdf8;
  }
  .strategy-badge.pr {
    background: rgba(100, 116, 139, 0.2);
    color: #cbd5e1;
  }
  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
    padding: 1.25rem 1.5rem;
    border-top: 1px solid #1e293b;
    background: #0b1120;
    border-bottom-left-radius: 12px;
    border-bottom-right-radius: 12px;
  }
  .primary-btn {
    background: #0284c7;
    color: #ffffff;
    border: none;
    border-radius: 6px;
    padding: 0.6rem 1.25rem;
    font-weight: 600;
    font-size: 0.875rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .primary-btn:hover:not(:disabled) {
    background: #0369a1;
  }
  .primary-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .secondary-btn {
    background: transparent;
    border: 1px solid #334155;
    color: #cbd5e1;
    border-radius: 6px;
    padding: 0.6rem 1rem;
    font-size: 0.875rem;
    cursor: pointer;
  }
  .secondary-btn:hover:not(:disabled) {
    background: #1e293b;
  }
  .spinner-small {
    width: 14px;
    height: 14px;
    border: 2px solid #ffffff40;
    border-top-color: #ffffff;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  .error-banner {
    background: #ef444420;
    border: 1px solid #ef4444;
    color: #fca5a5;
    padding: 0.75rem;
    border-radius: 6px;
    font-size: 0.875rem;
  }
  .result-box {
    padding: 2.5rem 1.5rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.25rem;
    text-align: center;
  }
  .result-icon {
    width: 48px;
    height: 48px;
    background: #10b98125;
    border: 2px solid #10b981;
    color: #10b981;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    font-weight: bold;
  }
  .result-details h3 {
    margin: 0;
    color: #f8fafc;
  }
  .result-details p {
    color: #94a3b8;
    margin: 0.5rem 0 1rem 0;
    font-size: 0.9rem;
  }
  .meta-row {
    display: flex;
    gap: 0.5rem;
    justify-content: center;
    align-items: center;
    font-size: 0.85rem;
    margin-top: 0.25rem;
  }
  .meta-row .label {
    color: #64748b;
  }
  .meta-row a {
    color: #38bdf8;
    text-decoration: none;
    font-weight: 600;
  }
  .meta-row a:hover {
    text-decoration: underline;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
