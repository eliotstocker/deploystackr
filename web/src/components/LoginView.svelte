<script lang="ts">
  import { onMount } from "svelte";
  import { fetchAuthProviders, loginDev, API_BASE } from "../lib/api";
  import type { AuthProvidersResponse } from "../types";

  interface Props {
    authError?: string;
    onLoginSuccess?: () => void;
  }

  let { authError = "", onLoginSuccess }: Props = $props();

  let loadingProviders = $state(true);
  let providersInfo = $state<AuthProvidersResponse>({
    providers: [],
    dev_mode: false,
    personas: [],
  });
  let errorMessage = $state("");
  let devMenuOpen = $state(false);

  let connectedProvider = $derived(
    providersInfo.connected_provider || providersInfo.providers[0] || "dev",
  );

  onMount(async () => {
    if (authError) {
      errorMessage = authError;
    }
    // Check URL search params for auth error
    if (typeof window !== "undefined") {
      const urlParams = new URLSearchParams(window.location.search);
      const err = urlParams.get("auth_error");
      if (err) {
        errorMessage = decodeURIComponent(err);
        const newUrl = window.location.pathname;
        window.history.replaceState(null, "", newUrl);
      }
    }

    try {
      const res = await fetchAuthProviders();
      providersInfo = res;
    } catch (err: any) {
      console.warn("Failed to load auth providers", err);
      // Fallback
      providersInfo = {
        providers: ["dev"],
        connected_provider: "dev",
        dev_mode: true,
        personas: [
          {
            id: "dev_admin",
            name: "Alex Rivera",
            role: "Platform Admin",
            description: "Full access",
          },
          {
            id: "dev_billing",
            name: "Sarah Chen",
            role: "Billing Lead",
            description: "Billing access",
          },
          {
            id: "dev_guest",
            name: "Guest Developer",
            role: "Viewer",
            description: "Read-only viewer",
          },
        ],
      };
    } finally {
      loadingProviders = false;
    }
  });

  async function loginWithProvider(providerName: string, personaId?: string) {
    const currentPath = typeof window !== "undefined" ? (window.location.pathname + window.location.search) : "/";
    const returnTo = currentPath !== "/" ? currentPath : (typeof localStorage !== "undefined" ? localStorage.getItem("deploystackr_return_to") || "" : "");
    if (returnTo && typeof localStorage !== "undefined") {
      localStorage.setItem("deploystackr_return_to", returnTo);
    }

    if (providerName === "dev") {
      const targetPersona =
        personaId || providersInfo.personas?.[0]?.id || "dev_admin";
      try {
        await loginDev(targetPersona);
        if (returnTo && typeof window !== "undefined" && window.location.pathname === "/") {
          window.history.replaceState(null, "", returnTo);
        }
        if (onLoginSuccess) {
          onLoginSuccess();
        } else {
          window.location.reload();
        }
      } catch (e: any) {
        errorMessage = e.message || "Login failed";
      }
      return;
    }
    const returnQuery = returnTo ? `?return_to=${encodeURIComponent(returnTo)}` : "";
    const url = `${API_BASE}/auth/login/${encodeURIComponent(providerName)}${returnQuery}`;
    window.location.href = url;
  }

  function clickOutside(node: HTMLElement, onOut: () => void) {
    function handleClick(e: MouseEvent) {
      if (!node.contains(e.target as Node)) {
        onOut();
      }
    }
    document.addEventListener("click", handleClick, true);
    return {
      destroy() {
        document.removeEventListener("click", handleClick, true);
      },
    };
  }
</script>

