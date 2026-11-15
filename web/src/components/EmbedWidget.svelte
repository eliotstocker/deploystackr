<script lang="ts">
    import {onMount} from 'svelte';
    import {
        API_BASE,
        fetchAuthMe,
        fetchAuthProviders,
        fetchServices,
        fetchVersions,
        loginDev,
        promote
    } from '../lib/api';
    import {sortEnvironments, formatShortVersion} from '../lib/envPriority';
    import type {AuthProvidersResponse, AuthUser, PromotionResult, ServiceResponse, VersionInfo} from '../types';
    import ServiceIcon from './ServiceIcon.svelte';

    type Props = {
        serviceSlug?: string;
        envParam?: string;
        themeParam?: string;
        titleParam?: string;
        isAuthPopup?: boolean;
    };

    let {
        serviceSlug = '',
        envParam = '',
        themeParam = 'auto',
        titleParam = '',
        isAuthPopup = false
    }: Props = $props();

    let authChecked = $state(false);
    let currentUser = $state<AuthUser | null>(null);
    let providersInfo = $state<AuthProvidersResponse | null>(null);
    let service = $state<ServiceResponse | null>(null);
    let versions = $state<VersionInfo[]>([]);
    let loading = $state(true);
    let loadError = $state('');
    let signingIn = $state(false);
    let selectedEnv = $state('');
    let confirmingDeploy = $state(false);
    let deploying = $state(false);
    let deployError = $state('');
    let deployResult = $state<PromotionResult | null>(null);
    let systemDark = $state(false);

    let isDark = $derived.by(() => {
        if(themeParam === 'dark') {
            return true;
        }
        if(themeParam === 'light') {
            return false;
        }
        return systemDark;
    });

    function cleanTag(v?: string): string {
        if(!v || v === 'unknown') {
            return '';
        }
        let tag = v.trim();
        if(tag.includes(':')) {
            tag = tag.split(':').pop() || tag;
        }
        if(tag.includes('@')) {
            tag = tag.split('@')[0];
        }
        return tag.replace(/^v/, '');
    }

    function areVersionsEqual(v1?: string, v2?: string): boolean {
        if(!v1 || !v2) {
            return false;
        }
        if(v1 === v2) {
            return true;
        }
        const c1 = cleanTag(v1);
        const c2 = cleanTag(v2);
        if(c1 && c1 === c2) {
            return true;
        }
        if(v1.endsWith(':' + v2) || v2.endsWith(':' + v1)) {
            return true;
        }
        return false;
    }

    let availableEnvs = $derived(
        service ? sortEnvironments(service.environments || {}, service.environment_priority).map(([name]) => name) : []
    );

    let canSelectEnv = $derived(!envParam && availableEnvs.length > 1);

    let targetEnvName = $derived.by(() => {
        if(!service) {
            return envParam;
        }
        const envs = service.environments || {};
        if(envParam) {
            const match = Object.keys(envs).find(k => k.toLowerCase() === envParam.toLowerCase());
            if(match) {
                return match;
            }
        }
        if(selectedEnv && envs[selectedEnv]) {
            return selectedEnv;
        }
        if(service.top_env_status?.environment && envs[service.top_env_status.environment]) {
            return service.top_env_status.environment;
        }
        return availableEnvs.length > 0 ? availableEnvs[0] : '';
    });

    function handleEnvChange(e: Event) {
        selectedEnv = (e.currentTarget as HTMLSelectElement).value;
        confirmingDeploy = false;
        deployError = '';
        deployResult = null;
    }

    let targetEnvView = $derived(
        service && targetEnvName ? service.environments?.[targetEnvName] ?? null : null
    );

    let activeVersion = $derived(targetEnvView?.active_version || 'unknown');

    let latestVersion = $derived.by(() => {
        if(versions.length > 0) {
            return versions[0].version;
        }
        return service?.top_env_status?.latest_version || service?.latest_version || '';
    });

    let versionsBehind = $derived.by(() => {
        if(!activeVersion || activeVersion === 'unknown' || !latestVersion) {
            return 0;
        }
        if(areVersionsEqual(activeVersion, latestVersion)) {
            return 0;
        }
        if(versions.length > 0) {
            const idx = versions.findIndex(v => areVersionsEqual(v.version, activeVersion));
            return idx > -1 ? idx : versions.length;
        }
        if(service?.top_env_status && service.top_env_status.environment === targetEnvName) {
            return service.top_env_status.versions_behind;
        }
        return 1;
    });

    let isUpToDate = $derived(
        Boolean(latestVersion) && activeVersion !== 'unknown' && versionsBehind === 0
    );

    let canDeploy = $derived(
        Boolean(currentUser && targetEnvName && service?.user_permissions?.[targetEnvName]?.can_deploy)
    );

    let permReason = $derived(
        targetEnvName && service?.user_permissions?.[targetEnvName]?.reason
            ? service.user_permissions[targetEnvName].reason
            : 'Deployment restricted'
    );

    let serviceUrl = $derived(
        typeof window !== 'undefined' && service
            ? `${window.location.origin}/${encodeURIComponent(service.service)}`
            : '#'
    );

    function syncAuth(): Promise<boolean> {
        return fetchAuthMe()
            .then(me => {
                if(me.authenticated && me.user) {
                    currentUser = me.user;
                    return true;
                }
                currentUser = null;
                return false;
            })
            .catch(() => {
                currentUser = null;
                return false;
            })
            .finally(() => {
                authChecked = true;
            });
    }

    function loadServiceData(): Promise<void> {
        if(!serviceSlug) {
            loading = false;
            loadError = 'Missing service parameter';
            return Promise.resolve();
        }

        return fetchServices()
            .then(list => {
                const found = list.find(s => s.service.toLowerCase() === serviceSlug.toLowerCase());
                if(!found) {
                    service = null;
                    loadError = `Service "${serviceSlug}" not found`;
                    return;
                }
                service = found;
                loadError = '';
                return fetchVersions(found.service)
                    .then(v => {
                        versions = v;
                    })
                    .catch(() => {
                        //keep cached top_env_status fallback if upstream version fetch fails
                    });
            })
            .catch((err: Error) => {
                loadError = err.message || 'Failed to load service';
            })
            .finally(() => {
                loading = false;
            });
    }

    function refreshAll() {
        syncAuth().then(() => loadServiceData());
    }

    function handleSignIn() {
        if(signingIn) {
            return;
        }
        signingIn = true;
        deployError = '';

        const finishDevLogin = () => {
            const persona = providersInfo?.personas?.[0]?.id || 'dev_admin';
            loginDev(persona)
                .then(() => refreshAll())
                .catch((err: Error) => {
                    deployError = err.message || 'Sign in failed';
                })
                .finally(() => {
                    signingIn = false;
                });
        };

        const openOAuthPopup = (provider: string) => {
            const returnTo = '/embed?auth_popup=1';
            const url = `${API_BASE}/auth/login/${encodeURIComponent(provider)}?return_to=${encodeURIComponent(returnTo)}`;
            const popup = window.open(url, 'deploystackr_embed_login', 'width=520,height=680');
            if(!popup) {
                signingIn = false;
                deployError = 'Popup blocked. Allow popups to sign in.';
                return;
            }
            const timer = window.setInterval(() => {
                if(popup.closed) {
                    window.clearInterval(timer);
                    signingIn = false;
                    refreshAll();
                }
            }, 500);
        };

        const ensureProviders = providersInfo
            ? Promise.resolve(providersInfo)
            : fetchAuthProviders().then(p => {
                providersInfo = p;
                return p;
            });

        ensureProviders
            .then(p => {
                const primary = p.connected_provider || p.providers?.[0] || 'dev';
                if(primary === 'dev') {
                    finishDevLogin();
                } else {
                    openOAuthPopup(primary);
                }
            })
            .catch(() => {
                finishDevLogin();
            });
    }

    function handleDeploy() {
        if(!service || !targetEnvName || !latestVersion || deploying || !canDeploy) {
            return;
        }
        deploying = true;
        confirmingDeploy = false;
        deployError = '';
        deployResult = null;

        promote({
            service: service.service,
            environment: targetEnvName,
            target_version: latestVersion,
            user: currentUser?.name || currentUser?.username || 'DeployStackr'
        })
            .then(res => {
                deployResult = res;
                return loadServiceData();
            })
            .catch((err: Error) => {
                deployError = err.message || 'Promotion failed';
            })
            .finally(() => {
                deploying = false;
            });
    }

    onMount(() => {
        document.documentElement.classList.add('embed-mode');

        if(isAuthPopup) {
            try {
                window.opener?.postMessage({type: 'deploystackr:auth_success'}, window.location.origin);
                localStorage.setItem('deploystackr_auth_ping', String(Date.now()));
            } catch {
                //ignore cross-origin storage restrictions
            }
            window.close();
            return;
        }

        const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
        systemDark = mediaQuery.matches;
        const onMediaChange = (e: MediaQueryListEvent) => {
            systemDark = e.matches;
        };
        mediaQuery.addEventListener('change', onMediaChange);

        const onMessage = (e: MessageEvent) => {
            if(e.origin === window.location.origin && e.data?.type === 'deploystackr:auth_success') {
                signingIn = false;
                refreshAll();
            }
        };
        const onStorage = (e: StorageEvent) => {
            if(e.key === 'deploystackr_auth_ping') {
                signingIn = false;
                refreshAll();
            }
        };
        window.addEventListener('message', onMessage);
        window.addEventListener('storage', onStorage);

        fetchAuthProviders()
            .then(p => {
                providersInfo = p;
            })
            .catch(() => {});

        refreshAll();
        const interval = window.setInterval(refreshAll, 30000);

        return () => {
            document.documentElement.classList.remove('embed-mode');
            mediaQuery.removeEventListener('change', onMediaChange);
            window.removeEventListener('message', onMessage);
            window.removeEventListener('storage', onStorage);
            window.clearInterval(interval);
        };
    });
