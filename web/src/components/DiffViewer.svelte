<script lang="ts">
  import type { ChangelogDiff } from '../types';

  export interface DiffEnvOption {
    name: string;
    activeVersion?: string;
  }

  interface Props {
    diff: ChangelogDiff | null;
    loading?: boolean;
    environments?: DiffEnvOption[];
    selectedEnvironment?: string;
    targetVersion?: string;
    repoUrl?: string;
    onSelectEnvironment?: (envName: string) => void;
    onClose?: () => void;
  }

  let {
    diff,
    loading = false,
    environments = [],
    selectedEnvironment = '',
    targetVersion = '',
    repoUrl = '',
    onSelectEnvironment,
    onClose,
  }: Props = $props();

  function isReleaseCommit(subject?: string): boolean {
    if (!subject) return false;
    const s = subject.trim().toLowerCase();
    return (
      s.startsWith('chore(release') ||
      s.startsWith('release:') ||
      s.startsWith('release(') ||
      s.startsWith('build(release') ||
      s.startsWith('chore: release') ||
      s.startsWith('bump version') ||
      s.startsWith('version bump') ||
      s.startsWith('bump to') ||
      /^release\s+v?[0-9]/i.test(s) ||
      /^bump\s+(to\s+)?v?[0-9]/i.test(s) ||
      /^v?[0-9]+\.[0-9]+\.[0-9]+(\s*release)?$/i.test(s)
    );
  }

  let filteredCommits = $derived(
    (diff?.commits || []).filter((c) => !isReleaseCommit(c.subject))
  );

  let isSameVersion = $derived(
    Boolean(
      diff?.from_version &&
      diff?.to_version &&
      (diff.from_version === diff.to_version ||
        diff.from_version.replace(/^v/, '') === diff.to_version.replace(/^v/, ''))
    )
  );

  function formatRelative(isoStr?: string): string {
    if (!isoStr) return '';
    const d = new Date(isoStr);
    const now = new Date();
    const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);
    if (diffSec < 60) return 'just now';
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
    if (diffSec < 2592000) return `${Math.floor(diffSec / 86400)}d ago`;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  function getInitials(name?: string): string {
    if (!name) return '•';
    const parts = name.trim().split(/\s+/);
    if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
    return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
  }

  function formatDiffStat(stat?: string): string {
    if (!stat) return '';
    const lastLine = stat.split('\n').map(l => l.trim()).filter(Boolean).pop() || stat;
    return lastLine.trim();
  }
</script>

