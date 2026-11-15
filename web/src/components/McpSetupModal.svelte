<script lang="ts">
  type Props = {
    provider?: string;
    onClose: () => void;
  };

  let { provider = '', onClose }: Props = $props();

  let mcpUrl = $derived(
    typeof window !== 'undefined' ? `${window.location.origin}/mcp` : '/mcp'
  );

  let clientConfig = $derived(
    JSON.stringify(
      {
        mcpServers: {
          deploystackr: {
            url: mcpUrl,
            headers: {
              Authorization: 'Bearer YOUR_GITHUB_TOKEN'
            }
          }
        }
      },
      null,
      2
    )
  );

  let copiedUrl = $state(false);
  let copiedConfig = $state(false);

  function portal(node: HTMLElement) {
    if(typeof document !== 'undefined') {
      document.body.appendChild(node);
    }
    return {
      destroy() {
        node.remove();
      }
    };
  }

  function copyText(text: string, which: 'url' | 'config') {
    if(typeof navigator === 'undefined' || !navigator.clipboard) return;
    navigator.clipboard.writeText(text).then(() => {
      if(which === 'url') {
        copiedUrl = true;
        setTimeout(() => (copiedUrl = false), 1600);
      } else {
        copiedConfig = true;
        setTimeout(() => (copiedConfig = false), 1600);
      }
    });
  }

  function handleKeydown(e: KeyboardEvent) {
    if(e.key === 'Escape') {
      onClose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div
  use:portal
  class="mcp-modal-backdrop"
  role="presentation"
  onclick={(e) => e.target === e.currentTarget && onClose()}
>
  <div class="mcp-modal" role="dialog" aria-modal="true" aria-labelledby="mcp-modal-title">
    <div class="mcp-modal__header">
      <div>
        <h2 id="mcp-modal-title">MCP Server Connection</h2>
        <p class="mcp-modal__subtitle">
          Connect AI coding agents via Streamable HTTP to inspect promotion state and trigger version promotions.
        </p>
      </div>
      <button class="mcp-modal__close" onclick={onClose} type="button" aria-label="Close">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>

    <div class="mcp-modal__body">
      <div class="mcp-field">
        <div class="mcp-field__label">Endpoint URL (Streamable HTTP)</div>
        <div class="mcp-copy-row">
          <code class="mcp-url-code">{mcpUrl}</code>
          <button
            class="mcp-copy-btn"
            onclick={() => copyText(mcpUrl, 'url')}
            type="button"
          >
            {copiedUrl ? 'Copied' : 'Copy URL'}
          </button>
        </div>
      </div>

      <div class="mcp-field">
        <div class="mcp-field__label">Authentication</div>
        <div class="mcp-auth-box">
          <p>
            Pass a GitHub Personal Access Token in the <code>Authorization: Bearer &lt;GITHUB_TOKEN&gt;</code> header (or <code>X-GitHub-Token</code>).
          </p>
          <p class="mcp-auth-note">
            DeployStackr resolves your GitHub user, teams, and repository permissions from the token to enforce RBAC and attribute every promotion commit and PR to you.
          </p>
          {#if provider === 'dev'}
            <p class="mcp-auth-dev">
              Dev auth is currently active on this instance; production deployments require a valid GitHub token.
            </p>
          {/if}
        </div>
      </div>

      <div class="mcp-field">
        <div class="mcp-field__header">
          <span class="mcp-field__label">MCP Client Configuration</span>
          <button
            class="mcp-copy-btn"
            onclick={() => copyText(clientConfig, 'config')}
            type="button"
          >
            {copiedConfig ? 'Copied' : 'Copy JSON'}
          </button>
        </div>
        <pre class="mcp-code-block">{clientConfig}</pre>
      </div>

      <div class="mcp-field">
        <div class="mcp-field__label">Available Tools</div>
        <div class="mcp-tools-grid">
          <div class="mcp-tool-item"><code>list_services</code><span>Discover services &amp; permissions</span></div>
          <div class="mcp-tool-item"><code>get_deployed</code><span>Active versions &amp; cluster status</span></div>
          <div class="mcp-tool-item"><code>get_deploy_queue</code><span>Pending PRs &amp; candidate versions</span></div>
          <div class="mcp-tool-item"><code>get_version_diff</code><span>Commit logs &amp; diff summary</span></div>
          <div class="mcp-tool-item"><code>get_history</code><span>Paginated promotion audit log</span></div>
          <div class="mcp-tool-item"><code>promote</code><span>Direct commit or Pull Request</span></div>
        </div>
      </div>
    </div>
  </div>
</div>

<style>
  .mcp-modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 10000;
    background: rgba(15, 23, 42, 0.55);
    backdrop-filter: blur(6px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
  }

  .mcp-modal {
    width: 100%;
    max-width: 600px;
    max-height: 90vh;
    overflow-y: auto;
    background: #ffffff;
    color: #0f172a;
    border: 1px solid rgba(0, 0, 0, 0.1);
    border-radius: 16px;
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.25);
    padding: 1.25rem 1.35rem;
  }

  :global(html.dark) .mcp-modal {
    background: #0f172a;
    color: #f8fafc;
    border-color: rgba(255, 255, 255, 0.12);
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.65);
  }

  .mcp-modal__header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    padding-bottom: 0.9rem;
    border-bottom: 1px solid rgba(0, 0, 0, 0.08);
  }

  :global(html.dark) .mcp-modal__header {
    border-bottom-color: rgba(255, 255, 255, 0.08);
  }

  .mcp-modal__header h2 {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 700;
  }

  .mcp-modal__subtitle {
    margin: 0.25rem 0 0;
    font-size: 0.8rem;
    color: #64748b;
    line-height: 1.4;
  }

  :global(html.dark) .mcp-modal__subtitle {
    color: #94a3b8;
  }

  .mcp-modal__close {
    background: transparent;
    border: 1px solid rgba(0, 0, 0, 0.1);
    color: #64748b;
    width: 30px;
    height: 30px;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    flex-shrink: 0;
  }

  :global(html.dark) .mcp-modal__close {
    border-color: rgba(255, 255, 255, 0.12);
    color: #94a3b8;
  }

  .mcp-modal__close:hover {
    color: #0f172a;
    background: rgba(0, 0, 0, 0.05);
  }

  :global(html.dark) .mcp-modal__close:hover {
    color: #f8fafc;
    background: rgba(255, 255, 255, 0.08);
  }

  .mcp-modal__body {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding-top: 1rem;
  }

  .mcp-field__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.4rem;
  }

  .mcp-field__label {
    font-size: 0.72rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #64748b;
    margin-bottom: 0.4rem;
  }

  .mcp-field__header .mcp-field__label {
    margin-bottom: 0;
  }

  :global(html.dark) .mcp-field__label {
    color: #94a3b8;
  }

  .mcp-copy-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.55rem 0.7rem;
    border-radius: 9px;
    background: #f8fafc;
    border: 1px solid rgba(0, 0, 0, 0.08);
  }

  :global(html.dark) .mcp-copy-row {
    background: rgba(255, 255, 255, 0.04);
    border-color: rgba(255, 255, 255, 0.1);
  }

  .mcp-url-code {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.8rem;
    word-break: break-all;
    color: #0284c7;
  }

  :global(html.dark) .mcp-url-code {
    color: #38bdf8;
  }

  .mcp-copy-btn {
    padding: 0.35rem 0.65rem;
    border-radius: 6px;
    border: 1px solid rgba(59, 130, 246, 0.3);
    background: rgba(59, 130, 246, 0.1);
    color: #2563eb;
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
    flex-shrink: 0;
  }

  :global(html.dark) .mcp-copy-btn {
    color: #60a5fa;
    background: rgba(59, 130, 246, 0.16);
  }

  .mcp-copy-btn:hover {
    background: rgba(59, 130, 246, 0.2);
  }

  .mcp-auth-box {
    padding: 0.7rem 0.85rem;
    border-radius: 9px;
    background: #f8fafc;
    border: 1px solid rgba(0, 0, 0, 0.08);
    font-size: 0.8rem;
    line-height: 1.45;
  }

  :global(html.dark) .mcp-auth-box {
    background: rgba(255, 255, 255, 0.04);
    border-color: rgba(255, 255, 255, 0.1);
  }

  .mcp-auth-box p {
    margin: 0;
  }

  .mcp-auth-box p + p {
    margin-top: 0.4rem;
  }

  .mcp-auth-note {
    color: #64748b;
    font-size: 0.76rem;
  }

  :global(html.dark) .mcp-auth-note {
    color: #94a3b8;
  }

  .mcp-auth-dev {
    font-size: 0.75rem;
    color: #b45309;
  }

  :global(html.dark) .mcp-auth-dev {
    color: #fbbf24;
  }

  .mcp-code-block {
    margin: 0;
    padding: 0.75rem 0.85rem;
    border-radius: 9px;
    background: #0f172a;
    color: #e2e8f0;
    border: 1px solid rgba(255, 255, 255, 0.1);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.76rem;
    line-height: 1.45;
    overflow-x: auto;
  }

  .mcp-tools-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.45rem;
  }

  @media (max-width: 540px) {
    .mcp-tools-grid {
      grid-template-columns: 1fr;
    }
  }

  .mcp-tool-item {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0.45rem 0.6rem;
    border-radius: 8px;
    background: #f8fafc;
    border: 1px solid rgba(0, 0, 0, 0.07);
    font-size: 0.74rem;
  }

  :global(html.dark) .mcp-tool-item {
    background: rgba(255, 255, 255, 0.03);
    border-color: rgba(255, 255, 255, 0.08);
  }

  .mcp-tool-item code {
    font-weight: 700;
    color: #7c3aed;
  }

  :global(html.dark) .mcp-tool-item code {
    color: #c084fc;
  }

  .mcp-tool-item span {
    color: #64748b;
  }

  :global(html.dark) .mcp-tool-item span {
    color: #94a3b8;
  }
</style>