<div class="login-container">
  <div class="login-card">
    <div class="logo-wrapper">
      <svg
        class="app-logo"
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 100 100"
        width="68"
        height="68"
        fill="none"
      >
        <defs>
          <linearGradient
            id="loginDolphinGrad"
            x1="85%"
            y1="15%"
            x2="15%"
            y2="85%"
          >
            <stop offset="0%" stop-color="#f8fafc" />
            <stop offset="100%" stop-color="#94a3b8" />
          </linearGradient>
          <linearGradient
            id="loginDolphinGradLight"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stop-color="#ffffff" stop-opacity="0.95" />
            <stop offset="100%" stop-color="#f8fafc" stop-opacity="0.65" />
          </linearGradient>
        </defs>

        <!-- Falcate Dorsal Fin -->
        <path
          d="M 44 28 C 42 16 37 7 30 5 C 27 13 26 23 28 34 Z"
          fill="url(#loginDolphinGrad)"
        />

        <!-- Upper Body / Dark Dorsal Cape -->
        <path
          d="M 16 62 
                 C 19 44 32 28 54 27 
                 C 68 26 77 34 81 40 
                 C 84 42.5 89 45 94 47.5 
                 C 95 48.5 94 50 92 51 
                 C 86 52.5 78 54 70 54.5 
                 C 54 55.5 38 56 16 62 Z"
          fill="url(#loginDolphinGrad)"
        />

        <!-- Lighter Countershaded Underbelly & Pectoral Flipper -->
        <path
          d="M 16 62 
                 C 38 56 54 55.5 70 54.5 
                 C 78 54 86 52.5 92 51 
                 C 90 53.5 84 55.5 78 57 
                 C 71 59.5 63 65 58 75 
                 C 56 79 53 79 54 73 
                 C 55 66 52 64 45 66 
                 C 34 69 24 68 16 62 Z"
          fill="url(#loginDolphinGradLight)"
        />
      </svg>
    </div>

    <h1 class="brand-title">DeployStackr</h1>

    {#if errorMessage}
      <div class="auth-error-banner" role="alert">
        <svg
          width="15"
          height="15"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <span>{errorMessage}</span>
      </div>
    {/if}

    <div class="login-actions">
      {#if connectedProvider === "github"}
        <button
          class="auth-btn github-btn"
          onclick={() => loginWithProvider("github")}
          type="button"
          disabled={loadingProviders}
        >
          <svg
            class="provider-icon"
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              clip-rule="evenodd"
              d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
            />
          </svg>
          <span>Sign in with GitHub</span>
        </button>
      {:else if connectedProvider === "google"}
        <button
          class="auth-btn google-btn"
          onclick={() => loginWithProvider("google")}
          type="button"
          disabled={loadingProviders}
        >
          <svg
            class="provider-icon"
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="currentColor"
          >
            <path
              d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
              fill="#4285F4"
            />
            <path
              d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
              fill="#34A853"
            />
            <path
              d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"
              fill="#FBBC05"
            />
            <path
              d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"
              fill="#EA4335"
            />
          </svg>
          <span>Sign in with Google</span>
        </button>
      {:else}
        <button
          class="auth-btn default-btn"
          onclick={() => loginWithProvider("dev")}
          type="button"
          disabled={loadingProviders}
        >
          <svg
            class="provider-icon"
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4" />
            <polyline points="10 17 15 12 10 7" />
            <line x1="15" y1="12" x2="3" y2="12" />
          </svg>
          <span>Sign In</span>
        </button>
      {/if}
    </div>
  </div>

  {#if providersInfo.dev_mode && providersInfo.personas && providersInfo.personas.length > 0}
    <div class="dev-dock" use:clickOutside={() => (devMenuOpen = false)}>
      {#if devMenuOpen}
        <div class="dev-popover" role="dialog" aria-label="Test Users">
          <div class="dev-popover-header">
            <span class="dev-popover-title">Test Users</span>
          </div>
          <div class="dev-popover-list">
            {#each providersInfo.personas as persona}
              <button
                type="button"
                class="dev-persona-btn"
                onclick={() => loginWithProvider("dev", persona.id)}
              >
                <div class="dev-persona-head">
                  <span class="persona-name">{persona.name}</span>
                  <span class="persona-role">{persona.role}</span>
                </div>
                <p class="persona-desc">{persona.description}</p>
              </button>
            {/each}
          </div>
        </div>
      {/if}

      <button
        type="button"
        class="dev-dock-trigger"
        class:active={devMenuOpen}
        onclick={() => (devMenuOpen = !devMenuOpen)}
        aria-expanded={devMenuOpen}
        aria-haspopup="dialog"
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
          <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
          <circle cx="9" cy="7" r="4" />
          <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
          <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </svg>
        <span>Test Users</span>
        <svg
          class="chevron-icon"
          class:open={devMenuOpen}
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polyline points="18 15 12 9 6 15"></polyline>
        </svg>
      </button>
    </div>
  {/if}
</div>

<style>
  .login-container {
    position: relative;
    width: 100vw;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1.5rem;
    box-sizing: border-box;
  }

  .login-card {
    width: 100%;
    max-width: 360px;
    padding: 2.5rem 2rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
  }

  :global(html.dark) .login-card {
    background: var(--glass-bg, rgba(15, 23, 42, 0.65));
    border-color: rgba(255, 255, 255, 0.08);
    box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.4);
  }

  .logo-wrapper {
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 0.75rem;
  }

  .app-logo {
    display: block;
  }

  .brand-title {
    font-size: 1.6rem;
    font-weight: 700;
    letter-spacing: -0.025em;
    color: var(--text-main);
    margin: 0 0 1.75rem;
  }

  .auth-error-banner {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.65rem 0.85rem;
    border-radius: var(--radius-md, 8px);
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.25);
    color: #ef4444;
    font-size: 0.8rem;
    font-weight: 500;
    margin-bottom: 1.25rem;
    text-align: left;
    box-sizing: border-box;
  }

  .login-actions {
    width: 100%;
  }

  .auth-btn {
    width: 100%;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.65rem;
    border-radius: var(--radius-md, 8px);
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
    border: 1px solid transparent;
  }

  .auth-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .github-btn {
    background: #0f172a;
    color: #f8fafc;
  }

  .github-btn:hover:not(:disabled) {
    background: #1e293b;
  }

  :global(html.dark) .github-btn {
    background: #f8fafc;
    color: #0f172a;
  }

  :global(html.dark) .github-btn:hover:not(:disabled) {
    background: #e2e8f0;
  }

  .google-btn {
    background: rgba(255, 255, 255, 0.9);
    color: #1e293b;
    border-color: rgba(100, 116, 139, 0.25);
  }

  .google-btn:hover:not(:disabled) {
    background: #ffffff;
    border-color: rgba(100, 116, 139, 0.4);
  }

  :global(html.dark) .google-btn {
    background: rgba(30, 41, 59, 0.8);
    color: #f8fafc;
    border-color: rgba(255, 255, 255, 0.12);
  }

  :global(html.dark) .google-btn:hover:not(:disabled) {
    background: rgba(40, 53, 76, 0.9);
  }

  .default-btn {
    background: var(--accent-color, #3b82f6);
    color: #ffffff;
  }

  .default-btn:hover:not(:disabled) {
    filter: brightness(1.08);
  }

  /* Dev Mode Popup Dock */
  .dev-dock {
    position: fixed;
    right: 1.5rem;
    bottom: 1.5rem;
    z-index: 100;
  }

  .dev-dock-trigger {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.45rem 0.75rem;
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-muted, #64748b);
    background: var(--glass-bg, rgba(255, 255, 255, 0.8));
    -webkit-backdrop-filter: blur(12px);
    backdrop-filter: blur(12px);
    border: 1px solid rgba(100, 116, 139, 0.2);
    border-radius: 9999px;
    cursor: pointer;
    transition: all 0.15s ease;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
  }

  .dev-dock-trigger:hover,
  .dev-dock-trigger.active {
    color: var(--text-main, #0f172a);
    border-color: rgba(100, 116, 139, 0.4);
    background: var(--glass-bg, rgba(255, 255, 255, 0.95));
  }

  :global(html.dark) .dev-dock-trigger {
    color: var(--text-muted, #94a3b8);
    background: rgba(30, 41, 59, 0.8);
    border-color: rgba(255, 255, 255, 0.1);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }

  :global(html.dark) .dev-dock-trigger:hover,
  :global(html.dark) .dev-dock-trigger.active {
    color: #f8fafc;
    border-color: rgba(255, 255, 255, 0.2);
    background: rgba(30, 41, 59, 0.95);
  }

  .chevron-icon {
    transition: transform 0.2s ease;
  }

  .chevron-icon.open {
    transform: rotate(180deg);
  }

  .dev-popover {
    position: absolute;
    bottom: calc(100% + 8px);
    right: 0;
    width: 280px;
    background: var(--glass-bg, rgba(255, 255, 255, 0.9));
    -webkit-backdrop-filter: blur(20px);
    backdrop-filter: blur(20px);
    border: 1px solid rgba(100, 116, 139, 0.2);
    border-radius: var(--radius-lg, 12px);
    box-shadow: 0 16px 32px -8px rgba(0, 0, 0, 0.15);
    padding: 0.6rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    box-sizing: border-box;
    animation: popoverFadeIn 0.15s ease-out;
  }

  :global(html.dark) .dev-popover {
    background: rgba(15, 23, 42, 0.92);
    border-color: rgba(255, 255, 255, 0.1);
    box-shadow: 0 16px 32px -8px rgba(0, 0, 0, 0.5);
  }

  @keyframes popoverFadeIn {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .dev-popover-header {
    padding: 0.35rem 0.5rem 0.45rem;
    border-bottom: 1px solid rgba(100, 116, 139, 0.12);
    margin-bottom: 0.2rem;
  }

  .dev-popover-title {
    font-size: 0.725rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted, #64748b);
  }

  .dev-popover-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .dev-persona-btn {
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--radius-md, 8px);
    padding: 0.5rem 0.65rem;
    cursor: pointer;
    text-align: left;
    transition: all 0.12s ease;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .dev-persona-btn:hover {
    background: rgba(15, 23, 42, 0.05);
    border-color: rgba(100, 116, 139, 0.15);
  }

  :global(html.dark) .dev-persona-btn:hover {
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(255, 255, 255, 0.1);
  }

  .dev-persona-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .persona-name {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-main);
  }

  .persona-role {
    font-size: 0.7rem;
    font-weight: 500;
    color: var(--accent-color, #3b82f6);
  }

  .persona-desc {
    font-size: 0.725rem;
    color: var(--text-muted);
    margin: 0;
    line-height: 1.3;
  }
</style>