<div class="diff-viewer">
  {#if environments.length > 0 || onClose}
    <div class="diff-toolbar">
      {#if environments.length > 0}
        <div class="diff-env-picker">
          <span class="toolbar-label">Diff against:</span>
          <div class="env-tabs" role="tablist">
            {#each environments as env}
              {@const isSelected = selectedEnvironment === env.name}
              <button
                type="button"
                role="tab"
                aria-selected={isSelected}
                class="env-tab-btn"
                class:selected={isSelected}
                onclick={() => onSelectEnvironment?.(env.name)}
              >
                <span class="tab-env-name">{env.name}</span>
                <span class="tab-env-ver" class:empty={!env.activeVersion || env.activeVersion === 'n/a'}>
                  {env.activeVersion || 'n/a'}
                </span>
              </button>
            {/each}
          </div>
        </div>
      {/if}

      <div class="toolbar-right-actions">
        {#if diff?.diff_stat}
          {@const formattedStat = formatDiffStat(diff.diff_stat)}
          {#if formattedStat}
            <div class="diff-stat-badge" title={formattedStat}>
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                <polyline points="14 2 14 8 20 8"></polyline>
                <line x1="16" y1="13" x2="8" y2="13"></line>
                <line x1="16" y1="17" x2="8" y2="17"></line>
                <polyline points="10 9 9 9 8 9"></polyline>
              </svg>
              <span class="diff-stat-text">{formattedStat}</span>
            </div>
          {/if}
        {/if}

        {#if onClose}
          <button
            type="button"
            class="diff-close-btn"
            onclick={onClose}
            title="Close diff pane"
            aria-label="Close diff pane"
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
            <span>Close</span>
          </button>
        {/if}
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="diff-loading-state">
      <div class="spinner-sm"></div>
      <span>Computing git diff network...</span>
    </div>
  {:else if !diff}
    <div class="diff-empty-state">
      <span>No diff information available for this environment.</span>
    </div>
  {:else if isSameVersion}
    <div class="diff-status-card up-to-date">
      <div class="status-circle-check">✓</div>
      <div class="status-info">
        <div class="status-headline">
          <span class="mono-code">{diff.to_version}</span>
          <span class="status-chip success">Currently Deployed</span>
        </div>
        <p class="status-desc">
          {selectedEnvironment
            ? `${selectedEnvironment.charAt(0).toUpperCase() + selectedEnvironment.slice(1)} is currently running ${diff.to_version}. No new commits to deploy.`
            : 'The selected environment is already deployed to this version.'}
        </p>
      </div>
    </div>
  {:else if filteredCommits.length < 1}
    <div class="diff-status-card empty-changes">
      <div class="status-circle-info">○</div>
      <div class="status-info">
        <div class="status-headline">
          <span class="mono-code">{diff.to_version}</span>
          {#if diff.from_version}
            <span class="status-sep">←</span>
            <span class="mono-code">{diff.from_version}</span>
          {/if}
          <span class="status-chip">No Commits</span>
        </div>
        <p class="status-desc">
          No functional commits found between <code>{diff.from_version || 'initial'}</code> and <code>{diff.to_version}</code>.
        </p>
      </div>
    </div>
  {:else}
    <!-- Git Network Graph View -->
    <div class="git-network">
      <div class="network-rail"></div>

      <!-- Target Node: Candidate version to deploy (or rollback target) -->
      <div class="network-node target-node" class:rollback={diff.is_rollback}>
        <div class="node-circle target-circle" class:rollback={diff.is_rollback} title={diff.is_rollback ? "Rollback Target" : "Deploy Candidate"}>
          <div class="inner-dot"></div>
        </div>
        <div class="node-details">
          <span class="ver-badge target" class:rollback={diff.is_rollback}>{diff.to_version}</span>
          <span class="node-caption">
            {diff.is_rollback ? 'Rollback Target' : 'Deploy Candidate'}
          </span>
        </div>
      </div>

      <!-- Commits List along the Git Rail -->
      <div class="network-commits-list">
        {#each filteredCommits as c}
          <div class="commit-row" class:rollback={diff.is_rollback}>
            <div class="node-circle commit-circle" class:rollback={diff.is_rollback} title="Commit {c.sha.slice(0, 7)}"></div>
            <div class="commit-card">
              <div class="commit-meta-line">
                <span class="commit-sha" title={c.sha}>{c.sha.slice(0, 7)}</span>
                <span class="author-tag">
                  <span class="author-avatar">{getInitials(c.author_name)}</span>
                  <span class="author-name">{c.author_name}</span>
                </span>
                <span class="meta-separator">•</span>
                <span class="commit-time">{formatRelative(c.timestamp)}</span>
                {#if c.pr_number}
                  {@const prLink = repoUrl ? `${repoUrl.replace(/\/$/, '')}/pull/${c.pr_number}` : ''}
                  {#if prLink}
                    <a
                      href={prLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      class="pr-badge is-link"
                      title="Open PR #{c.pr_number} on GitHub"
                      onclick={(e) => e.stopPropagation()}
                    >
                      #{c.pr_number}
                    </a>
                  {:else}
                    <span class="pr-badge">#{c.pr_number}</span>
                  {/if}
                {/if}
                {#if diff.is_rollback}
                  <span class="rollback-commit-badge" title="This commit will be removed/superseded if rolled back">will be removed</span>
                {/if}
              </div>
              <div class="commit-subject-line">{c.subject}</div>
            </div>
          </div>
        {/each}
      </div>

      <!-- Base Node: Currently running in environment -->
      <div class="network-node base-node">
        <div class="node-circle base-circle" title="Base Version">
          <div class="inner-dot"></div>
        </div>
        <div class="node-details">
          <span class="ver-badge base">{diff.from_version || 'Initial'}</span>
          <span class="node-caption">
            {selectedEnvironment ? `Currently running in ${selectedEnvironment}` : 'Currently Running'}
          </span>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .diff-viewer {
    background: #ffffff;
    border: 1px solid rgba(203, 213, 225, 0.7);
    border-radius: var(--radius-lg, 12px);
    padding: 1.15rem 1.25rem;
    box-shadow: 0 4px 16px -2px rgba(15, 23, 42, 0.05);
    color: var(--text-main);
    display: flex;
    flex-direction: column;
    gap: 1rem;
    box-sizing: border-box;
    width: 100%;
  }

  :global(html.dark) .diff-viewer {
    background: #1e293b;
    border-color: rgba(51, 65, 85, 0.75);
    box-shadow: 0 4px 20px -2px rgba(0, 0, 0, 0.35);
  }

  /* Toolbar */
  .diff-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
    padding-bottom: 0.85rem;
    border-bottom: 1px solid rgba(226, 232, 240, 0.8);
  }

  :global(html.dark) .diff-toolbar {
    border-bottom-color: rgba(51, 65, 85, 0.6);
  }

  .diff-env-picker {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    flex-wrap: wrap;
  }

  .toolbar-label {
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }

  .env-tabs {
    display: inline-flex;
    align-items: center;
    background: rgba(148, 163, 184, 0.12);
    padding: 3px;
    border-radius: 8px;
    gap: 2px;
  }

  :global(html.dark) .env-tabs {
    background: rgba(15, 23, 42, 0.5);
  }

  .env-tab-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.3rem 0.65rem;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--text-muted);
    font-family: inherit;
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .env-tab-btn:hover {
    color: var(--text-main);
    background: rgba(255, 255, 255, 0.5);
  }

  :global(html.dark) .env-tab-btn:hover {
    background: rgba(51, 65, 85, 0.4);
    color: #f1f5f9;
  }

  .env-tab-btn.selected {
    background: #ffffff;
    color: var(--text-main);
    font-weight: 600;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  }

  :global(html.dark) .env-tab-btn.selected {
    background: #334155;
    color: #f8fafc;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
  }

  .tab-env-name {
    text-transform: capitalize;
  }

  .tab-env-ver {
    font-family: var(--font-mono, monospace);
    font-size: 0.68rem;
    padding: 0.08rem 0.35rem;
    border-radius: 4px;
    background: rgba(148, 163, 184, 0.18);
    color: var(--text-main);
  }

  :global(html.dark) .tab-env-ver {
    background: rgba(15, 23, 42, 0.4);
    color: #cbd5e1;
  }

  .tab-env-ver.empty {
    opacity: 0.6;
  }

  .diff-stat-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    font-family: var(--font-mono, monospace);
    font-size: 0.72rem;
    padding: 0.25rem 0.6rem;
    border-radius: var(--radius-pill);
    background: rgba(148, 163, 184, 0.12);
    color: var(--text-muted);
    border: 1px solid rgba(148, 163, 184, 0.2);
    white-space: nowrap;
    max-width: 360px;
    flex-shrink: 0;
  }

  .diff-stat-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  :global(html.dark) .diff-stat-badge {
    background: rgba(15, 23, 42, 0.45);
    border-color: rgba(51, 65, 85, 0.6);
  }

  .toolbar-right-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    margin-left: auto;
  }

  .diff-close-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.28rem 0.65rem;
    border-radius: var(--radius-pill);
    border: 1px solid rgba(203, 213, 225, 0.8);
    background: rgba(255, 255, 255, 0.8);
    color: var(--text-muted);
    font-size: 0.72rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .diff-close-btn:hover {
    background: #ffffff;
    border-color: #94a3b8;
    color: var(--text-main);
  }

  :global(html.dark) .diff-close-btn {
    background: rgba(30, 41, 59, 0.7);
    border-color: rgba(51, 65, 85, 0.8);
    color: #94a3b8;
  }

  :global(html.dark) .diff-close-btn:hover {
    background: rgba(51, 65, 85, 0.95);
    border-color: rgba(100, 116, 139, 0.8);
    color: #f1f5f9;
  }

  /* States */
  .diff-loading-state,
  .diff-empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.65rem;
    padding: 2rem 1rem;
    color: var(--text-muted);
    font-size: 0.825rem;
  }

  .spinner-sm {
    width: 18px;
    height: 18px;
    border: 2px solid rgba(99, 102, 241, 0.2);
    border-top-color: #6366f1;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  /* Status Cards (Up to date / empty) */
  .diff-status-card {
    display: flex;
    align-items: flex-start;
    gap: 0.85rem;
    padding: 1rem 1.15rem;
    border-radius: 8px;
    background: rgba(241, 245, 249, 0.75);
    border: 1px solid rgba(203, 213, 225, 0.6);
  }

  :global(html.dark) .diff-status-card {
    background: rgba(15, 23, 42, 0.4);
    border-color: rgba(51, 65, 85, 0.6);
  }

  .status-circle-check {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: #dcfce7;
    color: #15803d;
    font-weight: 700;
    font-size: 0.8rem;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  :global(html.dark) .status-circle-check {
    background: rgba(16, 185, 129, 0.2);
    color: #34d399;
  }

  .status-circle-info {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: #e2e8f0;
    color: #475569;
    font-weight: 700;
    font-size: 0.85rem;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  :global(html.dark) .status-circle-info {
    background: rgba(51, 65, 85, 0.5);
    color: #cbd5e1;
  }

  .status-info {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .status-headline {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .status-sep {
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  .mono-code {
    font-family: var(--font-mono, monospace);
    font-weight: 600;
    font-size: 0.85rem;
    color: var(--text-main);
  }

  .status-chip {
    font-size: 0.68rem;
    font-weight: 600;
    padding: 0.12rem 0.45rem;
    border-radius: var(--radius-pill);
    background: #e2e8f0;
    color: #475569;
  }

  .status-chip.success {
    background: #dcfce7;
    color: #15803d;
  }

  :global(html.dark) .status-chip.success {
    background: rgba(16, 185, 129, 0.2);
    color: #34d399;
  }

  .status-desc {
    margin: 0;
    font-size: 0.78rem;
    color: var(--text-muted);
    line-height: 1.4;
  }

  /* Git Network Graph */
  .git-network {
    position: relative;
    padding-left: 28px;
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
  }

  .network-rail {
    position: absolute;
    left: 11px;
    top: 14px;
    bottom: 14px;
    width: 2px;
    background: #cbd5e1;
    z-index: 1;
  }

  :global(html.dark) .network-rail {
    background: #475569;
  }

  /* Terminal Nodes */
  .network-node {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.65rem;
    min-height: 28px;
    z-index: 2;
  }

  .node-circle {
    box-sizing: border-box;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    z-index: 2;
  }

  .target-circle {
    position: absolute;
    left: -23px;
    top: 5px;
    width: 14px;
    height: 14px;
    border: 2.5px solid #3b82f6;
    background: #ffffff;
  }

  :global(html.dark) .target-circle {
    background: #1e293b;
    border-color: #60a5fa;
  }

  .target-circle .inner-dot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: #3b82f6;
  }

  :global(html.dark) .target-circle .inner-dot {
    background: #60a5fa;
  }

  .base-circle {
    position: absolute;
    left: -23px;
    top: 5px;
    width: 14px;
    height: 14px;
    border: 2.5px solid #10b981;
    background: #ffffff;
  }

  :global(html.dark) .base-circle {
    background: #1e293b;
    border-color: #34d399;
  }

  .base-circle .inner-dot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: #10b981;
  }

  :global(html.dark) .base-circle .inner-dot {
    background: #34d399;
  }

  .node-details {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .ver-badge {
    font-family: var(--font-mono, monospace);
    font-size: 0.8rem;
    font-weight: 700;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
  }

  .ver-badge.target {
    background: #dbeafe;
    color: #1e40af;
    border: 1px solid rgba(59, 130, 246, 0.3);
  }

  .ver-badge.target.rollback {
    background: #fef3c7;
    color: #b45309;
    border-color: rgba(245, 158, 11, 0.4);
  }

  :global(html.dark) .ver-badge.target {
    background: rgba(59, 130, 246, 0.2);
    color: #93c5fd;
    border-color: rgba(59, 130, 246, 0.4);
  }

  :global(html.dark) .ver-badge.target.rollback {
    background: rgba(245, 158, 11, 0.2);
    color: #fcd34d;
    border-color: rgba(245, 158, 11, 0.4);
  }

  .target-circle.rollback {
    border-color: #f59e0b;
  }

  .target-circle.rollback .inner-dot {
    background: #f59e0b;
  }

  :global(html.dark) .target-circle.rollback {
    border-color: #fbbf24;
  }

  :global(html.dark) .target-circle.rollback .inner-dot {
    background: #fbbf24;
  }

  .commit-circle.rollback {
    border-color: #f59e0b;
  }

  .rollback-commit-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: rgba(239, 68, 68, 0.12);
    color: #dc2626;
    border: 1px solid rgba(239, 68, 68, 0.25);
    padding: 0.08rem 0.35rem;
    border-radius: var(--radius-pill, 9999px);
  }

  :global(html.dark) .rollback-commit-badge {
    background: rgba(239, 68, 68, 0.2);
    color: #f87171;
    border-color: rgba(239, 68, 68, 0.35);
  }

  .ver-badge.base {
    background: #d1fae5;
    color: #065f46;
    border: 1px solid rgba(16, 185, 129, 0.3);
  }

  :global(html.dark) .ver-badge.base {
    background: rgba(16, 185, 129, 0.2);
    color: #6ee7b7;
    border-color: rgba(16, 185, 129, 0.4);
  }

  .node-caption {
    font-size: 0.72rem;
    font-weight: 600;
    color: var(--text-muted);
  }

  /* Commits List */
  .network-commits-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.25rem 0;
  }

  .commit-row {
    position: relative;
    display: flex;
    align-items: flex-start;
  }

  .commit-circle {
    position: absolute;
    left: -21px;
    top: 14px;
    width: 10px;
    height: 10px;
    border: 2px solid #64748b;
    background: #ffffff;
    transition: all 0.15s ease;
  }

  :global(html.dark) .commit-circle {
    background: #1e293b;
    border-color: #94a3b8;
  }

  .commit-card {
    background: rgba(248, 250, 252, 0.85);
    border: 1px solid rgba(226, 232, 240, 0.85);
    border-radius: 8px;
    padding: 0.55rem 0.85rem;
    width: 100%;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    transition: all 0.15s ease;
  }

  :global(html.dark) .commit-card {
    background: rgba(15, 23, 42, 0.45);
    border-color: rgba(51, 65, 85, 0.5);
  }

  .commit-row:hover .commit-circle {
    border-color: #3b82f6;
    background: #3b82f6;
    transform: scale(1.25);
  }

  :global(html.dark) .commit-row:hover .commit-circle {
    border-color: #60a5fa;
    background: #60a5fa;
  }

  .commit-row:hover .commit-card {
    background: #ffffff;
    border-color: #94a3b8;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  }

  :global(html.dark) .commit-row:hover .commit-card {
    background: rgba(15, 23, 42, 0.7);
    border-color: #64748b;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25);
  }

  .commit-meta-line {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.73rem;
    flex-wrap: wrap;
  }

  .commit-sha {
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
    background: rgba(100, 116, 139, 0.12);
    padding: 0.1rem 0.35rem;
    border-radius: 4px;
  }

  .author-tag {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .author-avatar {
    width: 17px;
    height: 17px;
    border-radius: 50%;
    background: #e2e8f0;
    color: #475569;
    font-size: 0.6rem;
    font-weight: 700;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  :global(html.dark) .author-avatar {
    background: #334155;
    color: #cbd5e1;
  }

  .author-name {
    font-weight: 600;
    color: var(--text-main);
  }

  .meta-separator {
    color: var(--text-sub);
    font-size: 0.65rem;
  }

  .commit-time {
    color: var(--text-muted);
  }

  .pr-badge {
    background: #ede9fe;
    color: #7c3aed;
    font-size: 0.68rem;
    font-weight: 600;
    padding: 0.1rem 0.45rem;
    border-radius: 10px;
    margin-left: auto;
    text-decoration: none;
    transition: all 0.15s ease;
  }

  .pr-badge.is-link {
    cursor: pointer;
  }

  .pr-badge.is-link:hover {
    text-decoration: underline;
    background: #ddd6fe;
  }

  :global(html.dark) .pr-badge {
    background: rgba(124, 58, 237, 0.25);
    color: #c4b5fd;
  }

  :global(html.dark) .pr-badge.is-link:hover {
    background: rgba(124, 58, 237, 0.4);
  }

  .commit-subject-line {
    font-size: 0.8125rem;
    color: var(--text-main);
    line-height: 1.35;
    word-break: break-word;
  }
</style>