</script>

{#if isAuthPopup}
    <div class="embed-card" class:dark={isDark}>
        <div class="embed-status-msg">Signed in! You can close this window.</div>
    </div>
{:else}
    <div class="embed-card" class:dark={isDark}>
        {#if loading && !service}
            <div class="embed-loading">
                <span class="embed-spinner"></span>
                <span>Loading {serviceSlug || 'service'}...</span>
            </div>
        {:else if loadError && !service}
            <div class="embed-error-state">
                <span class="embed-error-text">{loadError}</span>
                <button class="embed-btn secondary" type="button" onclick={refreshAll}>Retry</button>
            </div>
        {:else if service && targetEnvView}
            <div class="embed-main">
                <div class="embed-identity">
                    <span class="embed-icon-wrap">
                        <ServiceIcon icon={service.icon} name={service.display_name || service.service} size={20}/>
                    </span>
                    <div class="embed-title-group">
                        <div class="embed-name-row">
                            <a
                                class="embed-service-link"
                                href={serviceUrl}
                                target="_blank"
                                rel="noopener noreferrer"
                                title="Open {service.display_name || service.service} in DeployStackr"
                            >
                                <span class="embed-service-name">{titleParam || service.display_name || service.service}</span>
                            </a>
                            {#if canSelectEnv}
                                <label class="embed-env-select-wrap">
                                    <select
                                        class="embed-env-select"
                                        aria-label="Select environment"
                                        value={targetEnvName}
                                        onchange={handleEnvChange}
                                    >
                                        {#each availableEnvs as envOption}
                                            <option value={envOption}>{envOption}</option>
                                        {/each}
                                    </select>
                                    <svg class="embed-env-chevron" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                                        <polyline points="6 9 12 15 18 9"></polyline>
                                    </svg>
                                </label>
                            {:else}
                                <span class="embed-env-badge">{targetEnvName}</span>
                            {/if}
                        </div>
                        <div class="embed-version-row">
                            <span class="embed-version-pill" title={activeVersion}>{formatShortVersion(activeVersion)}</span>
                            {#if isUpToDate}
                                <span class="embed-behind-pill up-to-date">Up to date</span>
                            {:else if versionsBehind > 0}
                                <span class="embed-behind-pill behind">
                                    {versionsBehind} {versionsBehind === 1 ? 'version' : 'versions'} behind
                                </span>
                                {#if latestVersion}
                                    <span class="embed-latest-hint" title="Latest: {latestVersion}">
                                        -> {formatShortVersion(latestVersion)}
                                    </span>
                                {/if}
                            {:else if !latestVersion}
                                <span class="embed-behind-pill neutral">Latest unknown</span>
                            {/if}
                            {#if targetEnvView.pending_pr}
                                {#if targetEnvView.pending_pr.pr_url}
                                    <a
                                        class="embed-pr-pill"
                                        href={targetEnvView.pending_pr.pr_url}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                    >
                                        PR #{targetEnvView.pending_pr.pr_number || 'open'}
                                    </a>
                                {:else}
                                    <span class="embed-pr-pill">PR pending</span>
                                {/if}
                            {/if}
                        </div>
                    </div>
                </div>

                <div class="embed-actions">
                    {#if authChecked && !currentUser}
                        <button
                            class="embed-btn primary"
                            type="button"
                            disabled={signingIn}
                            onclick={handleSignIn}
                        >
                            {#if signingIn}
                                <span class="embed-spinner light"></span>
                                <span>Signing in...</span>
                            {:else}
                                <span>Sign in</span>
                            {/if}
                        </button>
                    {:else if deployResult}
                        {#if deployResult.pr_url}
                            <a
                                class="embed-btn success"
                                href={deployResult.pr_url}
                                target="_blank"
                                rel="noopener noreferrer"
                            >
                                PR #{deployResult.pr_number} opened
                            </a>
                        {:else}
                            <span class="embed-btn success">Promoted</span>
                        {/if}
                    {:else if !canDeploy}
                        <button
                            class="embed-btn muted"
                            type="button"
                            disabled
                            title={permReason}
                        >
                            Restricted
                        </button>
                    {:else if isUpToDate}
                        <button class="embed-btn muted" type="button" disabled>
                            Up to date
                        </button>
                    {:else if confirmingDeploy}
                        <div class="embed-confirm-group">
                            <button
                                class="embed-btn secondary"
                                type="button"
                                onclick={() => (confirmingDeploy = false)}
                            >
                                Cancel
                            </button>
                            <button
                                class="embed-btn confirm"
                                type="button"
                                onclick={handleDeploy}
                                title="Confirm promotion of {formatShortVersion(latestVersion)} to {targetEnvName}"
                            >
                                Confirm {formatShortVersion(latestVersion)}
                            </button>
                        </div>
                    {:else}
                        <button
                            class="embed-btn primary"
                            type="button"
                            disabled={deploying || !latestVersion}
                            onclick={() => (confirmingDeploy = true)}
                            title={latestVersion ? `Deploy ${formatShortVersion(latestVersion)} to ${targetEnvName}` : 'No target version available'}
                        >
                            {#if deploying}
                                <span class="embed-spinner light"></span>
                                <span>Deploying...</span>
                            {:else}
                                <span>Deploy</span>
                            {/if}
                        </button>
                    {/if}
                </div>
            </div>
            {#if deployError}
                <div class="embed-inline-error" role="alert">{deployError}</div>
            {/if}
        {:else}
            <div class="embed-error-state">
                <span class="embed-error-text">No environments configured for {serviceSlug}</span>
            </div>
        {/if}
    </div>
{/if}

<style>
    .embed-card {
        --ew-bg: rgba(255, 255, 255, 0.96);
        --ew-border: rgba(15, 23, 42, 0.12);
        --ew-text: #0f172a;
        --ew-muted: #64748b;
        --ew-pill-bg: rgba(15, 23, 42, 0.06);
        --ew-btn-bg: #0f172a;
        --ew-btn-hover: #1e293b;
        --ew-btn-text: #ffffff;
        --ew-green-bg: #dcfce7;
        --ew-green-text: #15803d;
        --ew-amber-bg: #fef3c7;
        --ew-amber-text: #b45309;
        --ew-blue-bg: #e0f2fe;
        --ew-blue-text: #0369a1;
        --ew-red-bg: #fee2e2;
        --ew-red-text: #b91c1c;

        display: flex;
        flex-direction: column;
        justify-content: center;
        gap: 6px;
        width: 100%;
        min-height: 68px;
        padding: 12px 14px;
        border-radius: 12px;
        background: var(--ew-bg);
        border: 1px solid var(--ew-border);
        color: var(--ew-text);
        font-family: var(--font-sans);
    }

    .embed-card.dark {
        --ew-bg: rgba(15, 23, 42, 0.94);
        --ew-border: rgba(255, 255, 255, 0.14);
        --ew-text: #f8fafc;
        --ew-muted: #94a3b8;
        --ew-pill-bg: rgba(255, 255, 255, 0.1);
        --ew-btn-bg: #f8fafc;
        --ew-btn-hover: #e2e8f0;
        --ew-btn-text: #0f172a;
        --ew-green-bg: rgba(22, 163, 74, 0.22);
        --ew-green-text: #86efac;
        --ew-amber-bg: rgba(217, 119, 6, 0.22);
        --ew-amber-text: #fcd34d;
        --ew-blue-bg: rgba(2, 132, 199, 0.24);
        --ew-blue-text: #7dd3fc;
        --ew-red-bg: rgba(220, 38, 38, 0.22);
        --ew-red-text: #fca5a5;
    }

    .embed-main {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        flex-wrap: wrap;
    }

    .embed-identity {
        display: flex;
        align-items: center;
        gap: 10px;
        min-width: 0;
        flex: 1 1 200px;
    }

    .embed-service-link {
        color: inherit;
        text-decoration: none;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .embed-service-link:hover .embed-service-name {
        text-decoration: underline;
    }

    .embed-icon-wrap {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 34px;
        height: 34px;
        border-radius: 9px;
        background: var(--ew-pill-bg);
        flex-shrink: 0;
    }

    .embed-title-group {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
    }

    .embed-name-row {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
    }

    .embed-service-name {
        font-size: 13.5px;
        font-weight: 700;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .embed-env-badge,
    .embed-env-select {
        font-size: 11px;
        font-weight: 600;
        padding: 1px 7px;
        border-radius: 9999px;
        background: var(--ew-pill-bg);
        color: var(--ew-muted);
        flex-shrink: 0;
    }

    .embed-env-select-wrap {
        position: relative;
        display: inline-flex;
        align-items: center;
        flex-shrink: 0;
    }

    .embed-env-select {
        appearance: none;
        -webkit-appearance: none;
        border: none;
        padding-right: 18px;
        cursor: pointer;
        outline: none;
    }

    .embed-env-select:hover {
        color: var(--ew-text);
    }

    .embed-env-chevron {
        position: absolute;
        right: 6px;
        pointer-events: none;
        color: var(--ew-muted);
    }

    .embed-version-row {
        display: flex;
        align-items: center;
        gap: 6px;
        flex-wrap: wrap;
    }

    .embed-version-pill {
        font-family: var(--font-mono);
        font-size: 11.5px;
        font-weight: 600;
        padding: 1px 6px;
        border-radius: 6px;
        background: var(--ew-pill-bg);
    }

    .embed-behind-pill {
        font-size: 11px;
        font-weight: 600;
        padding: 1px 7px;
        border-radius: 9999px;
    }

    .embed-behind-pill.up-to-date {
        background: var(--ew-green-bg);
        color: var(--ew-green-text);
    }

    .embed-behind-pill.behind {
        background: var(--ew-amber-bg);
        color: var(--ew-amber-text);
    }

    .embed-behind-pill.neutral {
        background: var(--ew-pill-bg);
        color: var(--ew-muted);
    }

    .embed-latest-hint {
        font-family: var(--font-mono);
        font-size: 11px;
        color: var(--ew-muted);
    }

    .embed-pr-pill {
        font-size: 11px;
        font-weight: 600;
        padding: 1px 7px;
        border-radius: 9999px;
        background: var(--ew-blue-bg);
        color: var(--ew-blue-text);
        text-decoration: none;
    }

    .embed-actions {
        display: flex;
        align-items: center;
        flex-shrink: 0;
    }

    .embed-confirm-group {
        display: flex;
        align-items: center;
        gap: 6px;
    }

    .embed-btn.confirm {
        background: #15803d;
        color: #ffffff;
    }

    .embed-btn.confirm:hover {
        background: #166534;
    }

    .embed-btn {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        height: 32px;
        padding: 0 13px;
        border-radius: 8px;
        border: none;
        font-size: 12.5px;
        font-weight: 600;
        cursor: pointer;
        text-decoration: none;
        white-space: nowrap;
        transition: background-color 0.15s ease, opacity 0.15s ease;
    }

    .embed-btn.primary {
        background: var(--ew-btn-bg);
        color: var(--ew-btn-text);
    }

    .embed-btn.primary:hover:not(:disabled) {
        background: var(--ew-btn-hover);
    }

    .embed-btn.primary:disabled {
        opacity: 0.55;
        cursor: not-allowed;
    }

    .embed-btn.secondary {
        background: var(--ew-pill-bg);
        color: var(--ew-text);
    }

    .embed-btn.muted {
        background: var(--ew-pill-bg);
        color: var(--ew-muted);
        cursor: default;
    }

    .embed-btn.success {
        background: var(--ew-green-bg);
        color: var(--ew-green-text);
    }

    .embed-loading,
    .embed-error-state,
    .embed-status-msg {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        font-size: 12.5px;
        color: var(--ew-muted);
    }

    .embed-error-text {
        color: var(--ew-red-text);
    }

    .embed-inline-error {
        font-size: 11.5px;
        color: var(--ew-red-text);
        background: var(--ew-red-bg);
        padding: 4px 8px;
        border-radius: 6px;
    }

    .embed-spinner {
        width: 13px;
        height: 13px;
        border-radius: 50%;
        border: 2px solid currentColor;
        border-top-color: transparent;
        animation: embed-spin 0.7s linear infinite;
        flex-shrink: 0;
    }

    @keyframes embed-spin {
        to {
            transform: rotate(360deg);
        }
    }
</style>
