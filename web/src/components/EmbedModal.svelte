<script lang="ts">
    type Props = {
        serviceName: string;
        envName?: string;
        onClose: () => void;
    };

    let {serviceName, envName = '', onClose}: Props = $props();

    const IS_DEMO = import.meta.env.VITE_DEMO_MODE === 'true';
    const BASE_URL = import.meta.env.BASE_URL || '/';

    let theme = $state<'auto' | 'light' | 'dark'>('auto');
    let copied = $state(false);

    let embedUrl = $derived.by(() => {
        const origin = typeof window !== 'undefined' ? window.location.origin : '';
        const params = new URLSearchParams({
            service: serviceName
        });
        if(envName) {
            params.set('env', envName);
        }
        if(theme !== 'auto') {
            params.set('theme', theme);
        }
        return IS_DEMO
            ? `${origin}${BASE_URL}#embed?${params.toString()}`
            : `${origin}${BASE_URL}embed?${params.toString()}`;
    });

    let iframeSnippet = $derived(
        `<iframe src="${embedUrl}" width="420" height="84" frameborder="0" allowtransparency="true" style="border:0;background:transparent;"></iframe>`
    );

    function copySnippet() {
        navigator.clipboard.writeText(iframeSnippet).then(() => {
            copied = true;
            window.setTimeout(() => {
                copied = false;
            }, 2000);
        });
    }

    function handleBackdropClick(e: MouseEvent) {
        if(e.target === e.currentTarget) {
            onClose();
        }
    }

    function handleKeydown(e: KeyboardEvent) {
        if(e.key === 'Escape') {
            onClose();
        }
    }
</script>

<svelte:window onkeydown={handleKeydown}/>

<div
    class="embed-modal-backdrop"
    role="dialog"
    aria-modal="true"
    aria-label="Embed deployment widget"
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
>
    <div class="embed-modal">
        <div class="embed-modal-header">
            <div>
                <h3>Embed Deployment Widget</h3>
                <p>{serviceName} ({envName || 'All environments'})</p>
            </div>
            <button class="close-btn" type="button" onclick={onClose} aria-label="Close">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="18" y1="6" x2="6" y2="18"></line>
                    <line x1="6" y1="6" x2="18" y2="18"></line>
                </svg>
            </button>
        </div>

        <div class="embed-modal-body">
            <div class="theme-row">
                <span class="field-label">Theme</span>
                <div class="theme-pills">
                    <button type="button" class:active={theme === 'auto'} onclick={() => theme = 'auto'}>Auto</button>
                    <button type="button" class:active={theme === 'light'} onclick={() => theme = 'light'}>Light</button>
                    <button type="button" class:active={theme === 'dark'} onclick={() => theme = 'dark'}>Dark</button>
                </div>
            </div>

            <div class="preview-box">
                <span class="field-label">Preview</span>
                <iframe
                    src={embedUrl}
                    title="Embed widget preview"
                    width="100%"
                    height="84"
                    frameborder="0"
                    style="border:0;background:transparent;"
                ></iframe>
            </div>

            <div class="snippet-box">
                <div class="snippet-header">
                    <span class="field-label">HTML Embed Code</span>
                    <button class="copy-btn" type="button" onclick={copySnippet}>
                        {copied ? 'Copied!' : 'Copy HTML'}
                    </button>
                </div>
                <pre><code>{iframeSnippet}</code></pre>
            </div>
        </div>
    </div>
</div>

<style>
    .embed-modal-backdrop {
        position: fixed;
        inset: 0;
        background: rgba(15, 23, 42, 0.45);
        backdrop-filter: blur(4px);
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 16px;
        z-index: 1200;
    }

    .embed-modal {
        width: 100%;
        max-width: 520px;
        background: var(--glass-bg-active, #ffffff);
        color: var(--text-main, #0f172a);
        border: 1px solid rgba(148, 163, 184, 0.25);
        border-radius: 16px;
        box-shadow: 0 20px 50px rgba(15, 23, 42, 0.22);
        overflow: hidden;
    }

    :global(html.dark) .embed-modal {
        background: #0f172a;
        border-color: rgba(255, 255, 255, 0.14);
    }

    .embed-modal-header {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        padding: 18px 20px 12px;
        border-bottom: 1px solid rgba(148, 163, 184, 0.18);
    }

    .embed-modal-header h3 {
        margin: 0;
        font-size: 16px;
        font-weight: 700;
    }

    .embed-modal-header p {
        margin: 2px 0 0;
        font-size: 12.5px;
        color: var(--text-muted, #64748b);
    }

    .close-btn {
        background: transparent;
        border: none;
        color: var(--text-muted, #64748b);
        cursor: pointer;
        padding: 4px;
        border-radius: 6px;
    }

    .close-btn:hover {
        color: var(--text-main, #0f172a);
        background: rgba(148, 163, 184, 0.14);
    }

    .embed-modal-body {
        padding: 16px 20px 20px;
        display: flex;
        flex-direction: column;
        gap: 16px;
    }

    .field-label {
        font-size: 12px;
        font-weight: 600;
        color: var(--text-muted, #64748b);
    }

    .theme-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
    }

    .theme-pills {
        display: flex;
        gap: 6px;
    }

    .theme-pills button {
        padding: 4px 10px;
        font-size: 12px;
        font-weight: 600;
        border-radius: 9999px;
        border: 1px solid rgba(148, 163, 184, 0.3);
        background: transparent;
        color: var(--text-muted, #64748b);
        cursor: pointer;
    }

    .theme-pills button.active {
        background: var(--text-main, #0f172a);
        color: #ffffff;
        border-color: transparent;
    }

    :global(html.dark) .theme-pills button.active {
        background: #f8fafc;
        color: #0f172a;
    }

    .preview-box,
    .snippet-box {
        display: flex;
        flex-direction: column;
        gap: 8px;
    }

    .snippet-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
    }

    .copy-btn {
        padding: 4px 10px;
        font-size: 12px;
        font-weight: 600;
        border-radius: 6px;
        border: none;
        background: var(--text-main, #0f172a);
        color: #ffffff;
        cursor: pointer;
    }

    :global(html.dark) .copy-btn {
        background: #f8fafc;
        color: #0f172a;
    }

    pre {
        margin: 0;
        padding: 10px 12px;
        border-radius: 8px;
        background: rgba(15, 23, 42, 0.06);
        font-family: var(--font-mono);
        font-size: 11.5px;
        overflow-x: auto;
        white-space: pre-wrap;
        word-break: break-all;
    }

    :global(html.dark) pre {
        background: rgba(255, 255, 255, 0.08);
    }
</style>
