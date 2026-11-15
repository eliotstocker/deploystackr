<script lang="ts">
  import { onMount } from 'svelte';
  import type { ServiceResponse, VersionInfo } from '../types';

  interface Props {
    versionInfo: VersionInfo;
    service: ServiceResponse;
    prs?: number[];
    onClose: () => void;
    onDeploy?: (service: ServiceResponse, envName?: string, targetVersion?: string) => void;
  }

  let { versionInfo, service, prs = [], onClose, onDeploy }: Props = $props();

  let copied = $state(false);

  function getServiceRepo(svc: ServiceResponse): string {
    if(svc.context?.repo) return svc.context.repo;
    if(svc.source?.repository && (svc.source.type === 'github_releases' || svc.source.repository.includes('/'))) {
      return svc.source.repository;
    }
    return '';
  }

  function getPrUrl(repo: string, prNumber: number | string): string {
    if(!repo || !prNumber) return '';
    let r = repo.trim();
    if(r.startsWith('git@github.com:')) {
      r = r.replace('git@github.com:', '').replace(/\.git$/, '');
    } else if(r.startsWith('https://github.com/')) {
      r = r.replace('https://github.com/', '').replace(/\.git$/, '');
    }
    if(r.includes('gitlab')) {
      return `https://gitlab.com/${r}/-/merge_requests/${prNumber}`;
    }
    return `https://github.com/${r}/pull/${prNumber}`;
  }

  function escapeHtml(str: string): string {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  function linkifyText(str: string, repo: string): string {
    if(!str) return '';
    const escaped = escapeHtml(str);
    const withUrls = escaped.replace(
      /https?:\/\/(?:github\.com\/[^/\s]+\/[^/\s]+\/pull\/|gitlab\.com\/[^/\s]+\/[^/\s]+\/-\/merge_requests\/)(\d+)/gi,
      (match, prNum) => `<a href="${match}" target="_blank" rel="noopener noreferrer" class="changelog-inline-pr">#${prNum}</a>`
    );
    if(!repo) return withUrls;
    const basePrUrl = getPrUrl(repo, '__PR__');
    return withUrls.replace(
      /(^|[\s(])#(\d+)(?=[\s).,:]|$)/g,
      (match, prefix, prNum) => `${prefix}<a href="${basePrUrl.replace('__PR__', prNum)}" target="_blank" rel="noopener noreferrer" class="changelog-inline-pr">#${prNum}</a>`
    );
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  });

  function formatDate(isoStr?: string): string {
    if (!isoStr) return '';
    try {
      const d = new Date(isoStr);
      return d.toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric'
      });
    } catch {
      return '';
    }
  }

  async function handleCopy() {
    if (!versionInfo.changelog) return;
    try {
      await navigator.clipboard.writeText(versionInfo.changelog);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      // Ignore clipboard write failures silently
    }
  }

  interface ParsedBlock {
    type: 'header' | 'list' | 'paragraph';
    level?: number;
    text?: string;
    items?: string[];
  }

  function parseMarkdown(md?: string): ParsedBlock[] {
    if (!md) return [];
    const lines = md.split('\n');
    const blocks: ParsedBlock[] = [];
    let currentList: string[] = [];

    function flushList() {
      if (currentList.length > 0) {
        blocks.push({ type: 'list', items: [...currentList] });
        currentList = [];
      }
    }

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) {
        flushList();
        continue;
      }

      if (line.startsWith('#')) {
        flushList();
        const match = line.match(/^(#+)\s+(.+)$/);
        if (match) {
          blocks.push({
            type: 'header',
            level: match[1].length,
            text: match[2]
          });
        } else {
          blocks.push({ type: 'paragraph', text: line });
        }
      } else if (line.startsWith('- ') || line.startsWith('* ')) {
        currentList.push(line.replace(/^[-*]\s+/, ''));
      } else {
        flushList();
        blocks.push({ type: 'paragraph', text: line });
      }
    }

    flushList();
    return blocks;
  }

  let parsedBlocks = $derived(parseMarkdown(versionInfo.changelog));
</script>

