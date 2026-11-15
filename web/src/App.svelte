<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchServices, triggerSync, fetchAuthMe, logoutUser } from './lib/api';
  import type { ServiceResponse, AuthUser } from './types';
  import ApplicationList from './components/ApplicationList.svelte';
  import ApplicationDetail from './components/ApplicationDetail.svelte';
  import EnvironmentReleaseHealth from './components/EnvironmentReleaseHealth.svelte';
  import SimpleDeployModal from './components/SimpleDeployModal.svelte';
  import AuraBackground from './components/AuraBackground.svelte';
  import LoginView from './components/LoginView.svelte';
  import UserMenu from './components/UserMenu.svelte';
  import ServiceIcon from './components/ServiceIcon.svelte';
  import { sortEnvironments } from './lib/envPriority';

  function getSlugFromPath(): string {
    if (typeof window !== 'undefined') {
      return window.location.pathname.replace(/^\/+|\/+$/g, '');
    }
    return '';
  }

  function formatShortVersion(v?: string): string {
    if (!v || v === 'unknown') return 'n/a';
    if (v.includes(':')) {
      const parts = v.split(':');
      return parts[parts.length - 1];
    }
    return v;
  }

  function cleanTag(v?: string): string {
    if (!v || v === 'unknown') return '';
    let tag = v.trim();
    if (tag.includes(':')) {
      tag = tag.split(':').pop() || tag;
    }
    if (tag.includes('@')) {
      tag = tag.split('@')[0];
    }
    return tag.replace(/^v/, '');
  }

  function areVersionsEqual(v1?: string, v2?: string): boolean {
    if (!v1 || !v2) return false;
    if (v1 === v2) return true;
    const c1 = cleanTag(v1);
    const c2 = cleanTag(v2);
    if (c1 && c1 === c2) return true;
    if (v1.endsWith(':' + v2) || v2.endsWith(':' + v1)) return true;
    return false;
  }

  function getTopEnvStatus(svc: ServiceResponse) {
    if (svc.top_env_status) {
      return svc.top_env_status;
    }
    const envs = sortEnvironments(svc.environments || {}, svc.environment_priority);
    if (envs.length === 0) return null;
    const [envName, envView] = envs[0];
    const active = envView.active_version || 'unknown';
    const latest = svc.latest_version || '';
    const isUp = latest ? areVersionsEqual(active, latest) : false;
    return {
      environment: envName,
      active_version: active,
      latest_version: latest,
      versions_behind: !latest || isUp ? 0 : 1,
      is_up_to_date: isUp,
      pending_pr: envView.pending_pr
    };
  }

  let authChecked = $state(false);
  let currentUser = $state<AuthUser | null>(null);

  let services = $state<ServiceResponse[]>([]);
  let selectedServiceName = $state<string>(getSlugFromPath());
  let searchQuery = $state<string>('');
  let loading = $state<boolean>(true);
  let error = $state<string>('');
  let syncing = $state<boolean>(false);

  function getInitialMobileView(): 'list' | 'overview' {
    if (typeof window !== 'undefined') {
      const stored = localStorage.getItem('deploystackr_mobile_view');
      if (stored === 'list' || stored === 'overview') {
        return stored;
      }
    }
    return 'overview';
  }

  // Mobile layout state: show 'list', 'overview', or 'detail'
  const initialMobileView = getInitialMobileView();
  let isMobile = $state<boolean>(false);
  let mobileView = $state<'list' | 'overview'>(initialMobileView);
  let mobileActivePane = $state<'list' | 'overview' | 'detail'>(getSlugFromPath() ? 'detail' : initialMobileView);
  let mobileReturnPane = $state<'list' | 'overview'>(initialMobileView);

  function handleMobileViewChange(view: 'list' | 'overview') {
    mobileView = view;
    mobileActivePane = view;
    mobileReturnPane = view;
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('deploystackr_mobile_view', view);
    }
    if (selectedServiceName) {
      selectedServiceName = '';
      if (typeof localStorage !== 'undefined') {
        localStorage.removeItem('deploystackr_return_to');
      }
      window.history.pushState(null, '', '/');
    }
  }

  function getInitialSidebarCollapsed(): boolean {
    if(typeof window !== 'undefined') {
      if(window.innerWidth < 1000) {
        const savedNarrow = localStorage.getItem('sidebar_collapsed_narrow');
        if(savedNarrow !== null) {
          return savedNarrow === 'true';
        }
        return true;
      }
      const saved = localStorage.getItem('sidebar_collapsed');
      if(saved !== null) {
        return saved === 'true';
      }
      return false;
    }
    return false;
  }

  //desktop sidebar collapsed state (persisted in localStorage)
  let isCollapsed = $state<boolean>(getInitialSidebarCollapsed());

  function toggleSidebarCollapse() {
    isCollapsed = !isCollapsed;
    if(typeof window !== 'undefined') {
      if(window.innerWidth < 1000) {
        localStorage.setItem('sidebar_collapsed_narrow', String(isCollapsed));
      } else {
        localStorage.setItem('sidebar_collapsed', String(isCollapsed));
      }
    }
  }

  // Deployment popover state
  interface ModalState {
    open: boolean;
    service: ServiceResponse | null;
    env: string;
    version?: string;
  }
  let modalState = $state<ModalState>({
    open: false,
    service: null,
    env: '',
    version: undefined
  });

  let selectedService = $derived(
    services.find((s) => s.service === selectedServiceName) || null
  );

  let previousWidth = typeof window !== 'undefined' ? window.innerWidth : 1200;

  function checkMobile() {
    if(typeof window !== 'undefined') {
      isMobile = window.innerWidth <= 860;
    }
  }

  function handleResize() {
    checkMobile();
    if(typeof window !== 'undefined') {
      const currentWidth = window.innerWidth;
      const crossedBelow = currentWidth < 1000 && previousWidth >= 1000;
      const crossedAbove = currentWidth >= 1000 && previousWidth < 1000;

      if(crossedBelow) {
        const savedNarrow = localStorage.getItem('sidebar_collapsed_narrow');
        isCollapsed = savedNarrow !== null ? savedNarrow === 'true' : true;
      } else if(crossedAbove) {
        const savedWide = localStorage.getItem('sidebar_collapsed');
        isCollapsed = savedWide !== null ? savedWide === 'true' : false;
      }

      previousWidth = currentWidth;
    }
  }

  async function loadData() {
    try {
      if (services.length === 0) {
        loading = true;
      }
      error = '';
      const list = await fetchServices();
      services = list;

      let slug = getSlugFromPath();
      if (!slug && typeof localStorage !== 'undefined') {
        const saved = localStorage.getItem('deploystackr_return_to') || '';
        const savedSlug = saved.replace(/^\/+/, '').split('?')[0];
        if (savedSlug && list.some((s) => s.service === savedSlug)) {
          slug = savedSlug;
          if (typeof window !== 'undefined') {
            window.history.replaceState(null, '', `/${savedSlug}`);
          }
        }
      }

      if (slug) {
        const found = list.find((s) => s.service === slug);
        if (found) {
          selectedServiceName = found.service;
        } else {
          selectedServiceName = '';
        }
      } else {
        selectedServiceName = '';
      }
    } catch (err: any) {
      error = err.message || 'Failed to load services';
    } finally {
      loading = false;
    }
  }

  async function handleSync() {
    try {
      syncing = true;
      await triggerSync();
      await loadData();
    } catch (err) {
      console.error('Webhook sync failed', err);
    } finally {
      syncing = false;
    }
  }

  function handleSelectService(svcName: string) {
    selectedServiceName = svcName;
    if (svcName) {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem('deploystackr_return_to', `/${svcName}`);
      }
      if (isMobile) {
        if (mobileActivePane === 'overview' || mobileActivePane === 'list') {
          mobileReturnPane = mobileActivePane;
        }
        mobileActivePane = 'detail';
      }
      const currentSlug = getSlugFromPath();
      if (currentSlug !== svcName) {
        window.history.pushState(null, '', `/${svcName}`);
      }
    } else {
      if (typeof localStorage !== 'undefined') {
        localStorage.removeItem('deploystackr_return_to');
      }
      if (isMobile) {
        mobileActivePane = mobileReturnPane || 'overview';
      }
      window.history.pushState(null, '', '/');
    }
  }

  function handleBackToList() {
    selectedServiceName = '';
    if (isMobile) {
      mobileActivePane = mobileReturnPane || 'overview';
    }
    if (typeof localStorage !== 'undefined') {
      localStorage.removeItem('deploystackr_return_to');
    }
    window.history.pushState(null, '', '/');
  }

  function handleOpenPromote(svc: ServiceResponse, envName?: string, targetVersion?: string) {
    modalState = {
      open: true,
      service: svc,
      env: envName || '',
      version: targetVersion
    };
  }

  function handleCloseModal() {
    modalState = {
      open: false,
      service: null,
      env: '',
      version: undefined
    };
  }

  function getInitialTheme(): boolean {
    if (typeof window !== 'undefined') {
      const stored = localStorage.getItem('theme');
      if (stored === 'dark') return true;
      if (stored === 'light') return false;
      return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
    return false;
  }

  let isDark = $state<boolean>(getInitialTheme());

  function toggleTheme() {
    isDark = !isDark;
    if (typeof window !== 'undefined') {
      localStorage.setItem('theme', isDark ? 'dark' : 'light');
      document.documentElement.classList.toggle('dark', isDark);
    }
  }

  $effect(() => {
    if (typeof window !== 'undefined') {
      document.documentElement.classList.toggle('dark', isDark);
    }
  });

  $effect(() => {
    if (typeof window !== 'undefined') {
      if (selectedService) {
        const name = selectedService.display_name || selectedService.service;
        document.title = `${name} | DeployStackr`;
      } else {
        document.title = 'DeployStackr | GitOps Release Registry';
      }
    }
  });

  onMount(() => {
    handleResize();
    window.addEventListener('resize', handleResize);

    const handlePopState = () => {
      const slug = getSlugFromPath();
      if (slug) {
        const found = services.find((s) => s.service === slug);
        if (found) {
          selectedServiceName = found.service;
          if (isMobile) mobileActivePane = 'detail';
        } else {
          selectedServiceName = slug;
        }
      } else {
        selectedServiceName = '';
        if (isMobile) {
          mobileActivePane = mobileReturnPane || 'overview';
        }
      }
    };
    window.addEventListener('popstate', handlePopState);

    checkAuth().then((isAuthed) => {
      if (isAuthed) {
        loadData();
      }
    });

    const interval = setInterval(() => {
      if (currentUser) {
        loadData();
      }
    }, 30000);

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handleSystemChange = (e: MediaQueryListEvent) => {
      if (!localStorage.getItem('theme')) {
        isDark = e.matches;
      }
    };
    mediaQuery.addEventListener('change', handleSystemChange);

    return () => {
      window.removeEventListener('resize', handleResize);
      window.removeEventListener('popstate', handlePopState);
      mediaQuery.removeEventListener('change', handleSystemChange);
      clearInterval(interval);
    };
  });

  async function checkAuth(): Promise<boolean> {
    try {
      const me = await fetchAuthMe();
      if (me.authenticated && me.user) {
        currentUser = me.user;
        return true;
      } else {
        currentUser = null;
        return false;
      }
    } catch (err) {
      console.warn('Failed to check auth', err);
      currentUser = null;
      return false;
    } finally {
      authChecked = true;
    }
  }

  async function handleLogout() {
    try {
      await logoutUser();
    } catch (e) {
      console.error(e);
    }
    currentUser = null;
    window.location.href = '/';
  }
</script>

<AuraBackground dark={isDark}>
  {#if !authChecked}
    <div class="auth-loading-screen">
      <div class="spinner"></div>
      <p class="state-msg">Checking authentication...</p>
    </div>
  {:else if !currentUser}
    <LoginView onLoginSuccess={() => checkAuth().then(() => loadData())} />
  {:else}
    <div class="app-viewport">
    {#if loading && services.length === 0}
      <div class="center-state">
        <div class="spinner"></div>
        <p class="state-msg">Hydrating GitOps manifests and deployment state...</p>
      </div>
    {:else if error && services.length === 0}
      <div class="center-state">
        <div class="error-badge">Sync Failed</div>
        <p class="state-msg error-text">{error}</p>
        <button class="primary-btn" onclick={loadData}>Retry</button>
      </div>
    {:else if !isMobile}
      <!-- Desktop: Left pane full height + Right canvas content -->
      <aside class="left-sidebar-pane" class:collapsed={isCollapsed}>
        <ApplicationList
          {services}
          selectedService={selectedServiceName}
          {searchQuery}
          {isCollapsed}
          {isDark}
          user={currentUser}
          onLogout={handleLogout}
          onToggleTheme={toggleTheme}
          onSelectService={handleSelectService}
          onSearchChange={(q) => (searchQuery = q)}
          onToggleCollapse={toggleSidebarCollapse}
        />
      </aside>

      <main class="main-canvas-area" class:sidebar-collapsed={isCollapsed}>
        {#if selectedService}
          <ApplicationDetail
            service={selectedService}
            {isMobile}
            user={currentUser}
            onLogout={handleLogout}
            onBackToList={handleBackToList}
            onPromote={handleOpenPromote}
            onSync={handleSync}
          />
        {:else}
          <EnvironmentReleaseHealth
            {services}
            {currentUser}
            isMobile={false}
            {isDark}
            {syncing}
            onSelectService={handleSelectService}
            onLogout={handleLogout}
            onSync={handleSync}
          />
        {/if}
      </main>
    {:else}
      <!-- Mobile: Single pane at a time -->
      {#if mobileActivePane === 'detail' && selectedService}
        <div class="mobile-pane">
          <ApplicationDetail
            service={selectedService}
            {isMobile}
            user={currentUser}
            onLogout={handleLogout}
            onBackToList={handleBackToList}
            onPromote={handleOpenPromote}
            onSync={handleSync}
          />
        </div>
      {:else if mobileActivePane === 'overview' || (!selectedService && mobileView === 'overview')}
        <div class="mobile-pane">
          <EnvironmentReleaseHealth
            {services}
            {currentUser}
            isMobile={true}
            {isDark}
            {syncing}
            onSelectService={handleSelectService}
            onViewChange={handleMobileViewChange}
            onToggleTheme={toggleTheme}
            onLogout={handleLogout}
            onSync={handleSync}
          />
        </div>
      {:else}
        <div class="mobile-pane">
          <ApplicationList
            {services}
            selectedService={selectedServiceName}
            {searchQuery}
            {isMobile}
            currentView="list"
            user={currentUser}
            {isDark}
            onLogout={handleLogout}
            onToggleTheme={toggleTheme}
            onSelectService={handleSelectService}
            onSearchChange={(q) => (searchQuery = q)}
            onViewChange={handleMobileViewChange}
          />
        </div>
      {/if}
    {/if}

    <!-- Deployment Modal / Popover -->
    {#if modalState.open && modalState.service}
      <SimpleDeployModal
        service={modalState.service}
        initialEnv={modalState.env}
        initialVersion={modalState.version}
        onClose={handleCloseModal}
        onSuccess={loadData}
      />
    {/if}
    </div>
  {/if}
</AuraBackground>

<style>
  .auth-loading-screen {
    width: 100vw;
    height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1rem;
  }

  .app-viewport {
    height: 100vh;
    height: 100dvh;
    width: 100%;
    display: flex;
    padding: 1rem 1.25rem;
    gap: 1.15rem;
    box-sizing: border-box;
    align-items: stretch;
    overflow: hidden;
  }

  .left-sidebar-pane {
    position: fixed;
    top: 1rem;
    left: 1.25rem;
    bottom: 1rem;
    width: 290px;
    height: auto;
    max-height: calc(100vh - 2rem);
    max-height: calc(100dvh - 2rem);
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    transition: width 0.25s cubic-bezier(0.16, 1, 0.3, 1);
    z-index: 50;
  }

  .left-sidebar-pane.collapsed {
    width: 72px;
  }

  .main-canvas-area {
    flex: 1;
    min-width: 0;
    height: 100%;
    max-height: 100%;
    min-height: 0;
    margin-left: calc(290px + 1.15rem);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    transition: margin-left 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  }

  .main-canvas-area.sidebar-collapsed {
    margin-left: calc(72px + 1.15rem);
  }

  .mobile-pane {
    width: 100%;
    height: 100%;
    max-height: 100%;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .mobile-pane :global(.app-list-pane) {
    flex-grow: 1;
  }

  .center-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1rem;
    padding: 3rem;
  }

  .spinner {
    width: 34px;
    height: 34px;
    border: 3px solid rgba(99, 102, 241, 0.2);
    border-top-color: var(--accent-coral);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .state-msg {
    color: var(--text-muted);
    font-size: 0.95rem;
  }

  .error-badge {
    background: #fee2e2;
    color: #dc2626;
    font-size: 0.75rem;
    font-weight: 700;
    padding: 0.25rem 0.65rem;
    border-radius: 9999px;
  }

  .error-text {
    color: #dc2626;
  }

  .primary-btn {
    background: var(--accent-coral);
    color: #ffffff;
    border: none;
    border-radius: var(--radius-pill);
    padding: 0.55rem 1.35rem;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    box-shadow: none;
  }

  @keyframes spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }

  @media (max-width: 860px) {
    .app-viewport {
      padding: 0.75rem;
      gap: 0;
      height: 100vh;
      height: 100dvh;
      max-height: 100dvh;
      overflow: hidden;
    }
  }
</style>
