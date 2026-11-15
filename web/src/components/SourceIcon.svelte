<script lang="ts">
    type Props = {
        type?: string;
        size?: number;
        class?: string;
        title?: string;
    };

    let {
        type = '',
        size = 13,
        class: className = '',
        title = '',
    }: Props = $props();

    const normalized = $derived((type || '').trim().toLowerCase().replace(/[-_\s]/g, ''));
    const isGithub = $derived(normalized.includes('github') || normalized === 'gh');
    const isOci = $derived(normalized === 'oci' || normalized === 'docker' || normalized === 'container');
    const isGitlab = $derived(normalized.includes('gitlab'));
    const isGit = $derived(normalized.includes('git') && !isGithub && !isGitlab);
</script>

{#if isGithub}
    <svg
        class="source-icon github {className}"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
        title={title || 'GitHub'}
    >
        <path
            fill-rule="evenodd"
            clip-rule="evenodd"
            d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
        />
    </svg>
{:else if isOci}
    <svg
        class="source-icon oci {className}"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
        title={title || 'OCI'}
    >
        <path d="M0 0v24h24V0zm20.547 20.431H3.448V3.573h17.104V20.43zm-5.155-9.979h3.436v8.255h-3.436zm0-5.16h3.436v3.436h-3.436zm-6.789 9.976V8.732h5.074v-3.44H5.164v13.415h8.513v-3.44Z"/>
    </svg>
{:else if isGitlab}
    <svg
        class="source-icon gitlab {className}"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
        title={title || 'GitLab'}
    >
        <path d="M22.65 14.39L20.6 8.04a.79.79 0 0 0-.3-.41.8.8 0 0 0-.52-.08.8.8 0 0 0-.44.25.82.82 0 0 0-.17.51l-1.39 4.27H6.22l-1.39-4.27a.82.82 0 0 0-.17-.51.8.8 0 0 0-.44-.25.8.8 0 0 0-.52.08.79.79 0 0 0-.3.41L1.35 14.39a.84.84 0 0 0 .3.94l10.05 7.3a.58.58 0 0 0 .6 0l10.05-7.3a.84.84 0 0 0 .3-.94z"/>
    </svg>
{:else if isGit}
    <svg
        class="source-icon git {className}"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
        title={title || 'Git'}
    >
        <path d="M2.6 10.59L8.38 4.8a1.69 1.69 0 0 1 2.38 0l1.44 1.44-2.32 2.32c-.43-.16-.94-.12-1.33.15a1.71 1.71 0 0 0-.25 2.38l-2.02 2.02a1.72 1.72 0 0 0-1.07-.37c-.95 0-1.72.77-1.72 1.72s.77 1.72 1.72 1.72 1.72-.77 1.72-1.72c0-.36-.12-.7-.32-1l2.02-2.02c.4.2.85.25 1.25.13l2.25 2.25a1.72 1.72 0 1 0 1.2-1.2l-2.2-2.2.06-.06a1.71 1.71 0 0 0 .14-2.31l2.28-2.28 5.78 5.78a1.69 1.69 0 0 1 0 2.38l-5.78 5.78a1.69 1.69 0 0 1-2.38 0L2.6 12.97a1.69 1.69 0 0 1 0-2.38z"/>
    </svg>
{:else}
    <svg
        class="source-icon default {className}"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
        title={title || type}
    >
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
        <polyline points="3.27 6.96 12 12.01 20.73 6.96"/>
        <line x1="12" y1="22.08" x2="12" y2="12"/>
    </svg>
{/if}

<style>
    .source-icon {
        display: inline-block;
        flex-shrink: 0;
        vertical-align: middle;
    }
</style>