<div
  class="popover-backdrop"
  onclick={onClose}
  role="presentation"
>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="popover-panel"
    onclick={(e) => e.stopPropagation()}
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    aria-label="Release Notes for {versionInfo.version}"
  >
    <!-- Modal Header -->
    <div class="modal-header">
      <div class="header-titles">
        <div class="version-badge-row">
          <span class="version-tag">{versionInfo.version}</span>
          {#if versionInfo.prerelease}
            <span class="prerelease-tag">Pre-release</span>
          {/if}
        </div>
        <h2 class="modal-title">Release Notes</h2>
        <div class="modal-subtitle">
          <span class="service-name">{service.display_name || service.service}</span>
          {#if versionInfo.released_at}
            <span class="dot">•</span>
            <span class="released-date">Published {formatDate(versionInfo.released_at)}</span>
          {/if}
        </div>
      </div>

      <button
        class="close-btn"
        onclick={onClose}
        type="button"
        title="Close (Esc)"
        aria-label="Close"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>

    <!-- Modal Body / Changelog Content -->
    <div class="modal-body">
      {#if versionInfo.description && versionInfo.description !== versionInfo.version}
        <div class="release-summary-card">
          <span class="summary-label">Release Title</span>
          <span class="summary-text">{versionInfo.description}</span>
        </div>
      {/if}

      {#if prs && prs.length > 0}
        <div class="release-prs-card">
          <div class="prs-card-header">
            <span class="prs-card-title">Included Pull Requests</span>
            <span class="prs-card-count">{prs.length}</span>
          </div>
          <div class="prs-pill-grid">
            {#each prs as pr}
              {@const prUrl = getPrUrl(getServiceRepo(service), pr)}
              {#if prUrl}
                <a
                  href={prUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="modal-pr-link-pill"
                  title="Open PR #{pr} on GitHub"
                >
                  <span>#{pr}</span>
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                    <polyline points="15 3 21 3 21 9"></polyline>
                    <line x1="10" y1="14" x2="21" y2="3"></line>
                  </svg>
                </a>
              {:else}
                <span class="modal-pr-link-pill">#{pr}</span>
              {/if}
            {/each}
          </div>
        </div>
      {/if}

      {#if parsedBlocks.length === 0}
        <div class="empty-changelog">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
            <polyline points="14 2 14 8 20 8"></polyline>
            <line x1="16" y1="13" x2="8" y2="13"></line>
            <line x1="16" y1="17" x2="8" y2="17"></line>
          </svg>
          <p>No detailed changelog provided for this release candidate.</p>
        </div>
      {:else}
        <div class="changelog-content">
          {#each parsedBlocks as block}
            {#if block.type === 'header'}
              <h3 class="changelog-header level-{block.level}">{block.text}</h3>
            {:else if block.type === 'list'}
              <ul class="changelog-list">
                {#each block.items || [] as item}
                  <li class="changelog-item">
                    <span class="list-bullet"></span>
                    <span class="item-text">{@html linkifyText(item, getServiceRepo(service))}</span>
                  </li>
                {/each}
              </ul>
            {:else if block.type === 'paragraph'}
              <p class="changelog-p">{@html linkifyText(block.text || '', getServiceRepo(service))}</p>
            {/if}
          {/each}
        </div>
      {/if}
    </div>

    <!-- Modal Footer -->
    <div class="modal-footer">
      <div class="footer-left">
        {#if versionInfo.changelog}
          <button
            class="copy-btn"
            onclick={handleCopy}
            type="button"
            title="Copy raw release notes to clipboard"
          >
            {#if copied}
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
                <polyline points="20 6 9 17 4 12"></polyline>
              </svg>
              <span>Copied!</span>
            {:else}
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
              </svg>
              <span>Copy Notes</span>
            {/if}
          </button>
        {/if}
      </div>

      <div class="footer-right">
        <button
          class="cancel-btn"
          onclick={onClose}
          type="button"
        >
          Close
        </button>
        {#if onDeploy}
          <button
            class="deploy-btn"
            onclick={() => {
              onDeploy(service, undefined, versionInfo.version);
              onClose();
            }}
            type="button"
          >
            Deploy {versionInfo.version}
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .popover-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.55);
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1.5rem;
  }

  .popover-panel {
    background: rgba(255, 255, 255, 0.95);
    -webkit-backdrop-filter: blur(28px);
    backdrop-filter: blur(28px);
    border: 1px solid rgba(226, 232, 240, 0.8);
    border-radius: var(--radius-xl);
    box-shadow: none;
    width: 100%;
    max-width: 620px;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    animation: popIn 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  }

  :global(html.dark) .popover-panel {
    background: rgba(15, 23, 42, 0.92);
    border: 1px solid rgba(51, 65, 85, 0.7);
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

  .modal-header {
    padding: 1.25rem 1.5rem 1rem;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    border-bottom: 1px solid rgba(226, 232, 240, 0.7);
  }

  :global(html.dark) .modal-header {
    border-bottom-color: rgba(51, 65, 85, 0.6);
  }

  .version-badge-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-bottom: 0.3rem;
  }

  .version-tag {
    font-family: var(--font-mono);
    font-size: 0.78rem;
    font-weight: 600;
    padding: 0.15rem 0.5rem;
    border-radius: var(--radius-pill);
    background: rgba(100, 116, 139, 0.12);
    color: var(--text-main);
  }

  :global(html.dark) .version-tag {
    background: rgba(148, 163, 184, 0.2);
    color: #cbd5e1;
  }

  .prerelease-tag {
    font-size: 0.68rem;
    font-weight: 600;
    padding: 0.15rem 0.45rem;
    border-radius: var(--radius-pill);
    background: #fef3c7;
    color: #b45309;
  }

  :global(html.dark) .prerelease-tag {
    background: rgba(245, 158, 11, 0.2);
    color: #fbbf24;
  }

  .modal-title {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 600;
    color: var(--text-main);
    letter-spacing: -0.02em;
  }

  .modal-subtitle {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.8rem;
    color: var(--text-muted);
    margin-top: 0.25rem;
  }

  .service-name {
    font-weight: 600;
    color: var(--text-main);
  }

  .close-btn {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s ease;
    flex-shrink: 0;
  }

  .close-btn:hover {
    background: rgba(0, 0, 0, 0.05);
    color: var(--text-main);
  }

  :global(html.dark) .close-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .modal-body {
    padding: 1.25rem 1.5rem;
    overflow-y: auto;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .release-summary-card {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: 0.75rem 1rem;
    background: rgba(241, 245, 249, 0.7);
    border-left: 3px solid #64748b;
    border-radius: var(--radius-md);
  }

  :global(html.dark) .release-summary-card {
    background: rgba(30, 41, 59, 0.5);
    border-left-color: #94a3b8;
  }

  .summary-label {
    font-size: 0.65rem;
    text-transform: uppercase;
    font-weight: 600;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }

  .summary-text {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--text-main);
  }

  .empty-changelog {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 2.5rem 1rem;
    gap: 0.75rem;
    color: var(--text-sub);
    text-align: center;
    font-size: 0.88rem;
  }

  .changelog-content {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  .changelog-header {
    margin: 0.5rem 0 0.15rem;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-main);
    letter-spacing: -0.01em;
  }

  .changelog-header.level-1,
  .changelog-header.level-2 {
    font-size: 1.05rem;
    border-bottom: 1px solid rgba(226, 232, 240, 0.6);
    padding-bottom: 0.35rem;
    margin-top: 0.75rem;
  }

  .changelog-list {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .changelog-item {
    display: flex;
    align-items: flex-start;
    gap: 0.55rem;
    font-size: 0.84rem;
    line-height: 1.45;
    color: var(--text-main);
  }

  .list-bullet {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: #64748b;
    margin-top: 0.45rem;
    flex-shrink: 0;
  }

  .changelog-p {
    margin: 0;
    font-size: 0.84rem;
    line-height: 1.5;
    color: var(--text-main);
  }

  .modal-footer {
    padding: 0.9rem 1.5rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-top: 1px solid rgba(226, 232, 240, 0.7);
    background: rgba(255, 255, 255, 0.6);
  }

  :global(html.dark) .modal-footer {
    background: rgba(15, 23, 42, 0.6);
    border-top-color: rgba(51, 65, 85, 0.6);
  }

  .copy-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.4rem 0.75rem;
    border-radius: var(--radius-md);
    background: rgba(0, 0, 0, 0.04);
    border: 1px solid rgba(200, 210, 225, 0.6);
    color: var(--text-main);
    font-size: 0.78rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  :global(html.dark) .copy-btn {
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(71, 85, 105, 0.6);
  }

  .copy-btn:hover {
    background: rgba(100, 116, 139, 0.1);
    border-color: rgba(100, 116, 139, 0.4);
    color: var(--text-main);
  }

  .footer-right {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .cancel-btn {
    padding: 0.45rem 0.95rem;
    border-radius: var(--radius-md);
    background: transparent;
    border: 1px solid rgba(200, 210, 225, 0.6);
    color: var(--text-main);
    font-size: 0.82rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .cancel-btn:hover {
    background: rgba(0, 0, 0, 0.05);
  }

  :global(html.dark) .cancel-btn {
    border-color: rgba(71, 85, 105, 0.6);
  }

  :global(html.dark) .cancel-btn:hover {
    background: rgba(255, 255, 255, 0.08);
  }

  .deploy-btn {
    padding: 0.45rem 1.1rem;
    border-radius: var(--radius-md);
    background: var(--text-main);
    border: none;
    color: #ffffff;
    font-size: 0.82rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .deploy-btn:hover {
    background: #334155;
  }

  :global(html.dark) .deploy-btn {
    background: #f8fafc;
    color: #0f172a;
  }

  :global(html.dark) .deploy-btn:hover {
    background: #e2e8f0;
  }

  .release-prs-card {
    background: rgba(124, 58, 237, 0.06);
    border: 1px solid rgba(124, 58, 237, 0.15);
    border-radius: 8px;
    padding: 0.75rem 1rem;
    margin-bottom: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  :global(html.dark) .release-prs-card {
    background: rgba(124, 58, 237, 0.12);
    border-color: rgba(168, 85, 247, 0.25);
  }

  .prs-card-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .prs-card-title {
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-main);
  }

  .prs-card-count {
    font-size: 0.65rem;
    font-weight: 700;
    padding: 0.05rem 0.4rem;
    border-radius: var(--radius-pill);
    background: rgba(124, 58, 237, 0.2);
    color: #7c3aed;
  }

  :global(html.dark) .prs-card-count {
    color: #c4b5fd;
    background: rgba(124, 58, 237, 0.35);
  }

  .prs-pill-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .modal-pr-link-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    font-family: var(--font-mono);
    font-size: 0.72rem;
    font-weight: 600;
    color: #7c3aed;
    background: #ffffff;
    border: 1px solid rgba(124, 58, 237, 0.25);
    padding: 0.2rem 0.5rem;
    border-radius: var(--radius-pill);
    text-decoration: none;
    transition: all 0.15s ease;
  }

  .modal-pr-link-pill:hover {
    background: #ede9fe;
    border-color: #7c3aed;
    text-decoration: underline;
  }

  :global(html.dark) .modal-pr-link-pill {
    background: rgba(30, 41, 59, 0.8);
    color: #c4b5fd;
    border-color: rgba(168, 85, 247, 0.35);
  }

  :global(html.dark) .modal-pr-link-pill:hover {
    background: rgba(124, 58, 237, 0.25);
    border-color: #a855f7;
  }

  :global(.changelog-inline-pr) {
    font-family: var(--font-mono);
    font-weight: 600;
    color: #7c3aed;
    text-decoration: none;
    background: rgba(124, 58, 237, 0.08);
    padding: 0.05rem 0.3rem;
    border-radius: 4px;
    transition: all 0.15s ease;
  }

  :global(.changelog-inline-pr:hover) {
    text-decoration: underline;
    background: rgba(124, 58, 237, 0.16);
  }

  :global(html.dark .changelog-inline-pr) {
    color: #c4b5fd;
    background: rgba(124, 58, 237, 0.2);
  }
</style>
