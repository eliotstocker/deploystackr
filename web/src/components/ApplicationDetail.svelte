<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { fetchVersions, fetchHistory, fetchDiff, fetchServiceStatus, fetchReleasePRs } from "../lib/api";
  import type {
    ServiceResponse,
    VersionInfo,
    DeploymentEvent,
    ChangelogDiff,
    ClusterStatus,
    EnvironmentView,
  } from "../types";
  import ChangelogModal from "./ChangelogModal.svelte";
  import DiffViewer from "./DiffViewer.svelte";
  import RollbackModal from "./RollbackModal.svelte";
  import ServiceIcon from "./ServiceIcon.svelte";
  import SourceIcon from "./SourceIcon.svelte";
  import UserAvatar from "./UserAvatar.svelte";
  import UserMenu from "./UserMenu.svelte";
  import { sortEnvironments } from "../lib/envPriority";
  import type { AuthUser } from "../types";

  interface Props {
    service: ServiceResponse;
    isMobile: boolean;
    user?: AuthUser | null;
    onLogout?: () => void;
    onBackToList?: () => void;
    onPromote: (
      service: ServiceResponse,
      envName?: string,
      targetVersion?: string,
    ) => void;
    onSync: () => void;
  }

  let {
    service,
    isMobile,
    user,
    onLogout,
    onBackToList,
    onPromote,
    onSync,
  }: Props = $props();

  const PAGE_SIZE = 25;
  type VersionSortMode = 'published' | 'version';

  function getSavedVersionSort(): VersionSortMode {
    if(typeof window === 'undefined') {
      return 'published';
    }
    try {
      return localStorage.getItem('deploystackr_version_sort') === 'version' ? 'version' : 'published';
    } catch(e) {
      return 'published';
    }
  }

  let versions = $state<VersionInfo[]>([]);
  let versionSort = $state<VersionSortMode>(getSavedVersionSort());
  let sortedVersions = $derived.by(() => {
    if(versionSort === 'version') {
      return versions;
    }
    return [...versions].sort((a, b) => {
      const ta = a.released_at ? Date.parse(a.released_at) : NaN;
      const tb = b.released_at ? Date.parse(b.released_at) : NaN;
      const hasA = !Number.isNaN(ta);
      const hasB = !Number.isNaN(tb);
      if(hasA && hasB) {
        return tb - ta;
      }
      if(hasA) {
        return -1;
      }
      if(hasB) {
        return 1;
      }
      return 0;
    });
  });
  let visibleCount = $state<number>(PAGE_SIZE);
  let tableWrapperEl = $state<HTMLDivElement | null>(null);
  let isLoadingMore = $state<boolean>(false);
  let visibleVersions = $derived(sortedVersions.slice(0, visibleCount));
  let hasMore = $derived(visibleCount < sortedVersions.length);

  function setVersionSort(mode: VersionSortMode) {
    if(versionSort === mode) {
      return;
    }
    versionSort = mode;
    visibleCount = PAGE_SIZE;
    if(tableWrapperEl) {
      tableWrapperEl.scrollTop = 0;
    }
    if(typeof window !== 'undefined') {
      try {
        localStorage.setItem('deploystackr_version_sort', mode);
      } catch(e) {
        //fail silently on storage write restrictions
      }
    }
  }

  function loadMore() {
    if(!hasMore || isLoadingMore) {
      return;
    }
    isLoadingMore = true;
    visibleCount = Math.min(visibleCount + PAGE_SIZE, versions.length);
    setTimeout(() => {
      isLoadingMore = false;
    }, 150);
  }

  function handleTableScroll(e: Event) {
    const el = e.currentTarget as HTMLElement;
    if(!el || !hasMore) {
      return;
    }
    //load more when within 120px of bottom
    if(el.scrollTop + el.clientHeight >= el.scrollHeight - 120) {
      loadMore();
    }
  }

  function sentinelAction(node: HTMLElement) {
    const observer = new IntersectionObserver(entries => {
      if(entries[0]?.isIntersecting) {
        loadMore();
      }
    }, {root: tableWrapperEl, rootMargin: '100px'});

    observer.observe(node);
    return {
      destroy() {
        observer.disconnect();
      }
    };
  }

  let historyMap = $state<Record<string, DeploymentEvent[]>>({});
  let loadingArtifacts = $state(true);
  let releasePRs = $state<Record<string, number[]>>({});

  let selectedChangelog = $state<VersionInfo | null>(null);
  let expandedDiffVersion = $state<string | null>(null);
  let diffData = $state<ChangelogDiff | null>(null);
  let loadingDiff = $state(false);

  function getSavedShowNotes(): boolean {
    if(typeof window === 'undefined') {
      return false;
    }
    try {
      return localStorage.getItem('deploystackr_show_notes') === 'true';
    } catch(e) {
      return false;
    }
  }

  let showExpandedNotes = $state(getSavedShowNotes());

  function toggleExpandedNotes() {
    showExpandedNotes = !showExpandedNotes;
    if(typeof window !== 'undefined') {
      try {
        localStorage.setItem('deploystackr_show_notes', String(showExpandedNotes));
      } catch(e) {
        //fail silently on storage write restrictions
      }
    }
  }

  type EnvStatusState = {
    key: 'ready' | 'deploying' | 'pending' | 'failed';
    label: string;
    tooltip: string;
  };

  let liveClusterStatuses = $state<Record<string, ClusterStatus>>({});
  let pollTimer: ReturnType<typeof setInterval> | null = null;

  function getClusterStatus(envName: string, envView: EnvironmentView): ClusterStatus | undefined {
    return liveClusterStatuses[envName] || envView.cluster_status;
  }

  function getEnvStatus(envName: string, envView: EnvironmentView): EnvStatusState | null {
    if(envView.pending_pr) {
      return {
        key: 'pending',
        label: 'Pending',
        tooltip: `PR #${envView.pending_pr.pr_number || 'open'} pending for ${envView.pending_pr.target_version}`,
      };
    }

    const cs = getClusterStatus(envName, envView);
    if(!cs) return null;

    if(cs.status === 'ready') {
      return {
        key: 'ready',
        label: 'Ready',
        tooltip: cs.message || 'All replicas ready',
      };
    }
    if(cs.status === 'progressing') {
      return {
        key: 'deploying',
        label: 'Deploying',
        tooltip: cs.message || 'Rollout in progress',
      };
    }
    if(cs.status === 'pending') {
      return {
        key: 'pending',
        label: 'Pending',
        tooltip: cs.message || 'Waiting for GitOps rollout to start',
      };
    }
    if(cs.status === 'degraded' || cs.status === 'error' || cs.status === 'not_found') {
      return {
        key: 'failed',
        label: 'Failed',
        tooltip: cs.message || 'Deployment degraded or failed',
      };
    }

    return null;
  }

  function checkAndPollStatuses() {
    let isBusy = false;
    for(const [name, env] of Object.entries(service.environments || {})) {
      const st = getEnvStatus(name, env);
      if(st?.key === 'deploying' || st?.key === 'pending') {
        isBusy = true;
        break;
      }
    }

    if(isBusy && !pollTimer) {
      pollTimer = setInterval(async () => {
        try {
          const res = await fetchServiceStatus(service.service);
          liveClusterStatuses = res;
          //stop polling once rollout completes or fails
          const stillBusy = Object.values(res).some(s => s.status === 'progressing' || s.status === 'pending');
          if(!stillBusy && pollTimer) {
            clearInterval(pollTimer);
            pollTimer = null;
          }
        } catch(e) {
          //fail silently on background poll error
        }
      }, 2500);
    } else if(!isBusy && pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  $effect(() => {
    const svcName = service.service;
    const envSnapshot = Object.entries(service.environments || {})
      .map(([k, v]) => `${k}:${v.active_version}:${v.cluster_status?.status || ''}:${v.pending_pr?.pr_number || ''}`)
      .join('|');
    if(svcName && envSnapshot !== undefined) {
      fetchServiceStatus(svcName)
        .then(res => {
          liveClusterStatuses = res;
          checkAndPollStatuses();
        })
        .catch(() => {});
    }
  });

  onDestroy(() => {
    if(pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  });

  let canDeployAny = $derived(
    Object.keys(service.user_permissions || {}).length === 0 ||
      Object.values(service.user_permissions || {}).some((p) => p.can_deploy),
  );

  let copiedField = $state<string | null>(null);
  let copyTimeout: ReturnType<typeof setTimeout> | undefined;

  function copyToClipboard(field: string, text: string) {
    if (!text) return;
    navigator.clipboard?.writeText(text);
    copiedField = field;
    if (copyTimeout) clearTimeout(copyTimeout);
    copyTimeout = setTimeout(() => {
      copiedField = null;
    }, 1500);
  }

  function formatShortPath(str: string): string {
    if (!str) return "";
    if (str.includes("://") || str.includes("git@")) {
      const parts = str.split("/");
      return parts.slice(-2).join("/");
    }
    const parts = str.split("/").filter(Boolean);
    if (parts.length <= 2) return str;
    return "…/" + parts.slice(-2).join("/");
  }

  function formatShortRepo(str: string): string {
    if (!str) return "";
    const parts = str.split("/");
    if (parts.length <= 2) return str;
    return parts.slice(-2).join("/");
  }

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

  function getSourceUrl(src?: { type?: string; repository?: string }): string {
    if(!src?.repository) return '';
    const t = (src.type || '').toLowerCase();
    if(t === 'github_releases' || t === 'github' || src.repository.includes('github.com')) {
      let r = src.repository.trim();
      if(r.startsWith('http://') || r.startsWith('https://')) return r.replace(/\.git$/, '');
      if(r.startsWith('git@github.com:')) return `https://github.com/${r.replace('git@github.com:', '').replace(/\.git$/, '')}`;
      if(!r.includes('://') && r.split('/').filter(Boolean).length === 2) {
        return `https://github.com/${r.replace(/\.git$/, '')}`;
      }
      return r;
    }
    return '';
  }

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

  function getGitopsPrUrl(gitopsRepo: string | undefined, prNumber: number | string): string {
    if(!prNumber) return '';
    const repo = gitopsRepo || 'https://github.com/eliotstocker/pi-cluster';
    return getPrUrl(repo, prNumber);
  }

  function extractPRsFromText(text?: string): number[] {
    if(!text) return [];
    const prs = new Set<number>();
    
    const urlMatches = text.matchAll(/(?:\/pull\/|\/merge_requests\/)(\d+)/gi);
    for(const m of urlMatches) {
      const num = parseInt(m[1], 10);
      if(!isNaN(num) && num > 0 && num < 1000000) prs.add(num);
    }
    
    const parenMatches = text.matchAll(/\(#(\d+)\)/g);
    for(const m of parenMatches) {
      const num = parseInt(m[1], 10);
      if(!isNaN(num) && num > 0 && num < 1000000) prs.add(num);
    }

    const prMatches = text.matchAll(/(?:pull request|pr|gh)[-:\s]+#?(\d+)/gi);
    for(const m of prMatches) {
      const num = parseInt(m[1], 10);
      if(!isNaN(num) && num > 0 && num < 1000000) prs.add(num);
    }

    return Array.from(prs).sort((a, b) => b - a);
  }

  function getChangelogSnippet(changelog?: string): string {
    if(!changelog) {
      return '';
    }
    const lines = changelog.split('\n');
    for(const rawLine of lines) {
      const line = rawLine.trim();
      if(!line) {
        continue;
      }
      const clean = line.replace(/^[#\s\-*]+/, '').trim();
      const lower = clean.toLowerCase();
      if(
        lower.startsWith('bug fix') ||
        lower.startsWith('feature') ||
        lower.startsWith('highlight') ||
        lower.startsWith('mainten') ||
        lower.startsWith('initial')
      ) {
        continue;
      }
      if(clean.length > 5) {
        return clean;
      }
    }
    const first = lines.find(l => l.trim().length > 0)?.trim() || '';
    return first.replace(/^[#\s\-*]+/, '').trim();
  }

  function getChangelogExpandedLines(changelog?: string, maxLines = 3): string[] {
    if(!changelog) {
      return [];
    }
    const rawLines = changelog.split('\n');
    const lines: string[] = [];
    for(const raw of rawLines) {
      const line = raw.trim();
      if(!line) {
        continue;
      }
      const clean = line.replace(/^[#\s\-*]+/, '').trim();
      const lower = clean.toLowerCase();
      if(
        lower.startsWith('bug fix') ||
        lower.startsWith('feature') ||
        lower.startsWith('highlight') ||
        lower.startsWith('mainten') ||
        lower.startsWith('breaking change') ||
        lower.startsWith('initial release') ||
        lower.startsWith('improvements') ||
        lower.startsWith('fixes')
      ) {
        continue;
      }
      if(clean.length > 3) {
        lines.push(clean);
        if(lines.length >= maxLines) {
          break;
        }
      }
    }
    if(lines.length < 1) {
      for(const raw of rawLines) {
        const clean = raw.replace(/^[#\s\-*]+/, '').trim();
        if(clean.length > 0) {
          lines.push(clean);
          if(lines.length >= maxLines) {
            break;
          }
        }
      }
    }
    return lines;
  }

  let lastLoadedSvc = $state("");
  $effect(() => {
    if(service.service && service.service !== lastLoadedSvc) {
      lastLoadedSvc = service.service;
      closeDiff();
      visibleCount = PAGE_SIZE;
      if(tableWrapperEl) {
        tableWrapperEl.scrollTop = 0;
      }
      loadDetails(service.service);
    }
  });

  async function loadDetails(svcName: string) {
    loadingArtifacts = true;
    expandedDiffVersion = null;
    diffData = null;
    releasePRs = {};
    try {
      const [vers, hist] = await Promise.all([
        fetchVersions(svcName),
        fetchHistory(svcName),
      ]);
      versions = Array.isArray(vers) ? vers : [];
      historyMap = hist || {};
      visibleCount = PAGE_SIZE;
      if(tableWrapperEl) {
        tableWrapperEl.scrollTop = 0;
      }
    } catch (err) {
      console.error("Failed to load application details", err);
    } finally {
      loadingArtifacts = false;
    }

    // Fast synchronous extraction from changelogs already present
    const localPRs: Record<string, number[]> = {};
    for(const v of versions) {
      if(v.changelog) {
        const found = extractPRsFromText(v.changelog);
        if(found.length > 0) localPRs[v.version] = found;
      }
    }
    releasePRs = localPRs;

    // Background lazy fetch to enrich from backend
    fetchReleasePRs(svcName)
      .then(res => {
        if(lastLoadedSvc === svcName && res && typeof res === 'object') {
          releasePRs = { ...releasePRs, ...res };
        }
      })
      .catch(() => {});
  }

  let selectedDiffEnv = $state<string>("production");

  let sortedEnvs = $derived(
    sortEnvironments(service.environments || {}, service.environment_priority)
  );

  let diffEnvironments = $derived(
    sortedEnvs.map(([name, env]) => ({
      name,
      activeVersion: formatShortVersion(env.active_version),
    })),
  );

  async function handleDiffEnvChange(envName: string) {
    selectedDiffEnv = envName;
    if(!expandedDiffVersion) {
      return;
    }
    diffData = null;
    loadingDiff = true;
    try {
      const rawActive = service.environments?.[envName]?.active_version || "";
      const activeVer = formatShortVersion(rawActive);
      const targetVer = formatShortVersion(expandedDiffVersion);
      diffData = await fetchDiff(service.service, activeVer, targetVer);
    } catch(err) {
      console.warn("Failed to load diff for env " + envName, err);
      diffData = null;
    } finally {
      loadingDiff = false;
    }
  }

  function closeDiff() {
    expandedDiffVersion = null;
    diffData = null;
  }

  async function toggleDiff(targetVersion: string) {
    if (expandedDiffVersion === targetVersion) {
      closeDiff();
      return;
    }

    // Immediately clear previous diff so it hides instantly when switching versions
    diffData = null;
    expandedDiffVersion = targetVersion;
    if (!selectedDiffEnv || !service.environments?.[selectedDiffEnv]) {
      selectedDiffEnv = sortedEnvs[0]?.[0] || "production";
    }

    loadingDiff = true;
    try {
      const rawActive =
        service.environments?.[selectedDiffEnv]?.active_version || "";
      const activeVer = formatShortVersion(rawActive);
      const targetVer = formatShortVersion(targetVersion);
      diffData = await fetchDiff(service.service, activeVer, targetVer);
    } catch (e) {
      console.warn("Failed to load diff", e);
      diffData = null;
    } finally {
      loadingDiff = false;
    }
  }

  function getEnvBadgeClass(envName: string): string {
    const lower = envName.toLowerCase();
    if (lower.includes("pre")) return "badge-preprod";
    if (lower.includes("prod")) return "badge-prod";
    if (lower.includes("stag")) return "badge-staging";
    if (lower.includes("dev")) return "badge-dev";
    return "badge-default";
  }

  function formatRelative(isoStr?: string): string {
    if (!isoStr) return "Never deployed";
    const d = new Date(isoStr);
    const now = new Date();
    const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);
    if (diffSec < 60) return "just now";
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
    return `${Math.floor(diffSec / 86400)}d ago`;
  }

  function formatDate(isoStr?: string): string {
    if (!isoStr) return "";
    try {
      const d = new Date(isoStr);
      return d.toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return "";
    }
  }

  function formatShortVersion(v: string): string {
    if (!v || v === "unknown") return "n/a";
    if (v.includes(":")) {
      const parts = v.split(":");
      return parts[parts.length - 1];
    }
    return v;
  }

  function getEventsForVersion(vName: string): DeploymentEvent[] {
    const list: DeploymentEvent[] = [];
    for (const events of Object.values(historyMap || {})) {
      for (const e of events) {
        if (e.version === vName || formatShortVersion(e.version) === vName) {
          list.push(e);
        }
      }
    }
    return list;
  }

  let deployDropdownOpen = $state(false);
  let hoveredEnv = $state<string | null>(null);
  let openKebabVersion = $state<string | null>(null);
  let openDeployRowVersion = $state<string | null>(null);
  let openEnvKebab = $state<string | null>(null);
  let activeRollbackEnv = $state<string | null>(null);

  interface DeploymentDotInfo {
    environment: string;
    isActive: boolean;
    isPending?: boolean;
    isDeploying?: boolean;
    isFailed?: boolean;
    statusMessage?: string;
    runningVersion?: string;
    authorName: string;
    authorEmail?: string;
    deployedAt?: string;
    activeDuration?: string;
    retiredAt?: string;
    prNumber?: number;
    commitSha?: string;
    prUrl?: string;
  }

  function matchVer(v1?: string | null, v2?: string | null): boolean {
    if (!v1 || !v2) return false;
    if (v1 === v2) return true;
    const s1 = formatShortVersion(v1);
    const s2 = formatShortVersion(v2);
    if (s1 === s2) return true;
    if (v1.endsWith(":" + v2) || v2.endsWith(":" + v1)) return true;
    return v1.replace(/^v/, "") === v2.replace(/^v/, "");
  }

  function getPreviousVersionFromHistory(
    envName: string,
    targetVersion?: string | null,
  ): string | null {
    const events =
      historyMap[envName] ||
      service.environments?.[envName]?.recent_events ||
      [];
    for (const e of events) {
      if (e.version && !matchVer(e.version, targetVersion)) {
        return e.version;
      }
    }
    return null;
  }

  function getEnvDeploymentInfo(envName: string, envView: EnvironmentView) {
    const cs = getClusterStatus(envName, envView);
    const pendingPR = envView.pending_pr;

    let state: "ready" | "pending" | "deploying" | "failed" = "ready";
    let targetVersion: string | null = null;
    let runningVersion: string | null = envView.active_version || null;
    let message: string | undefined = undefined;

    if (pendingPR) {
      state = "pending";
      targetVersion = pendingPR.target_version;
      runningVersion = envView.active_version || null;
      message = `PR #${pendingPR.pr_number || "open"} pending`;
    } else if (cs) {
      if (cs.status === "progressing") {
        state = "deploying";
        targetVersion = envView.active_version;
        runningVersion =
          cs.current_version ||
          getPreviousVersionFromHistory(envName, targetVersion) ||
          envView.active_version;
        message = cs.message || "Rollout in progress";
      } else if (cs.status === "pending") {
        state = "pending";
        targetVersion = envView.active_version;
        runningVersion =
          cs.current_version ||
          getPreviousVersionFromHistory(envName, targetVersion) ||
          envView.active_version;
        message = cs.message || "Waiting for GitOps rollout to start";
      } else if (cs.status === "degraded" || cs.status === "error") {
        state = "failed";
        targetVersion = envView.active_version;
        runningVersion =
          cs.current_version ||
          getPreviousVersionFromHistory(envName, targetVersion) ||
          envView.active_version;
        message = cs.message || "Deployment degraded or failed";
      } else if (cs.status === "ready") {
        state = "ready";
        runningVersion = envView.active_version;
        targetVersion = null;
      }
    }

    return { state, targetVersion, runningVersion, message, cs, pendingPR };
  }

  function getDeploymentDots(version: string): DeploymentDotInfo[] {
    const dots: DeploymentDotInfo[] = [];
    const evts = getEventsForVersion(version);
    const accountedEnvs = new Set<string>();

    // 1. Process defined environments for target state (pending/deploying/failed) or active running state
    for (const [envName, envView] of Object.entries(
      service.environments || {},
    )) {
      const info = getEnvDeploymentInfo(envName, envView);
      const isTarget = info.targetVersion && matchVer(info.targetVersion, version);
      const isRunning = info.runningVersion && matchVer(info.runningVersion, version);

      // Flashing dot for in-flight deployment or clear failed state dot on the target version
      if (isTarget && info.state !== "ready") {
        const isSameAsRunning = matchVer(info.targetVersion, info.runningVersion);
        if (info.state === "failed") {
          dots.push({
            environment: envName,
            isActive: false,
            isFailed: true,
            statusMessage: info.message || "Deployment degraded or failed",
            runningVersion: isSameAsRunning ? undefined : (info.runningVersion || undefined),
            authorName: envView.deployed_by || "DeployStackr",
            deployedAt: envView.last_deployed,
            prNumber: envView.pr_number,
            commitSha: envView.commit_sha,
          });
          accountedEnvs.add(envName);
        } else if (info.state === "deploying") {
          dots.push({
            environment: envName,
            isActive: false,
            isDeploying: true,
            statusMessage: info.message || "Rollout in progress",
            runningVersion: isSameAsRunning ? undefined : (info.runningVersion || undefined),
            authorName: envView.deployed_by || "DeployStackr",
            deployedAt: envView.last_deployed,
            prNumber: envView.pr_number,
            commitSha: envView.commit_sha,
          });
          accountedEnvs.add(envName);
        } else if (info.state === "pending") {
          dots.push({
            environment: envName,
            isActive: false,
            isPending: true,
            statusMessage: info.message || "Waiting for GitOps rollout to start",
            runningVersion: isSameAsRunning ? undefined : (info.runningVersion || undefined),
            authorName: info.pendingPR?.user || envView.deployed_by || "DeployStackr",
            deployedAt: info.pendingPR?.created_at || envView.last_deployed,
            prNumber: info.pendingPR?.pr_number || envView.pr_number,
            prUrl: info.pendingPR?.pr_url,
            commitSha: envView.commit_sha,
          });
          accountedEnvs.add(envName);
        }
      }

      // Currently running version dot (stays solid env color)
      if (isRunning && !accountedEnvs.has(envName)) {
        const activeEvt = evts.find(
          (e) => e.environment === envName && (e.is_active || matchVer(e.version, version)),
        );
        dots.push({
          environment: envName,
          isActive: true,
          runningVersion: info.targetVersion ? (info.runningVersion || undefined) : undefined,
          statusMessage: info.targetVersion ? `Target ${formatShortVersion(info.targetVersion)} ${info.state}` : undefined,
          authorName: activeEvt?.author_name || envView.deployed_by || "System",
          authorEmail: activeEvt?.author_email,
          deployedAt: activeEvt?.deployed_at || envView.last_deployed,
          activeDuration: activeEvt?.active_duration,
          prNumber: activeEvt?.pr_number || envView.pr_number,
          commitSha: activeEvt?.commit_sha || envView.commit_sha,
        });
        accountedEnvs.add(envName);
      }
    }

    // 2. Also check if any active events in evts were not in service.environments
    for (const e of evts) {
      if (e.is_active && !accountedEnvs.has(e.environment)) {
        dots.push({
          environment: e.environment,
          isActive: true,
          authorName: e.author_name || "System",
          authorEmail: e.author_email,
          deployedAt: e.deployed_at,
          activeDuration: e.active_duration,
          prNumber: e.pr_number,
          commitSha: e.commit_sha,
        });
        accountedEnvs.add(e.environment);
      }
    }

    // 3. Collect previous deployments (historical events not currently active or target on this version)
    const previousEnvs = new Set<string>();
    for (const e of evts) {
      if (
        !accountedEnvs.has(e.environment) &&
        !previousEnvs.has(e.environment)
      ) {
        dots.push({
          environment: e.environment,
          isActive: false,
          isPending: false,
          isDeploying: false,
          isFailed: false,
          authorName: e.author_name || "System",
          authorEmail: e.author_email,
          deployedAt: e.deployed_at,
          activeDuration: e.active_duration,
          retiredAt: e.retired_at,
          prNumber: e.pr_number,
          commitSha: e.commit_sha,
        });
        previousEnvs.add(e.environment);
      }
    }

    return dots;
  }

  function isEnvActiveOnVersion(
    envName: string | null,
    version: string,
  ): boolean {
    if (!envName) return false;
    const env = service.environments?.[envName];
    if (!env) return false;
    const info = getEnvDeploymentInfo(envName, env);
    if (info.runningVersion && matchVer(info.runningVersion, version)) {
      return true;
    }
    const cleanActive = formatShortVersion(env.active_version || "");
    const cleanVersion = formatShortVersion(version);
    if (
      cleanActive === cleanVersion ||
      env.active_version === version ||
      cleanActive === version ||
      (env.active_version && env.active_version.endsWith(":" + version)) ||
      (env.active_version && version.endsWith(":" + env.active_version))
    ) {
      return true;
    }
    const evts = getEventsForVersion(version);
    return evts.some((e) => e.environment === envName && e.is_active);
  }

  function handleWindowClick(e: MouseEvent) {
    const target = e.target as HTMLElement | null;
    if (deployDropdownOpen && !target?.closest(".deploy-dropdown-wrapper")) {
      deployDropdownOpen = false;
    }
    if (openKebabVersion && !target?.closest(".kebab-wrapper")) {
      openKebabVersion = null;
    }
    if (openDeployRowVersion && !target?.closest(".row-deploy-wrapper")) {
      openDeployRowVersion = null;
    }
    if (openEnvKebab && !target?.closest(".env-kebab-wrapper")) {
      openEnvKebab = null;
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<div class="canvas-flow">
  <!-- Top Title & Navigation: Sits directly on the canvas with NO background -->
  <header class="canvas-header">
    <div class="header-main-col">
      {#if isMobile && onBackToList}
        <button class="back-btn" onclick={onBackToList} type="button">
          ← Back to Apps
        </button>
      {/if}

      <div class="title-row">
        <span class="title-icon">
          <ServiceIcon icon={service.icon} name={service.service} size={26} />
        </span>
        <h1 class="page-title">{service.display_name || service.service}</h1>
      </div>
    </div>

    <div class="header-action-buttons">
      <div class="sync-git-tooltip-wrap">
        <button
          class="sync-icon-btn"
          onclick={onSync}
          type="button"
          aria-label="Sync Git"
        >
          <svg
            width="17"
            height="17"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67" />
          </svg>
        </button>
        <span class="custom-tooltip" role="tooltip">Sync Git</span>
      </div>

      <div class="deploy-dropdown-wrapper">
        <div class="deploy-tooltip-wrap">
          <button
            class="deploy-icon-btn"
            class:active={deployDropdownOpen}
            onclick={() => (deployDropdownOpen = !deployDropdownOpen)}
            type="button"
            aria-label="Deploy to..."
            aria-expanded={deployDropdownOpen}
          >
            <svg
              width="17"
              height="17"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path
                d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"
              />
              <path
                d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"
              />
              <path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0" />
              <path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5" />
            </svg>
          </button>
          {#if !deployDropdownOpen}
            <span class="custom-tooltip" role="tooltip">Deploy to...</span>
          {/if}
        </div>

        {#if deployDropdownOpen}
          <div class="deploy-menu-popover">
            <div class="deploy-menu-header">Select Target Environment</div>
            {#each sortedEnvs as [envName]}
              {@const perm = service.user_permissions?.[envName]}
              {@const allowed = perm ? perm.can_deploy : true}
              <button
                class="deploy-menu-item"
                class:disabled={!allowed}
                disabled={!allowed}
                title={!allowed ? perm?.reason : undefined}
                onclick={() => {
                  if (!allowed) return;
                  deployDropdownOpen = false;
                  onPromote(service, envName);
                }}
                type="button"
              >
                <div class="menu-item-left">
                  <span class="menu-item-dot {getEnvBadgeClass(envName)}"
                  ></span>
                  <div class="menu-item-meta">
                    <span class="menu-item-name">{envName}</span>
                    {#if !allowed && perm?.reason}
                      <span class="menu-item-reason">{perm.reason}</span>
                    {/if}
                  </div>
                </div>
                {#if !allowed}
                  <svg
                    class="lock-icon"
                    width="12"
                    height="12"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                  >
                    <rect x="3" y="11" width="18" height="11" rx="2" ry="2"
                    ></rect>
                    <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
                  </svg>
                {:else}
                  <span class="menu-item-arrow">→</span>
                {/if}
              </button>
            {/each}
            {#if Object.keys(service.environments || {}).length === 0}
              <button
                class="deploy-menu-item"
                onclick={() => {
                  deployDropdownOpen = false;
                  onPromote(service);
                }}
                type="button"
              >
                <span class="menu-item-name">Default Environment</span>
                <span class="menu-item-arrow">→</span>
              </button>
            {/if}
          </div>
        {/if}
      </div>

      {#if user}
        <UserMenu {user} {service} {onLogout} />
      {/if}
    </div>
  </header>

  <!-- Active Environments: Compact floating panes -->
  <section class="env-section">
    <div class="section-title-row">
      <h2 class="section-heading">Active Environments</h2>
      <span class="count-pill"
        >{Object.keys(service.environments || {}).length} configured</span
      >
    </div>

    <div class="env-panes-grid">
      {#each sortedEnvs as [envName, envView]}
        <div
          class="floating-pane env-card-pane {getEnvBadgeClass(envName)}"
          class:is-hovered={hoveredEnv === envName}
          class:has-open-menu={openEnvKebab === envName}
          role="group"
          aria-label="{envName} environment"
          onmouseenter={() => (hoveredEnv = envName)}
          onmouseleave={() => (hoveredEnv = null)}
        >
          <div class="env-card-header">
            <div class="env-badge-title">
              <span class="status-dot {getEnvBadgeClass(envName)}"></span>
              <span class="env-title">{envName}</span>
            </div>
            <div class="env-header-actions">
              {#if getEnvStatus(envName, envView)}
                {@const st = getEnvStatus(envName, envView)}
                <span class="env-status-pill {st.key}" title={st.tooltip}>
                  <span class="status-pill-dot"></span>
                  <span class="status-pill-text">{st.label}</span>
                </span>
              {/if}

              <span
                class="strategy-badge"
                class:pr={envView.write_strategy === "pull_request"}
              >
                {envView.write_strategy === "pull_request" ? "PR" : "Direct"}
              </span>

              <div class="kebab-wrapper env-kebab-wrapper">
                <button
                  class="kebab-btn env-kebab-btn"
                  class:active={openEnvKebab === envName}
                  onclick={(e) => {
                    e.stopPropagation();
                    openEnvKebab = openEnvKebab === envName ? null : envName;
                  }}
                  type="button"
                  aria-label="Options for {envName}"
                  title="Environment options"
                >
                  <svg
                    width="15"
                    height="15"
                    viewBox="0 0 24 24"
                    fill="currentColor"
                  >
                    <circle cx="12" cy="12" r="2"></circle>
                    <circle cx="12" cy="5" r="2"></circle>
                    <circle cx="12" cy="19" r="2"></circle>
                  </svg>
                </button>

                {#if openEnvKebab === envName}
                  <div class="kebab-dropdown env-kebab-dropdown">
                    <button
                      class="kebab-item"
                      type="button"
                      onclick={(e) => {
                        e.stopPropagation();
                        openEnvKebab = null;
                        activeRollbackEnv = envName;
                      }}
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
                        <polyline points="1 4 1 10 7 10"></polyline>
                        <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path>
                      </svg>
                      <span>Rollback</span>
                    </button>
                  </div>
                {/if}
              </div>
            </div>
          </div>

          <div class="active-version-display">
            <span class="version-value" title={envView.active_version}>
              {formatShortVersion(envView.active_version)}
            </span>
            {#if getClusterStatus(envName, envView)}
              {@const cs = getClusterStatus(envName, envView)}
              <span
                class="cluster-replicas-count {cs.status}"
                title={cs.message || cs.status}
              >
                {cs.ready_replicas}/{cs.desired_replicas} pods
              </span>
            {/if}
            {#if envView.pr_number}
              <span class="pr-pill">#{envView.pr_number}</span>
            {/if}
            {#if envView.pending_pr}
              {#if envView.pending_pr.pr_url}
                <a
                  href={envView.pending_pr.pr_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="pending-pill is-link"
                  title="Open PR #{envView.pending_pr.pr_number || ''} on GitHub"
                  onclick={e => e.stopPropagation()}
                >
                  <span class="pending-pill-ring"></span>
                  <span>PR #{envView.pending_pr.pr_number || 'open'} pending</span>
                  <svg
                    class="pending-pill-popout"
                    width="11"
                    height="11"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                    <polyline points="15 3 21 3 21 9"></polyline>
                    <line x1="10" y1="14" x2="21" y2="3"></line>
                  </svg>
                </a>
              {:else}
                <span
                  class="pending-pill"
                  title="Deploy PR #{envView.pending_pr.pr_number || ''} open for {envView.pending_pr.target_version}"
                >
                  <span class="pending-pill-ring"></span>
                  <span>PR #{envView.pending_pr.pr_number || 'open'} pending</span>
                </span>
              {/if}
            {/if}
          </div>

          <div class="env-meta-details">
            <div class="meta-item">
              <UserAvatar name={envView.deployed_by} size={15} />
              <span class="meta-val author"
                >{envView.deployed_by || "system"}</span
              >
              <span class="dot">•</span>
              <span class="meta-val time"
                >{formatRelative(envView.last_deployed)}</span
              >
            </div>
            <div class="meta-item file-item">
              <code class="meta-code" title={envView.target_file}
                >{envView.target_file}</code
              >
            </div>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <!-- Compact Context Metadata Bar -->
  <div class="repo-meta-strip">
    {#if getSourceUrl(service.source)}
      {@const sourceUrl = getSourceUrl(service.source)}
      <a
        class="meta-pill meta-link"
        href={sourceUrl}
        target="_blank"
        rel="noopener noreferrer"
        title="Open {service.source.repository} on GitHub"
      >
        <span class="meta-k">Source:</span>
        <span class="meta-context-wrap">
          <SourceIcon type={service.source.type} size={13} class="meta-provider-icon" />
          <span class="meta-v">{formatShortRepo(service.source.repository)}</span>
          <svg
            class="meta-external-icon"
            width="10"
            height="10"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
            <polyline points="15 3 21 3 21 9"></polyline>
            <line x1="10" y1="14" x2="21" y2="3"></line>
          </svg>
        </span>
      </a>
    {:else}
      <button
        class="meta-pill"
        onclick={() =>
          copyToClipboard(
            "source",
            `${service.source.type} (${service.source.repository})`,
          )}
        type="button"
        title="{service.source.type} ({service.source.repository}) — Click to copy"
      >
        <span class="meta-k">Source:</span>
        <span class="meta-context-wrap">
          <SourceIcon type={service.source.type} size={13} class="meta-provider-icon" />
          <span class="meta-v">{formatShortRepo(service.source.repository)}</span>
        </span>
        {#if copiedField === "source"}
          <span class="copied-indicator">Copied!</span>
        {/if}
      </button>
    {/if}

    {#if service.source.filter}
      <button
        class="meta-pill"
        onclick={() => copyToClipboard("filter", service.source.filter || "")}
        type="button"
        title="{service.source.filter} — Click to copy"
      >
        <span class="meta-k">Filter:</span>
        <code class="meta-c">{service.source.filter}</code>
        {#if copiedField === "filter"}
          <span class="copied-indicator">Copied!</span>
        {/if}
      </button>
    {/if}

    {#if service.context?.repo}
      {@const repoUrl = getContextRepoUrl(service.context.repo)}
      {#if repoUrl}
        <a
          class="meta-pill meta-link"
          href={repoUrl}
          target="_blank"
          rel="noopener noreferrer"
          title="Open {service.context.repo} on GitHub"
        >
          <span class="meta-k">Context:</span>
          <span class="meta-context-wrap">
            {#if repoUrl.toLowerCase().includes('gitlab')}
              <svg
                class="meta-provider-icon"
                width="13"
                height="13"
                viewBox="0 0 24 24"
                fill="currentColor"
                aria-hidden="true"
              >
                <path d="M22.65 14.39L20.6 8.04a.79.79 0 0 0-.3-.41.8.8 0 0 0-.52-.08.8.8 0 0 0-.44.25.82.82 0 0 0-.17.51l-1.39 4.27H6.22l-1.39-4.27a.82.82 0 0 0-.17-.51.8.8 0 0 0-.44-.25.8.8 0 0 0-.52.08.79.79 0 0 0-.3.41L1.35 14.39a.84.84 0 0 0 .3.94l10.05 7.3a.58.58 0 0 0 .6 0l10.05-7.3a.84.84 0 0 0 .3-.94z"/>
              </svg>
            {:else}
              <svg
                class="meta-provider-icon"
                width="13"
                height="13"
                viewBox="0 0 24 24"
                fill="currentColor"
                aria-hidden="true"
              >
                <path
                  fill-rule="evenodd"
                  clip-rule="evenodd"
                  d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
                />
              </svg>
            {/if}
            <span class="meta-v">{formatShortPath(service.context.repo)}</span>
            <svg
              class="meta-external-icon"
              width="10"
              height="10"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
              <polyline points="15 3 21 3 21 9"></polyline>
              <line x1="10" y1="14" x2="21" y2="3"></line>
            </svg>
          </span>
        </a>
      {:else}
        <button
          class="meta-pill"
          onclick={() => copyToClipboard("context", service.context.repo)}
          type="button"
          title="{service.context.repo} - Click to copy"
        >
          <span class="meta-k">Context:</span>
          <span class="meta-v">{formatShortPath(service.context.repo)}</span>
          {#if copiedField === "context"}
            <span class="copied-indicator">Copied!</span>
          {/if}
        </button>
      {/if}
    {/if}
  </div>

  <!-- Bottom Section: Artifacts & Release History as its OWN floating pane -->
  <section class="floating-pane artifacts-pane">
    <div class="artifacts-pane-header">
      <div class="pane-header-row">
        <div class="title-with-count">
          <h2 class="pane-title">Artifacts & Release History</h2>
          <span class="count-pill">{versions.length} versions</span>
        </div>
        <div class="pane-header-controls">
          <div class="sort-toggle-group" role="group" aria-label="Order versions by">
            <button
              type="button"
              class="sort-toggle-btn"
              class:is-active={versionSort === 'published'}
              onclick={setVersionSort.bind(null, 'published')}
              aria-pressed={versionSort === 'published'}
              title="Order by date published (newest first)"
            >
              <svg
                class="sort-toggle-icon"
                width="12"
                height="12"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <circle cx="12" cy="12" r="10"></circle>
                <polyline points="12 6 12 12 16 14"></polyline>
              </svg>
              <span>Published</span>
            </button>
            <button
              type="button"
              class="sort-toggle-btn"
              class:is-active={versionSort === 'version'}
              onclick={setVersionSort.bind(null, 'version')}
              aria-pressed={versionSort === 'version'}
              title="Order by version number (highest first)"
            >
              <svg
                class="sort-toggle-icon"
                width="12"
                height="12"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"></path>
                <line x1="7" y1="7" x2="7.01" y2="7"></line>
              </svg>
              <span>Version</span>
            </button>
          </div>
          <button
            type="button"
            class="notes-toggle-pill"
            class:is-active={showExpandedNotes}
            onclick={toggleExpandedNotes}
            title={showExpandedNotes ? "Collapse release notes" : "Show more release notes per version"}
            aria-pressed={showExpandedNotes}
          >
            <svg
              class="notes-toggle-icon"
              width="13"
              height="13"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
              <polyline points="14 2 14 8 20 8"></polyline>
              <line x1="16" y1="13" x2="8" y2="13"></line>
              <line x1="16" y1="17" x2="8" y2="17"></line>
              <polyline points="10 9 9 9 8 9"></polyline>
            </svg>
            <span class="notes-toggle-label">
              <span class="label-full">Release notes</span>
              <span class="label-short">Notes</span>
            </span>
            <span class="notes-toggle-track" aria-hidden="true">
              <span class="notes-toggle-thumb"></span>
            </span>
          </button>
        </div>
      </div>
      <p class="pane-subtitle">
        Upstream release candidates and complete deployment lifetimes
      </p>
    </div>

    {#if loadingArtifacts}
      <div class="loading-box">
        <div class="spinner"></div>
        <span>Loading artifacts & history timeline...</span>
      </div>
    {:else if versions.length === 0}
      <div class="empty-box">No artifacts found for this service.</div>
    {:else}
      <div
        class="table-responsive-wrapper"
        bind:this={tableWrapperEl}
        onscroll={handleTableScroll}
      >
        <table
          class="artifacts-data-table"
          class:has-expanded-notes={showExpandedNotes}
          class:has-env-hovered={hoveredEnv !== null}
        >
          <thead>
            <tr>
              <th class="th-version">Version</th>
              <th class="th-status"><span class="th-status-title">Deployment Status</span></th>
              <th class="th-commit"><span class="th-commit-full">Deploy Commit</span><span class="th-commit-short">Commit</span></th>
              <th class="th-prs">Included PRs</th>
              <th class="th-time">Published</th>
              <th class="th-actions">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each visibleVersions as art, artIndex}
              {@const evts = getEventsForVersion(art.version)}
              {@const activeEvt =
                evts.find((e) => e.is_active) ||
                (evts.length > 0 ? evts[0] : null)}
              {@const dots = getDeploymentDots(art.version)}
              {@const hasActive = dots.some((d) => d.isActive)}
              {@const isExpanded = expandedDiffVersion === art.version}
              {@const isHighlighted = hoveredEnv
                ? isEnvActiveOnVersion(hoveredEnv, art.version)
                : false}
              {@const vPRs = releasePRs[art.version] || extractPRsFromText(art.changelog)}

              <tr
                class="table-row"
                class:has-active={hasActive}
                class:env-highlighted={isHighlighted}
                class:row-expanded-notes={showExpandedNotes}
                data-highlight-env={isHighlighted
                  ? getEnvBadgeClass(hoveredEnv || "")
                  : undefined}
              >
                <!-- Version Column -->
                <td class="td-version">
                  <div class="version-cell" class:has-expanded={showExpandedNotes}>
                    <div class="version-name-row">
                      <span class="version-name">{art.version}</span>
                      {#if isHighlighted && hoveredEnv}
                        <span
                          class="deployed-highlight-badge {getEnvBadgeClass(
                            hoveredEnv,
                          )}"
                        >
                          Active in {hoveredEnv}
                        </span>
                      {/if}
                      {#if art.prerelease}
                        <span class="prerelease-tag">Pre-release</span>
                      {/if}
                    </div>

                    {#if !showExpandedNotes}
                      {#if art.changelog}
                        <button
                          type="button"
                          class="changelog-preview-btn"
                          onclick={(e) => {
                            e.stopPropagation();
                            selectedChangelog = art;
                          }}
                          title="View release notes"
                        >
                          <span class="changelog-preview-text"
                            >{getChangelogSnippet(art.changelog)}</span
                          >
                          <span class="changelog-more-tag">more</span>
                        </button>
                      {/if}
                    {:else}
                      <div class="version-notes-block">
                        {#if art.changelog}
                          {@const previewLines = getChangelogExpandedLines(art.changelog, 3)}
                          <div class="version-notes-content">
                            {#each previewLines as line}
                              <div class="version-notes-item">
                                <span class="notes-bullet">•</span>
                                <span class="notes-text">{line}</span>
                              </div>
                            {/each}
                          </div>
                          <button
                            type="button"
                            class="version-notes-more-btn"
                            onclick={(e) => {
                              e.stopPropagation();
                              selectedChangelog = art;
                            }}
                            title="View full release notes"
                          >
                            more
                          </button>
                        {:else}
                          <span class="version-notes-empty">No release notes</span>
                        {/if}
                      </div>
                    {/if}
                  </div>
                </td>

                <!-- Status Column (Colored dots for active, grey dots for previous) -->
                <td class="td-status">
                  {#if dots.length === 0}
                    <span class="muted-dash">—</span>
                  {:else}
                    <div class="status-dots-group">
                      {#each dots as dot}
                        <span
                          class="status-dot-target"
                          class:is-active-target={hoveredEnv ===
                            dot.environment && (dot.isActive || dot.isDeploying || dot.isPending || dot.isFailed)}
                          class:is-failed-target={dot.isFailed}
                          aria-label="{dot.environment} ({dot.isActive
                            ? 'Active'
                            : dot.isDeploying
                              ? 'Deploying'
                              : dot.isPending
                                ? 'Pending'
                                : dot.isFailed
                                  ? 'Failed'
                                  : 'Previous'})"
                        >
                          <span
                            class="status-dot-circle"
                            class:is-previous={!dot.isActive && !dot.isPending && !dot.isDeploying && !dot.isFailed}
                            class:is-active={dot.isActive}
                            class:is-pending={dot.isPending}
                            class:is-deploying={dot.isDeploying}
                            class:is-failed={dot.isFailed}
                            class:badge-prod={getEnvBadgeClass(dot.environment) ===
                              "badge-prod"}
                            class:badge-preprod={getEnvBadgeClass(dot.environment) ===
                              "badge-preprod"}
                            class:badge-staging={getEnvBadgeClass(dot.environment) ===
                              "badge-staging"}
                            class:badge-dev={getEnvBadgeClass(dot.environment) === "badge-dev"}
                            class:badge-default={getEnvBadgeClass(dot.environment) ===
                              "badge-default"}
                          >
                            {#if dot.isFailed}
                              <span class="failed-inner-mark">!</span>
                            {/if}
                          </span>

                          <!-- Rich Custom Tooltip -->
                          <div
                            class="dot-tooltip"
                            class:tooltip-up={artIndex >=
                              Math.max(0, visibleVersions.length - 3)}
                            role="tooltip"
                          >
                            <div class="tooltip-header">
                              <span
                                class="tooltip-header-dot"
                                class:is-previous={!dot.isActive && !dot.isPending && !dot.isDeploying && !dot.isFailed}
                                class:is-pending={dot.isPending}
                                class:is-deploying={dot.isDeploying}
                                class:is-failed={dot.isFailed}
                                class:badge-prod={getEnvBadgeClass(dot.environment) ===
                                  "badge-prod"}
                                class:badge-preprod={getEnvBadgeClass(dot.environment) ===
                                  "badge-preprod"}
                                class:badge-staging={getEnvBadgeClass(dot.environment) ===
                                  "badge-staging"}
                                class:badge-dev={getEnvBadgeClass(dot.environment) ===
                                  "badge-dev"}
                                class:badge-default={getEnvBadgeClass(dot.environment) ===
                                  "badge-default"}
                              >
                                {#if dot.isFailed}
                                  <span class="failed-inner-mark mini">!</span>
                                {/if}
                              </span>
                              <span class="tooltip-env-title"
                                >{dot.environment}</span
                              >
                              <span
                                class="tooltip-status-tag"
                                class:tag-active={dot.isActive}
                                class:tag-deploying={dot.isDeploying}
                                class:tag-pending={dot.isPending}
                                class:tag-failed={dot.isFailed}
                                class:tag-previous={!dot.isActive && !dot.isPending && !dot.isDeploying && !dot.isFailed}
                              >
                                {#if dot.isActive}
                                  Active
                                {:else if dot.isDeploying}
                                  Deploying
                                {:else if dot.isPending}
                                  Pending
                                {:else if dot.isFailed}
                                  Failed
                                {:else}
                                  Previous
                                {/if}
                              </span>
                            </div>

                            <div class="tooltip-details">
                              {#if dot.isFailed}
                                <div class="tooltip-detail-row">
                                  <span class="tooltip-label">Status:</span>
                                  <span class="tooltip-value failed-status-val">
                                    {dot.statusMessage || "Deployment failed"}
                                  </span>
                                </div>
                                {#if dot.runningVersion}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Serving:</span>
                                    <span class="tooltip-value serving-status-val">
                                      {formatShortVersion(dot.runningVersion)} (still running)
                                    </span>
                                  </div>
                                {/if}
                                {#if dot.authorName}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Triggered by:</span>
                                    <span class="tooltip-value author-name">{dot.authorName}</span>
                                  </div>
                                {/if}
                                {#if dot.deployedAt}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Triggered:</span>
                                    <span class="tooltip-value"
                                      >{formatRelative(dot.deployedAt)}{formatDate(dot.deployedAt) ? ` • ${formatDate(dot.deployedAt)}` : ''}</span
                                    >
                                  </div>
                                {/if}
                              {:else if dot.isDeploying}
                                <div class="tooltip-detail-row">
                                  <span class="tooltip-label">Status:</span>
                                  <span class="tooltip-value deploying-status-val">
                                    {dot.statusMessage || "Rollout in progress..."}
                                  </span>
                                </div>
                                {#if dot.runningVersion}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Serving:</span>
                                    <span class="tooltip-value serving-status-val">
                                      {formatShortVersion(dot.runningVersion)} (active until ready)
                                    </span>
                                  </div>
                                {/if}
                                {#if dot.authorName}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Triggered by:</span>
                                    <span class="tooltip-value author-name">{dot.authorName}</span>
                                  </div>
                                {/if}
                                {#if dot.deployedAt}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Started:</span>
                                    <span class="tooltip-value"
                                      >{formatRelative(dot.deployedAt)}{formatDate(dot.deployedAt) ? ` • ${formatDate(dot.deployedAt)}` : ''}</span
                                    >
                                  </div>
                                {/if}
                              {:else if dot.isPending}
                                <div class="tooltip-detail-row">
                                  <span class="tooltip-label">Status:</span>
                                  <span class="tooltip-value pending-status-val">
                                    {#if dot.prNumber}
                                      Deploy PR #{dot.prNumber} open
                                    {:else}
                                      {dot.statusMessage || "Waiting for GitOps rollout to start"}
                                    {/if}
                                  </span>
                                </div>
                                {#if dot.runningVersion}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Serving:</span>
                                    <span class="tooltip-value serving-status-val">
                                      {formatShortVersion(dot.runningVersion)} (currently active)
                                    </span>
                                  </div>
                                {/if}
                                {#if dot.authorName}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Requested by:</span>
                                    <span class="tooltip-value author-name">{dot.authorName}</span>
                                  </div>
                                {/if}
                                {#if dot.deployedAt}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Created:</span>
                                    <span class="tooltip-value"
                                      >{formatRelative(dot.deployedAt)}{formatDate(dot.deployedAt) ? ` • ${formatDate(dot.deployedAt)}` : ''}</span
                                    >
                                  </div>
                                {/if}
                                {#if dot.prUrl}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">PR Link:</span>
                                    <a
                                      href={dot.prUrl}
                                      target="_blank"
                                      rel="noopener noreferrer"
                                      class="tooltip-pr-link"
                                    >
                                      View PR &nearr;
                                    </a>
                                  </div>
                                {/if}
                              {:else}
                                {#if dot.authorName}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Deployed by:</span>
                                    <span class="tooltip-value author-name">{dot.authorName}</span>
                                  </div>
                                {/if}
                                {#if dot.deployedAt}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Deployed:</span>
                                    <span class="tooltip-value"
                                      >{formatRelative(dot.deployedAt)}{formatDate(dot.deployedAt) ? ` • ${formatDate(dot.deployedAt)}` : ''}</span
                                    >
                                  </div>
                                {/if}
                                {#if !dot.isActive && dot.retiredAt}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label">Retired:</span>
                                    <span class="tooltip-value"
                                      >{formatRelative(dot.retiredAt)}{formatDate(dot.retiredAt) ? ` • ${formatDate(dot.retiredAt)}` : ''}</span
                                    >
                                  </div>
                                {/if}
                                {#if dot.activeDuration}
                                  <div class="tooltip-detail-row">
                                    <span class="tooltip-label"
                                      >{dot.isActive
                                        ? "Duration:"
                                        : "Active for:"}</span
                                    >
                                    <span class="tooltip-value"
                                      >{dot.activeDuration}</span
                                    >
                                  </div>
                                {/if}
                              {/if}
                            </div>
                          </div>
                        </span>
                      {/each}
                    </div>
                  {/if}
                </td>

                <!-- Deploy Commit Column -->
                <td class="td-commit">
                  {#if activeEvt?.commit_sha}
                    <div class="commit-cell">
                      {#if activeEvt.pr_number}
                        {@const gitopsUrl = getGitopsPrUrl(service.gitops_repo, activeEvt.pr_number)}
                        {#if gitopsUrl}
                          <a
                            href={gitopsUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="pr-pill gitops-pr"
                            title="Open deploy PR #{activeEvt.pr_number} on GitHub"
                            onclick={(e) => e.stopPropagation()}
                          >
                            #{activeEvt.pr_number}
                          </a>
                        {:else}
                          <span class="pr-pill gitops-pr">#{activeEvt.pr_number}</span>
                        {/if}
                      {/if}
                      <span class="commit-sha" title="GitOps commit {activeEvt.commit_sha}">
                        {activeEvt.commit_sha.slice(0, 7)}
                      </span>
                    </div>
                  {:else}
                    <span class="muted-dash">—</span>
                  {/if}
                </td>

                <!-- Included PRs Column -->
                <td class="td-prs">
                  {#if vPRs.length > 0}
                    <div class="included-prs-cell">
                      {#each vPRs.slice(0, 3) as pr}
                        {@const prLink = getPrUrl(getServiceRepo(service), pr)}
                        {#if prLink}
                          <a
                            href={prLink}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="pr-pill release-pr"
                            title="Open PR #{pr} on GitHub"
                            onclick={(e) => e.stopPropagation()}
                          >
                            #{pr}
                          </a>
                        {:else}
                          <span class="pr-pill release-pr">#{pr}</span>
                        {/if}
                      {/each}
                      {#if vPRs.length > 3}
                        <button
                          type="button"
                          class="pr-more-btn"
                          title="View all {vPRs.length} PRs in release notes"
                          onclick={(e) => {
                            e.stopPropagation();
                            selectedChangelog = art;
                          }}
                        >
                          +{vPRs.length - 3} more
                        </button>
                      {/if}
                    </div>
                  {:else}
                    <span class="muted-dash">—</span>
                  {/if}
                </td>

                <!-- Age / Deployed At Column -->
                <td class="td-time">
                  {#if activeEvt?.deployed_at}
                    <span class="time-text"
                      >{formatRelative(activeEvt.deployed_at)}</span
                    >
                  {:else if art.released_at}
                    <span class="time-text"
                      >{formatRelative(art.released_at)}</span
                    >
                  {:else}
                    <span class="muted-dash">—</span>
                  {/if}
                </td>

                <!-- Actions Column: Deploy + Kebab Menu -->
                <td class="td-actions">
                  <div class="actions-group">
                    <div class="row-deploy-wrapper">
                      <button
                        class="deploy-pill-btn"
                        class:active={openDeployRowVersion === art.version}
                        class:disabled={!canDeployAny}
                        disabled={!canDeployAny}
                        title={!canDeployAny
                          ? "Insufficient permissions to deploy this service"
                          : "Deploy to environment"}
                        onclick={(e) => {
                          e.stopPropagation();
                          openKebabVersion = null;
                          openDeployRowVersion =
                            openDeployRowVersion === art.version
                              ? null
                              : art.version;
                        }}
                        type="button"
                        aria-label="Deploy {art.version}"
                        aria-expanded={openDeployRowVersion === art.version}
                      >
                        <svg
                          class="deploy-btn-icon"
                          width="13"
                          height="13"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          stroke-width="2.2"
                          stroke-linecap="round"
                          stroke-linejoin="round"
                        >
                          <path
                            d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"
                          />
                          <path
                            d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"
                          />
                          <path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0" />
                          <path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5" />
                        </svg>
                        <span class="deploy-btn-text">Deploy</span>
                        <svg
                          class="deploy-chevron"
                          class:open={openDeployRowVersion === art.version}
                          width="10"
                          height="10"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          stroke-width="2.5"
                        >
                          <polyline points="6 9 12 15 18 9"></polyline>
                        </svg>
                      </button>

                      {#if openDeployRowVersion === art.version}
                        <div
                          class="row-deploy-dropdown"
                          class:drop-up={artIndex >=
                            Math.max(0, visibleVersions.length - 2)}
                        >
                          <div class="row-deploy-header">Deploy to:</div>
                          {#each sortedEnvs as [envName, envView]}
                            {@const perm = service.user_permissions?.[envName]}
                            {@const allowed = perm ? perm.can_deploy : true}
                            {@const isCurrentEnv = isEnvActiveOnVersion(
                              envName,
                              art.version,
                            )}
                            <button
                              class="row-deploy-item"
                              class:disabled={!allowed || isCurrentEnv}
                              disabled={!allowed || isCurrentEnv}
                              title={isCurrentEnv
                                ? `Already active in ${envName}`
                                : !allowed
                                  ? perm?.reason || "No deploy permission"
                                  : `Deploy ${art.version} to ${envName}`}
                              onclick={(e) => {
                                e.stopPropagation();
                                if (!allowed || isCurrentEnv) return;
                                openDeployRowVersion = null;
                                onPromote(service, envName, art.version);
                              }}
                              type="button"
                            >
                              <div class="item-left">
                                <span
                                  class="menu-item-dot {getEnvBadgeClass(
                                    envName,
                                  )}"
                                ></span>
                                <span class="item-name">{envName}</span>
                              </div>
                              <div class="item-right">
                                {#if isCurrentEnv}
                                  <span class="current-badge">Active</span>
                                {:else if !allowed}
                                  <svg
                                    class="lock-icon"
                                    width="11"
                                    height="11"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                  >
                                    <rect
                                      x="3"
                                      y="11"
                                      width="18"
                                      height="11"
                                      rx="2"
                                      ry="2"
                                    ></rect>
                                    <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
                                  </svg>
                                {/if}
                              </div>
                            </button>
                          {/each}
                        </div>
                      {/if}
                    </div>

                    <div class="kebab-wrapper">
                      <button
                        class="kebab-btn"
                        class:active={openKebabVersion === art.version}
                        onclick={(e) => {
                          e.stopPropagation();
                          openDeployRowVersion = null;
                          openKebabVersion =
                            openKebabVersion === art.version
                              ? null
                              : art.version;
                        }}
                        type="button"
                        aria-label="More actions"
                        title="More actions"
                      >
                        <svg
                          width="16"
                          height="16"
                          viewBox="0 0 24 24"
                          fill="currentColor"
                        >
                          <circle cx="12" cy="12" r="2"></circle>
                          <circle cx="12" cy="5" r="2"></circle>
                          <circle cx="12" cy="19" r="2"></circle>
                        </svg>
                      </button>

                      {#if openKebabVersion === art.version}
                        <div
                          class="kebab-dropdown"
                          class:drop-up={artIndex >=
                            Math.max(0, visibleVersions.length - 2)}
                        >
                          <button
                            class="kebab-item"
                            type="button"
                            onclick={(e) => {
                              e.stopPropagation();
                              openKebabVersion = null;
                              toggleDiff(art.version);
                            }}
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
                              <circle cx="18" cy="18" r="3"></circle>
                              <circle cx="6" cy="6" r="3"></circle>
                              <path d="M6 9v12"></path>
                              <path d="M21 15a9 9 0 0 0-9-9H6"></path>
                            </svg>
                            <span>{isExpanded ? "Hide Diff" : "Git Diff"}</span>
                          </button>

                          {#if art.changelog}
                            <button
                              class="kebab-item"
                              type="button"
                              onclick={(e) => {
                                e.stopPropagation();
                                openKebabVersion = null;
                                selectedChangelog = art;
                              }}
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
                                <path
                                  d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"
                                ></path>
                                <polyline points="14 2 14 8 20 8"></polyline>
                                <line x1="16" y1="13" x2="8" y2="13"></line>
                                <line x1="16" y1="17" x2="8" y2="17"></line>
                                <polyline points="10 9 9 9 8 9"></polyline>
                              </svg>
                              <span>Release Notes</span>
                            </button>
                          {/if}
                        </div>
                      {/if}
                    </div>
                  </div>
                </td>
              </tr>

              <!-- Expandable Git Diff Inline Panel -->
              {#if isExpanded}
                <tr class="diff-expand-row">
                  <td colspan="6" class="diff-expand-cell">
                    <div class="diff-container-inline">
                      <DiffViewer
                        diff={diffData}
                        loading={loadingDiff}
                        environments={diffEnvironments}
                        selectedEnvironment={selectedDiffEnv}
                        targetVersion={art.version}
                        repoUrl={getContextRepoUrl(getServiceRepo(service))}
                        onSelectEnvironment={handleDiffEnvChange}
                        onClose={closeDiff}
                      />
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
            {#if hasMore}
              <tr class="scroll-loader-row" use:sentinelAction>
                <td colspan="6">
                  <div class="scroll-loader-content">
                    <span class="scroll-loader-spinner"></span>
                    <span class="scroll-loader-text">Loading older versions ({visibleVersions.length} of {versions.length})...</span>
                  </div>
                </td>
              </tr>
            {/if}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  {#if selectedChangelog}
    <ChangelogModal
      versionInfo={selectedChangelog}
      {service}
      prs={releasePRs[selectedChangelog.version] || extractPRsFromText(selectedChangelog.changelog)}
      onClose={() => (selectedChangelog = null)}
      onDeploy={onPromote}
    />
  {/if}

  {#if activeRollbackEnv}
    <RollbackModal
      {service}
      envName={activeRollbackEnv}
      events={historyMap[activeRollbackEnv] || service.environments?.[activeRollbackEnv]?.recent_events || []}
      {user}
      onClose={() => (activeRollbackEnv = null)}
      onPromote={(svc, env, ver) => {
        activeRollbackEnv = null;
        onPromote(svc, env, ver);
      }}
      onSuccess={() => {
        loadDetails(service.service);
        onSync();
      }}
    />
  {/if}
</div>

<style>
  .canvas-flow {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    height: 100%;
    max-height: 100%;
    min-height: 0;
    box-sizing: border-box;
    overflow: hidden;
  }

  /* Top Canvas Header: Floating with NO background */
  .canvas-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.75rem;
    padding: 0 0.15rem;
    background: transparent;
    border: none;
    flex-shrink: 0;
  }

  .header-main-col {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .back-btn {
    align-self: flex-start;
    background: rgba(255, 255, 255, 0.85);
    border: 1px solid rgba(203, 213, 225, 0.6);
    padding: 0.25rem 0.65rem;
    border-radius: var(--radius-pill);
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-main);
    cursor: pointer;
    margin-bottom: 0.35rem;
  }

  .title-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .title-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-main);
    flex-shrink: 0;
  }

  .page-title {
    margin: 0;
    font-size: 1.45rem;
    font-weight: 600;
    letter-spacing: -0.025em;
    color: var(--text-main);
  }

  .header-action-buttons {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .sync-git-tooltip-wrap,
  .deploy-tooltip-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .sync-icon-btn,
  .deploy-icon-btn {
    background: transparent;
    border: none;
    padding: 0.45rem;
    margin: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: var(--radius-sm, 6px);
    transition:
      color 0.15s ease,
      background-color 0.15s ease,
      transform 0.25s ease;
  }

  .sync-icon-btn:hover,
  .deploy-icon-btn:hover,
  .deploy-icon-btn.active {
    color: var(--text-main);
    background: rgba(148, 163, 184, 0.12);
  }

  :global(html.dark) .sync-icon-btn:hover,
  :global(html.dark) .deploy-icon-btn:hover,
  :global(html.dark) .deploy-icon-btn.active {
    background: rgba(255, 255, 255, 0.1);
  }

  .sync-icon-btn:active svg {
    transform: rotate(180deg);
  }

  .sync-git-tooltip-wrap .custom-tooltip,
  .deploy-tooltip-wrap .custom-tooltip {
    position: absolute;
    bottom: -32px;
    left: 50%;
    transform: translateX(-50%) translateY(4px);
    background: rgba(15, 23, 42, 0.92);
    color: #f8fafc;
    padding: 0.25rem 0.55rem;
    border-radius: var(--radius-sm, 6px);
    font-size: 0.725rem;
    font-weight: 500;
    white-space: nowrap;
    pointer-events: none;
    opacity: 0;
    visibility: hidden;
    transition:
      opacity 0.15s ease,
      transform 0.15s ease;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
    z-index: 50;
  }

  :global(html.dark) .sync-git-tooltip-wrap .custom-tooltip,
  :global(html.dark) .deploy-tooltip-wrap .custom-tooltip {
    background: rgba(30, 41, 59, 0.95);
    border: 1px solid rgba(255, 255, 255, 0.1);
  }

  .sync-git-tooltip-wrap:hover .custom-tooltip,
  .deploy-tooltip-wrap:hover .custom-tooltip {
    opacity: 1;
    visibility: visible;
    transform: translateX(-50%) translateY(0);
  }

  .deploy-dropdown-wrapper {
    position: relative;
  }

  .deploy-menu-popover {
    position: absolute;
    top: calc(100% + 8px);
    right: 0;
    min-width: 220px;
    background: rgba(255, 255, 255, 0.96);
    -webkit-backdrop-filter: blur(24px);
    backdrop-filter: blur(24px);
    border: 1px solid rgba(220, 210, 235, 0.85);
    border-radius: var(--radius-lg);
    box-shadow: none;
    padding: 0.45rem;
    z-index: 100;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    animation: fadeInDropdown 0.15s ease-out;
  }

  @keyframes fadeInDropdown {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .deploy-menu-header {
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-sub);
    padding: 0.35rem 0.65rem 0.2rem;
  }

  .deploy-menu-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 0.55rem 0.75rem;
    border-radius: var(--radius-md);
    border: none;
    background: transparent;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-main);
    transition: all 0.15s ease;
  }

  .deploy-menu-item:hover {
    background: rgba(99, 102, 241, 0.08);
    color: var(--accent-coral);
  }

  .menu-item-left {
    display: flex;
    align-items: center;
    gap: 0.55rem;
  }

  .menu-item-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-sub);
  }

  .menu-item-dot.badge-prod {
    background: #10b981;
  }

  .menu-item-dot.badge-preprod {
    background: #f59e0b;
  }

  .menu-item-dot.badge-staging {
    background: #3b82f6;
  }

  .menu-item-dot.badge-dev {
    background: #64748b;
  }

  .menu-item-dot.badge-default {
    background: #8b5cf6;
  }

  .menu-item-arrow {
    font-size: 0.85rem;
    color: var(--text-muted);
    transition: transform 0.15s ease;
  }

  .deploy-menu-item:hover .menu-item-arrow {
    transform: translateX(2px);
    color: var(--accent-coral);
  }

  .deploy-menu-item.disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .deploy-menu-item.disabled:hover {
    background: transparent;
    color: var(--text-main);
  }

  .menu-item-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    text-align: left;
  }

  .menu-item-reason {
    font-size: 0.675rem;
    color: var(--text-sub);
    font-weight: 500;
    max-width: 140px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .lock-icon {
    color: var(--text-sub);
    flex-shrink: 0;
  }

  .deploy-pill-btn.disabled {
    opacity: 0.4;
    cursor: not-allowed;
    pointer-events: none;
  }

  .plus-icon {
    font-size: 1.1rem;
    line-height: 1;
  }

  /* Floating Pane Base: Clean rounded glass cards floating directly on canvas */
  .floating-pane {
    background: var(--glass-bg);
    -webkit-backdrop-filter: blur(24px);
    backdrop-filter: blur(24px);
    border: none;
    border-radius: var(--radius-xl);
    box-shadow: none;
  }

  /* Active Environments Section */
  .env-section {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    flex-shrink: 0;
  }

  .section-title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0.15rem;
  }

  .section-heading {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    letter-spacing: -0.015em;
    color: var(--text-main);
  }

  .count-pill {
    background: rgba(255, 255, 255, 0.85);
    border: 1px solid rgba(220, 210, 235, 0.6);
    font-size: 0.72rem;
    font-weight: 600;
    color: var(--text-muted);
    padding: 0.12rem 0.5rem;
    border-radius: var(--radius-pill);
  }

  .env-panes-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 0.65rem;
  }

  .env-panes-grid:has(.env-card-pane:hover) .env-card-pane:not(:hover) {
    opacity: 0.65;
  }

  .env-card-pane {
    padding: 0.6rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    position: relative;
    overflow: hidden;
    cursor: default;
    transition:
      transform 0.2s ease,
      opacity 0.2s ease;
  }

  .env-card-pane.has-open-menu,
  .env-card-pane:has(.env-kebab-dropdown) {
    overflow: visible;
    z-index: 50;
  }

  .env-card-pane:hover {
    transform: translateY(-2px);
    opacity: 1 !important;
  }

  .env-card-pane::before {
    content: "";
    position: absolute;
    inset: 0;
    border-radius: inherit;
    pointer-events: none;
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(148, 163, 184, 0.15) 0%,
      transparent 100%
    );
    transition: opacity 0.2s ease;
  }

  .env-card-pane.badge-prod::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(16, 185, 129, 0.22) 0%,
      transparent 100%
    );
  }

  .env-card-pane.badge-preprod::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(245, 158, 11, 0.22) 0%,
      transparent 100%
    );
  }

  .env-card-pane.badge-staging::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(59, 130, 246, 0.22) 0%,
      transparent 100%
    );
  }

  .env-card-pane.badge-dev::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(100, 116, 139, 0.16) 0%,
      transparent 100%
    );
  }

  .env-card-pane.badge-default::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(139, 92, 246, 0.22) 0%,
      transparent 100%
    );
  }

  :global(html.dark) .env-card-pane.badge-prod::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(16, 185, 129, 0.28) 0%,
      transparent 100%
    );
  }

  :global(html.dark) .env-card-pane.badge-preprod::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(245, 158, 11, 0.28) 0%,
      transparent 100%
    );
  }

  :global(html.dark) .env-card-pane.badge-staging::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(59, 130, 246, 0.28) 0%,
      transparent 100%
    );
  }

  :global(html.dark) .env-card-pane.badge-dev::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(148, 163, 184, 0.2) 0%,
      transparent 100%
    );
  }

  :global(html.dark) .env-card-pane.badge-default::before {
    background: radial-gradient(
      ellipse 100% 80% at 50% 0%,
      rgba(139, 92, 246, 0.28) 0%,
      transparent 100%
    );
  }

  .env-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .env-header-actions {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .env-kebab-btn {
    width: 24px;
    height: 24px;
    padding: 0;
  }

  .env-kebab-dropdown {
    right: 0;
    top: calc(100% + 4px);
    min-width: 135px;
    z-index: 70;
  }

  .env-badge-title {
    display: flex;
    align-items: center;
    gap: 0.45rem;
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #10b981;
    flex-shrink: 0;
    box-shadow: none;
    transition: background-color 0.2s ease;
  }

  .env-card-pane.badge-prod .status-dot,
  .status-dot.badge-prod {
    background: #10b981;
  }

  .env-card-pane.badge-preprod .status-dot,
  .status-dot.badge-preprod {
    background: #f59e0b;
  }

  .env-card-pane.badge-staging .status-dot,
  .status-dot.badge-staging {
    background: #3b82f6;
  }

  .env-card-pane.badge-dev .status-dot,
  .status-dot.badge-dev {
    background: #64748b;
  }

  .env-card-pane.badge-default .status-dot,
  .status-dot.badge-default {
    background: #8b5cf6;
  }

  .env-title {
    font-size: 0.85rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-main);
  }

  .strategy-badge {
    font-size: 0.68rem;
    font-weight: 600;
    color: var(--text-muted);
    background: rgba(255, 255, 255, 0.8);
    border: 1px solid rgba(220, 210, 235, 0.6);
    padding: 0.15rem 0.45rem;
    border-radius: var(--radius-pill);
  }

  .strategy-badge.pr {
    color: #7c3aed;
    background: rgba(237, 233, 254, 0.7);
    border-color: rgba(196, 181, 253, 0.7);
  }

  .active-version-display {
    display: flex;
    align-items: baseline;
    gap: 0.45rem;
    padding: 0.1rem 0;
  }

  .version-value {
    font-size: 1.35rem;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-main);
    letter-spacing: -0.02em;
    line-height: 1.2;
  }

  .pr-pill {
    background: rgba(237, 233, 254, 0.9);
    color: #6d28d9;
    font-family: var(--font-mono);
    font-size: 0.72rem;
    font-weight: 600;
    padding: 0.1rem 0.4rem;
    border-radius: var(--radius-pill);
  }

  .env-meta-details {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    padding-top: 0.45rem;
  }

  .meta-item {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    overflow: hidden;
  }

  .meta-val.author {
    font-weight: 600;
    color: var(--text-main);
  }

  .dot {
    color: var(--text-sub);
  }

  .meta-code {
    font-family: var(--font-mono);
    font-size: 0.7rem;
    background: rgba(240, 235, 248, 0.7);
    padding: 0.1rem 0.35rem;
    border-radius: 4px;
    color: var(--text-main);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }

  /* Repo Context Strip: Compact single-line pill bar */
  .repo-meta-strip {
    display: flex;
    align-items: center;
    flex-wrap: nowrap;
    gap: 0.4rem;
    padding: 0;
    margin: -0.2rem 0 0 0;
    background: transparent;
    border: none;
    box-shadow: none;
    overflow-x: auto;
    scrollbar-width: none;
    -ms-overflow-style: none;
    flex-shrink: 0;
  }

  .repo-meta-strip::-webkit-scrollbar {
    display: none;
  }

  .meta-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.15rem 0.5rem;
    border-radius: var(--radius-pill, 9999px);
    background: rgba(100, 116, 139, 0.08);
    border: 1px solid rgba(100, 116, 139, 0.16);
    font-size: 0.7rem;
    cursor: pointer;
    color: var(--text-muted);
    transition: all 0.15s ease;
    white-space: nowrap;
    line-height: 1.2;
    margin: 0;
    text-align: left;
    text-decoration: none;
  }

  .meta-context-wrap {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .meta-provider-icon {
    flex-shrink: 0;
    color: var(--text-muted);
    transition: color 0.15s ease;
  }

  .meta-pill:hover .meta-provider-icon {
    color: var(--text-main);
  }

  .meta-external-icon {
    flex-shrink: 0;
    opacity: 0.55;
    transition: opacity 0.15s ease, transform 0.15s ease;
  }

  .meta-pill:hover .meta-external-icon {
    opacity: 0.95;
    transform: translate(1px, -1px);
  }

  :global(html.dark) .meta-pill {
    background: rgba(255, 255, 255, 0.04);
    border-color: rgba(255, 255, 255, 0.09);
  }

  .meta-pill:hover {
    background: rgba(100, 116, 139, 0.15);
    border-color: rgba(100, 116, 139, 0.3);
    color: var(--text-main);
  }

  :global(html.dark) .meta-pill:hover {
    background: rgba(255, 255, 255, 0.08);
    border-color: rgba(255, 255, 255, 0.16);
  }

  .meta-k {
    font-weight: 700;
    font-size: 0.65rem;
    text-transform: uppercase;
    color: var(--text-sub);
    letter-spacing: 0.05em;
  }

  .meta-badge {
    text-transform: uppercase;
    font-size: 0.625rem;
    font-weight: 700;
    color: var(--accent-color, #3b82f6);
    background: rgba(59, 130, 246, 0.1);
    padding: 0.08rem 0.3rem;
    border-radius: 4px;
  }

  .meta-v {
    font-weight: 500;
    color: var(--text-main);
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta-c {
    font-family: var(--font-mono);
    color: var(--text-muted);
    font-size: 0.7rem;
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    background: transparent;
    padding: 0;
  }

  .copied-indicator {
    font-size: 0.65rem;
    font-weight: 600;
    color: #10b981;
    margin-left: 0.15rem;
    animation: fadeIn 0.15s ease-out;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: scale(0.9);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  /* Artifacts & Release History Floating Pane */
  .artifacts-pane {
    padding: 0.85rem 1.15rem;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .artifacts-pane-header {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    flex-shrink: 0;
  }

  .pane-header-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.75rem;
  }

  .pane-header-controls {
    display: inline-flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .sort-toggle-group {
    display: inline-flex;
    align-items: center;
    padding: 2px;
    border-radius: var(--radius-pill);
    background: rgba(241, 245, 249, 0.6);
    border: 1px solid rgba(226, 232, 240, 0.8);
    gap: 2px;
  }

  :global(html.dark) .sort-toggle-group {
    background: rgba(15, 23, 42, 0.4);
    border-color: rgba(51, 65, 85, 0.6);
  }

  .sort-toggle-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.16rem 0.5rem;
    border: none;
    border-radius: var(--radius-pill);
    background: transparent;
    color: var(--text-muted);
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
    user-select: none;
    -webkit-user-select: none;
  }

  :global(html.dark) .sort-toggle-btn {
    color: #94a3b8;
  }

  .sort-toggle-btn:hover {
    color: var(--text-main);
  }

  :global(html.dark) .sort-toggle-btn:hover {
    color: #f1f5f9;
  }

  .sort-toggle-btn.is-active {
    background: #ffffff;
    color: #2563eb;
    box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
  }

  :global(html.dark) .sort-toggle-btn.is-active {
    background: rgba(59, 130, 246, 0.22);
    color: #93c5fd;
    box-shadow: none;
  }

  .sort-toggle-icon {
    flex-shrink: 0;
    opacity: 0.8;
  }

  .sort-toggle-btn.is-active .sort-toggle-icon {
    opacity: 1;
  }

  .notes-toggle-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.2rem 0.55rem;
    border-radius: var(--radius-pill);
    background: rgba(241, 245, 249, 0.6);
    border: 1px solid rgba(226, 232, 240, 0.8);
    color: var(--text-muted);
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
    user-select: none;
    -webkit-user-select: none;
  }

  :global(html.dark) .notes-toggle-pill {
    background: rgba(15, 23, 42, 0.4);
    border-color: rgba(51, 65, 85, 0.6);
    color: #94a3b8;
  }

  .notes-toggle-pill:hover {
    color: var(--text-main);
    border-color: rgba(148, 163, 184, 0.5);
    background: rgba(241, 245, 249, 0.9);
  }

  :global(html.dark) .notes-toggle-pill:hover {
    border-color: rgba(100, 116, 139, 0.6);
    background: rgba(30, 41, 59, 0.6);
    color: #f1f5f9;
  }

  .notes-toggle-pill.is-active {
    background: rgba(59, 130, 246, 0.1);
    border-color: rgba(59, 130, 246, 0.4);
    color: #2563eb;
  }

  :global(html.dark) .notes-toggle-pill.is-active {
    background: rgba(59, 130, 246, 0.18);
    border-color: rgba(96, 165, 250, 0.45);
    color: #93c5fd;
  }

  .notes-toggle-icon {
    flex-shrink: 0;
    opacity: 0.8;
  }

  .notes-toggle-pill.is-active .notes-toggle-icon {
    opacity: 1;
  }

  .notes-toggle-label {
    letter-spacing: -0.01em;
  }

  .notes-toggle-label .label-short {
    display: none;
  }

  .notes-toggle-track {
    width: 22px;
    height: 12px;
    border-radius: 9999px;
    background: rgba(148, 163, 184, 0.3);
    position: relative;
    transition: background-color 0.15s ease;
    flex-shrink: 0;
  }

  :global(html.dark) .notes-toggle-track {
    background: rgba(100, 116, 139, 0.4);
  }

  .notes-toggle-pill.is-active .notes-toggle-track {
    background: #3b82f6;
  }

  .notes-toggle-thumb {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #ffffff;
    position: absolute;
    top: 2px;
    left: 2px;
    transition: transform 0.15s ease;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
  }

  .notes-toggle-pill.is-active .notes-toggle-thumb {
    transform: translateX(10px);
  }

  .title-with-count {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .pane-title {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    letter-spacing: -0.015em;
    color: var(--text-main);
  }

  .pane-subtitle {
    margin: 0;
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  /* Table styling - NO BACKGROUND ON ITEMS, super simple clean table with headers */
  .table-responsive-wrapper {
    width: 100%;
    flex: 1;
    min-height: 0;
    overflow-x: auto;
    overflow-y: auto;
  }

  .artifacts-data-table {
    width: 100%;
    table-layout: fixed;
    border-collapse: separate;
    border-spacing: 0;
    text-align: left;
  }

  .artifacts-data-table thead {
    position: sticky;
    top: 0;
    z-index: 10;
    -webkit-backdrop-filter: blur(6px) brightness(1.5) contrast(0.9);
    backdrop-filter: blur(6px) brightness(1.5) contrast(0.9);
    border-radius: 10px;
    overflow: hidden;
  }

  .artifacts-data-table thead tr {
    height: 45px;
  }

  .artifacts-data-table th {
    position: sticky;
    top: 0;
    z-index: 10;
    height: 45px;
    box-sizing: border-box;
    padding: 0 0.75rem;
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    vertical-align: middle;
  }

  .artifacts-data-table th:first-child {
    border-top-left-radius: 8px;
  }

  .artifacts-data-table th:last-child {
    border-top-right-radius: 8px;
  }

  .artifacts-data-table th.th-version {
    width: auto;
  }

  .artifacts-data-table th.th-status {
    width: 160px;
  }

  .artifacts-data-table th.th-commit {
    width: 135px;
  }

  .artifacts-data-table th.th-prs {
    width: 125px;
  }

  .artifacts-data-table th.th-time {
    width: 95px;
  }

  .artifacts-data-table th.th-actions {
    width: 135px;
    text-align: right;
    padding-right: 1.25rem;
  }

  .artifacts-data-table td {
    padding: 0.15rem 0.75rem;
    height: 45px;
    box-sizing: border-box;
    vertical-align: middle;
    border-bottom: none;
    background: transparent;
  }

  .scroll-loader-row td {
    padding: 1rem 0.5rem;
    text-align: center;
  }

  .scroll-loader-content {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.65rem;
    color: var(--text-muted);
    font-size: 0.78rem;
    font-weight: 500;
  }

  .scroll-loader-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(100, 116, 139, 0.2);
    border-top-color: var(--text-muted);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  /* Table row transitions and hover states */
  .artifacts-data-table.has-env-hovered .table-row:not(.env-highlighted) {
    opacity: 0.35;
  }

  .artifacts-data-table tbody tr {
    height: 45px;
  }

  .table-row {
    background: transparent;
    border: none;
    box-shadow: none;
    height: 45px;
    transition:
      background-color 0.15s ease,
      opacity 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  }

  .table-row.env-highlighted {
    opacity: 1 !important;
    background-color: rgba(255, 255, 255, 0.85);
  }

  .table-row.env-highlighted[data-highlight-env="badge-prod"] {
    background-color: rgba(16, 185, 129, 0.14);
  }

  .table-row.env-highlighted[data-highlight-env="badge-preprod"] {
    background-color: rgba(245, 158, 11, 0.16);
  }

  .table-row.env-highlighted[data-highlight-env="badge-staging"] {
    background-color: rgba(59, 130, 246, 0.14);
  }

  .table-row.env-highlighted[data-highlight-env="badge-dev"] {
    background-color: rgba(100, 116, 139, 0.12);
  }

  .table-row.env-highlighted[data-highlight-env="badge-default"] {
    background-color: rgba(139, 92, 246, 0.14);
  }

  :global(html.dark) .table-row.env-highlighted {
    background-color: rgba(255, 255, 255, 0.12);
  }

  :global(html.dark)
    .table-row.env-highlighted[data-highlight-env="badge-prod"] {
    background-color: rgba(16, 185, 129, 0.22);
  }

  :global(html.dark)
    .table-row.env-highlighted[data-highlight-env="badge-preprod"] {
    background-color: rgba(245, 158, 11, 0.24);
  }

  :global(html.dark)
    .table-row.env-highlighted[data-highlight-env="badge-staging"] {
    background-color: rgba(59, 130, 246, 0.22);
  }

  :global(html.dark)
    .table-row.env-highlighted[data-highlight-env="badge-dev"] {
    background-color: rgba(100, 116, 139, 0.2);
  }

  :global(html.dark)
    .table-row.env-highlighted[data-highlight-env="badge-default"] {
    background-color: rgba(139, 92, 246, 0.22);
  }

  .table-row.env-highlighted .version-name {
    font-weight: 700;
  }

  .version-cell {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.1rem;
    min-width: 0;
    max-width: 100%;
    width: 100%;
  }

  .version-cell.has-expanded {
    gap: 0.25rem;
  }

  .version-notes-block {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    margin-top: 0.25rem;
    width: 100%;
  }

  .version-notes-content {
    display: flex;
    flex-direction: column;
    gap: 0.18rem;
  }

  .version-notes-item {
    display: flex;
    align-items: flex-start;
    gap: 0.35rem;
    font-size: 0.72rem;
    line-height: 1.35;
    color: var(--text-muted);
  }

  :global(html.dark) .version-notes-item {
    color: #94a3b8;
  }

  .notes-bullet {
    color: var(--text-muted);
    opacity: 0.6;
    font-size: 0.75rem;
    line-height: 1.35;
    flex-shrink: 0;
  }

  .notes-text {
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    word-break: break-word;
  }

  .version-notes-more-btn {
    align-self: flex-start;
    padding: 0.05rem 0.3rem;
    margin-left: 0.65rem;
    background: transparent;
    border: none;
    border-radius: var(--radius-sm);
    color: #3b82f6;
    font-size: 0.68rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s ease;
  }

  :global(html.dark) .version-notes-more-btn {
    color: #60a5fa;
  }

  .version-notes-more-btn:hover {
    text-decoration: underline;
  }

  .version-notes-empty {
    font-size: 0.7rem;
    font-style: italic;
    color: var(--text-muted);
    opacity: 0.65;
    margin-top: 0.1rem;
  }

  .artifacts-data-table.has-expanded-notes tbody tr.table-row {
    height: auto;
    min-height: 75px;
  }

  .artifacts-data-table.has-expanded-notes td {
    height: auto;
    vertical-align: top;
    padding-top: 0.75rem;
    padding-bottom: 0.75rem;
  }

  .artifacts-data-table.has-expanded-notes .td-status .status-dots-group {
    padding-top: 0.2rem;
  }

  @media (min-width: 861px) {
    .artifacts-data-table.has-expanded-notes .th-version,
    .artifacts-data-table.has-expanded-notes .td-version {
      min-width: 180px;
      max-width: 520px;
    }
  }

  .version-name-row {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
  }

  .changelog-preview-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.1rem 0.35rem;
    margin-left: -0.35rem;
    background: transparent;
    border: none;
    border-radius: var(--radius-sm);
    cursor: pointer;
    text-align: left;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    box-sizing: border-box;
    transition: all 0.15s ease;
  }

  .changelog-preview-btn:hover {
    background: rgba(100, 116, 139, 0.1);
  }

  .changelog-preview-text {
    font-size: 0.73rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    flex: 1 1 auto;
  }

  .changelog-preview-btn:hover .changelog-preview-text {
    color: var(--text-main);
  }

  .changelog-more-tag {
    font-size: 0.68rem;
    font-weight: 600;
    color: var(--text-muted);
    opacity: 0.85;
    flex-shrink: 0;
  }

  :global(html.dark) .changelog-more-tag {
    color: #94a3b8;
  }

  .changelog-preview-btn:hover .changelog-more-tag {
    opacity: 1;
    text-decoration: underline;
  }

  .version-name {
    font-family: var(--font-mono);
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-main);
  }

  .prerelease-tag {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    background: #fef3c7;
    color: #d97706;
    padding: 0.15rem 0.4rem;
    border-radius: var(--radius-pill);
  }

  .deployed-highlight-badge {
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.15rem 0.55rem;
    border-radius: var(--radius-pill);
    display: inline-flex;
    align-items: center;
    animation: fadeInHighlight 0.15s ease-out;
  }

  .deployed-highlight-badge.badge-prod {
    background: rgba(16, 185, 129, 0.15);
    color: #10b981;
  }

  .deployed-highlight-badge.badge-preprod {
    background: rgba(245, 158, 11, 0.15);
    color: #f59e0b;
  }

  .deployed-highlight-badge.badge-staging {
    background: rgba(59, 130, 246, 0.15);
    color: #3b82f6;
  }

  .deployed-highlight-badge.badge-dev {
    background: rgba(100, 116, 139, 0.15);
    color: #64748b;
  }

  .deployed-highlight-badge.badge-default {
    background: rgba(139, 92, 246, 0.15);
    color: #8b5cf6;
  }

  @keyframes fadeInHighlight {
    from {
      opacity: 0;
      transform: scale(0.94);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  .status-dots-group {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
  }

  .status-dot-target {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    cursor: pointer;
    transition:
      transform 0.15s ease,
      background-color 0.15s ease;
  }

  .status-dot-target:hover {
    background-color: rgba(0, 0, 0, 0.06);
  }

  :global(html.dark) .status-dot-target:hover {
    background-color: rgba(255, 255, 255, 0.1);
  }

  .status-dot-circle {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    display: inline-block;
    flex-shrink: 0;
    transition:
      transform 0.15s ease,
      box-shadow 0.15s ease,
      opacity 0.15s ease;
  }

  .status-dot-target:hover .status-dot-circle,
  .status-dot-target.is-active-target .status-dot-circle {
    transform: scale(1.35);
  }

  /* Active environment dot colors */
  .status-dot-circle.badge-prod {
    background: #10b981;
    box-shadow: 0 0 6px rgba(16, 185, 129, 0.5);
  }

  .status-dot-circle.badge-preprod {
    background: #f59e0b;
    box-shadow: 0 0 6px rgba(245, 158, 11, 0.5);
  }

  .status-dot-circle.badge-staging {
    background: #3b82f6;
    box-shadow: 0 0 6px rgba(59, 130, 246, 0.5);
  }

  .status-dot-circle.badge-dev {
    background: #64748b;
    box-shadow: 0 0 6px rgba(100, 116, 139, 0.5);
  }

  .status-dot-circle.badge-default {
    background: #8b5cf6;
    box-shadow: 0 0 6px rgba(139, 92, 246, 0.5);
  }

  /* Previous/retired deployment grey dots */
  .status-dot-circle.is-previous {
    background: #94a3b8;
    box-shadow: none;
    opacity: 0.7;
  }

  :global(html.dark) .status-dot-circle.is-previous {
    background: #64748b;
  }

  .status-dot-target:hover .status-dot-circle.is-previous {
    opacity: 1;
    box-shadow: 0 0 5px rgba(148, 163, 184, 0.5);
  }

  /* Rich instant custom tooltip */
  .dot-tooltip {
    position: absolute;
    top: calc(100% + 7px);
    left: 50%;
    transform: translateX(-50%) translateY(-4px);
    background: #0f172a;
    color: #f8fafc;
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 6px;
    box-shadow:
      0 8px 24px rgba(0, 0, 0, 0.4),
      0 2px 6px rgba(0, 0, 0, 0.2);
    padding: 0.45rem 0.65rem;
    white-space: nowrap;
    pointer-events: none;
    opacity: 0;
    visibility: hidden;
    transition:
      opacity 0.12s ease,
      transform 0.12s ease,
      visibility 0.12s ease;
    z-index: 1000;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .dot-tooltip::after {
    content: "";
    position: absolute;
    bottom: 100%;
    left: 50%;
    transform: translateX(-50%);
    border-width: 5px;
    border-style: solid;
    border-color: transparent transparent #0f172a transparent;
  }

  /* Upward tooltip positioning for bottom rows */
  .dot-tooltip.tooltip-up {
    top: auto;
    bottom: calc(100% + 7px);
    transform: translateX(-50%) translateY(4px);
  }

  .dot-tooltip.tooltip-up::after {
    bottom: auto;
    top: 100%;
    border-color: #0f172a transparent transparent transparent;
  }

  .status-dot-target:hover .dot-tooltip {
    opacity: 1;
    visibility: visible;
    transform: translateX(-50%) translateY(0);
  }

  .status-dot-target:hover .dot-tooltip.tooltip-up {
    transform: translateX(-50%) translateY(0);
  }

  :global(html.dark) .dot-tooltip {
    background: #090d16;
    border-color: rgba(255, 255, 255, 0.2);
  }

  :global(html.dark) .dot-tooltip::after {
    border-color: transparent transparent #090d16 transparent;
  }

  :global(html.dark) .dot-tooltip.tooltip-up::after {
    border-color: #090d16 transparent transparent transparent;
  }

  /* Tooltip internal elements */
  .tooltip-header {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    padding-bottom: 0.25rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.12);
  }

  .tooltip-header-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .tooltip-header-dot.badge-prod {
    background: #10b981;
  }

  .tooltip-header-dot.badge-preprod {
    background: #f59e0b;
  }

  .tooltip-header-dot.badge-staging {
    background: #3b82f6;
  }

  .tooltip-header-dot.badge-dev {
    background: #64748b;
  }

  .tooltip-header-dot.badge-default {
    background: #8b5cf6;
  }

  .tooltip-header-dot.is-previous {
    background: #94a3b8;
  }

  .tooltip-env-title {
    font-size: 0.72rem;
    font-weight: 700;
    color: #ffffff;
    text-transform: capitalize;
  }

  .tooltip-status-tag {
    font-size: 0.6rem;
    font-weight: 600;
    padding: 0.08rem 0.35rem;
    border-radius: 3px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .tooltip-status-tag.tag-active {
    background: rgba(16, 185, 129, 0.2);
    color: #34d399;
    border: 1px solid rgba(16, 185, 129, 0.35);
  }

  .tooltip-status-tag.tag-previous {
    background: rgba(148, 163, 184, 0.2);
    color: #cbd5e1;
    border: 1px solid rgba(148, 163, 184, 0.3);
  }

  .tooltip-details {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .tooltip-detail-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    font-size: 0.68rem;
  }

  .tooltip-label {
    color: #94a3b8;
    font-weight: 500;
  }

  .tooltip-value {
    color: #f1f5f9;
    font-weight: 600;
  }

  .tooltip-value.author-name {
    color: #ffffff;
  }

  .commit-cell {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  /* Flashing dots for in-flight pending and deploying promotions */
  .status-dot-circle.is-pending,
  .status-dot-circle.is-deploying {
    position: relative;
    animation: dotPulseFlash 1s cubic-bezier(0.4, 0, 0.2, 1) infinite alternate;
  }

  @keyframes dotPulseFlash {
    0% {
      opacity: 1;
      transform: scale(1.15);
    }
    100% {
      opacity: 0.3;
      transform: scale(0.85);
    }
  }

  .status-dot-circle.is-pending::after,
  .status-dot-circle.is-deploying::after {
    content: "";
    position: absolute;
    top: -2px;
    left: -2px;
    right: -2px;
    bottom: -2px;
    border-radius: 50%;
    border: 1.5px solid currentColor;
    opacity: 0;
    animation: dotPing 1.5s cubic-bezier(0, 0, 0.2, 1) infinite;
  }

  @keyframes dotPing {
    0% {
      transform: scale(1);
      opacity: 0.85;
    }
    75%, 100% {
      transform: scale(2);
      opacity: 0;
    }
  }

  .status-dot-circle.is-pending.badge-prod,
  .status-dot-circle.is-deploying.badge-prod {
    background: #10b981 !important;
    color: #10b981;
    box-shadow: 0 0 8px rgba(16, 185, 129, 0.7);
  }

  .status-dot-circle.is-pending.badge-preprod,
  .status-dot-circle.is-deploying.badge-preprod {
    background: #f59e0b !important;
    color: #f59e0b;
    box-shadow: 0 0 8px rgba(245, 158, 11, 0.7);
  }

  .status-dot-circle.is-pending.badge-staging,
  .status-dot-circle.is-deploying.badge-staging {
    background: #3b82f6 !important;
    color: #3b82f6;
    box-shadow: 0 0 8px rgba(59, 130, 246, 0.7);
  }

  .status-dot-circle.is-pending.badge-dev,
  .status-dot-circle.is-deploying.badge-dev {
    background: #64748b !important;
    color: #64748b;
    box-shadow: 0 0 8px rgba(100, 116, 139, 0.7);
  }

  .status-dot-circle.is-pending.badge-default,
  .status-dot-circle.is-deploying.badge-default {
    background: #8b5cf6 !important;
    color: #8b5cf6;
    box-shadow: 0 0 8px rgba(139, 92, 246, 0.7);
  }

  /* Clear failed state on failure */
  .status-dot-circle.is-failed {
    background: #ef4444 !important;
    color: #ef4444;
    box-shadow: 0 0 0 2px rgba(239, 68, 68, 0.4), 0 0 8px rgba(239, 68, 68, 0.7) !important;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    position: relative;
  }

  .status-dot-circle.is-failed .failed-inner-mark {
    font-size: 7px;
    font-weight: 900;
    line-height: 1;
    color: #ffffff;
    user-select: none;
    transform: translateY(-0.5px);
  }

  .status-dot-target:hover .status-dot-circle.is-failed {
    box-shadow: 0 0 0 2px rgba(239, 68, 68, 0.6), 0 0 10px rgba(239, 68, 68, 0.9) !important;
  }

  /* Tooltip header dot styles */
  .tooltip-header-dot.is-failed {
    background: #ef4444 !important;
    box-shadow: 0 0 6px rgba(239, 68, 68, 0.6);
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .tooltip-header-dot.is-failed .failed-inner-mark.mini {
    font-size: 6px;
    font-weight: 900;
    line-height: 1;
    color: #ffffff;
    user-select: none;
  }

  .tooltip-header-dot.is-deploying,
  .tooltip-header-dot.is-pending {
    animation: dotPulseFlash 1s cubic-bezier(0.4, 0, 0.2, 1) infinite alternate;
  }

  .tooltip-header-dot.is-deploying.badge-prod,
  .tooltip-header-dot.is-pending.badge-prod { background: #10b981; }
  .tooltip-header-dot.is-deploying.badge-preprod,
  .tooltip-header-dot.is-pending.badge-preprod { background: #f59e0b; }
  .tooltip-header-dot.is-deploying.badge-staging,
  .tooltip-header-dot.is-pending.badge-staging { background: #3b82f6; }
  .tooltip-header-dot.is-deploying.badge-dev,
  .tooltip-header-dot.is-pending.badge-dev { background: #64748b; }
  .tooltip-header-dot.is-deploying.badge-default,
  .tooltip-header-dot.is-pending.badge-default { background: #8b5cf6; }

  /* Tooltip tags and status text */
  .tooltip-status-tag.tag-deploying {
    background: rgba(59, 130, 246, 0.2);
    color: #60a5fa;
    border: 1px solid rgba(59, 130, 246, 0.4);
  }

  .tooltip-status-tag.tag-failed {
    background: rgba(239, 68, 68, 0.2);
    color: #f87171;
    border: 1px solid rgba(239, 68, 68, 0.45);
  }

  .tooltip-status-tag.tag-pending {
    background: rgba(245, 158, 11, 0.2);
    color: #fbbf24;
    border: 1px solid rgba(245, 158, 11, 0.4);
  }

  .failed-status-val {
    color: #f87171 !important;
    font-weight: 600;
  }

  .deploying-status-val {
    color: #60a5fa !important;
    font-weight: 600;
  }

  .serving-status-val {
    color: #34d399 !important;
    font-weight: 600;
  }

  .pending-status-val {
    color: #fbbf24 !important;
    font-weight: 600;
  }

  .tooltip-pr-link {
    color: #60a5fa;
    text-decoration: none;
    font-weight: 600;
  }

  .tooltip-pr-link:hover {
    text-decoration: underline;
  }

  .pending-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.68rem;
    font-weight: 600;
    color: #d97706;
    background: #fef3c7;
    padding: 0.15rem 0.5rem;
    border-radius: var(--radius-pill);
    border: 1px solid rgba(245, 158, 11, 0.3);
    animation: fadeInHighlight 0.2s ease-out;
  }

  :global(html.dark) .pending-pill {
    color: #fbbf24;
    background: rgba(245, 158, 11, 0.15);
    border-color: rgba(245, 158, 11, 0.35);
  }

  .pending-pill.is-link {
    text-decoration: none;
    cursor: pointer;
    transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  }

  .pending-pill.is-link:hover {
    background: #fde68a;
    border-color: rgba(245, 158, 11, 0.6);
    color: #b45309;
  }

  :global(html.dark) .pending-pill.is-link:hover {
    background: rgba(245, 158, 11, 0.25);
    border-color: rgba(245, 158, 11, 0.55);
    color: #fcd34d;
  }

  .pending-pill-popout {
    flex-shrink: 0;
    opacity: 0.75;
    transition: transform 0.15s ease, opacity 0.15s ease;
  }

  .pending-pill.is-link:hover .pending-pill-popout {
    opacity: 1;
    transform: translate(1px, -1px);
  }

  .pending-pill-ring {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    border: 2px solid #f59e0b;
    background: transparent;
    animation: pendingRingPulse 2s infinite ease-in-out;
  }

  .env-status-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.65rem;
    font-weight: 600;
    line-height: 1;
    padding: 0.2rem 0.5rem;
    border-radius: var(--radius-pill);
    letter-spacing: 0.01em;
  }

  .env-status-pill .status-pill-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }

  .env-status-pill.ready {
    color: #059669;
    background: rgba(16, 185, 129, 0.12);
    border: 1px solid rgba(16, 185, 129, 0.25);
  }

  :global(html.dark) .env-status-pill.ready {
    color: #34d399;
    background: rgba(16, 185, 129, 0.15);
    border-color: rgba(16, 185, 129, 0.35);
  }

  .env-status-pill.deploying {
    color: #d97706;
    background: rgba(245, 158, 11, 0.12);
    border: 1px solid rgba(245, 158, 11, 0.3);
  }

  :global(html.dark) .env-status-pill.deploying {
    color: #fbbf24;
    background: rgba(245, 158, 11, 0.15);
    border-color: rgba(245, 158, 11, 0.35);
  }

  .env-status-pill.deploying .status-pill-dot {
    animation: statusDotPulse 1.8s infinite ease-in-out;
  }

  .env-status-pill.pending {
    color: #4f46e5;
    background: rgba(99, 102, 241, 0.12);
    border: 1px solid rgba(99, 102, 241, 0.25);
  }

  :global(html.dark) .env-status-pill.pending {
    color: #818cf8;
    background: rgba(99, 102, 241, 0.15);
    border-color: rgba(99, 102, 241, 0.35);
  }

  .env-status-pill.pending .status-pill-dot {
    background: #6366f1;
  }

  .env-status-pill.failed {
    color: #dc2626;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.25);
  }

  :global(html.dark) .env-status-pill.failed {
    color: #f87171;
    background: rgba(239, 68, 68, 0.15);
    border-color: rgba(239, 68, 68, 0.35);
  }

  @keyframes statusDotPulse {
    0%, 100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.35;
      transform: scale(0.8);
    }
  }

  .cluster-replicas-count {
    font-size: 0.72rem;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-muted);
    padding: 0.12rem 0.38rem;
    background: rgba(0, 0, 0, 0.04);
    border-radius: 4px;
    align-self: center;
  }

  :global(html.dark) .cluster-replicas-count {
    background: rgba(255, 255, 255, 0.06);
  }

  .cluster-replicas-count.progressing {
    color: #d97706;
  }

  .cluster-replicas-count.pending {
    color: #8b5cf6;
  }

  :global(html.dark) .cluster-replicas-count.progressing {
    color: #fbbf24;
  }

  :global(html.dark) .cluster-replicas-count.pending {
    color: #c084fc;
  }

  .pr-pill {
    font-size: 0.68rem;
    font-weight: 600;
    color: #7c3aed;
    background: #ede9fe;
    padding: 0.15rem 0.45rem;
    border-radius: var(--radius-pill);
    text-decoration: none;
    display: inline-flex;
    align-items: center;
    transition: all 0.15s ease;
  }

  .pr-pill.gitops-pr {
    color: #059669;
    background: #ecfdf5;
    border: 1px solid rgba(16, 185, 129, 0.3);
  }

  :global(html.dark) .pr-pill.gitops-pr {
    color: #34d399;
    background: rgba(16, 185, 129, 0.15);
    border-color: rgba(16, 185, 129, 0.3);
  }

  .pr-pill.release-pr {
    color: #7c3aed;
    background: #ede9fe;
    border: 1px solid rgba(124, 58, 237, 0.25);
  }

  :global(html.dark) .pr-pill.release-pr {
    color: #c084fc;
    background: rgba(147, 51, 234, 0.2);
    border-color: rgba(168, 85, 247, 0.3);
  }

  .pr-pill:hover {
    text-decoration: underline;
    filter: brightness(0.95);
  }

  :global(html.dark) .pr-pill:hover {
    background: rgba(147, 51, 234, 0.35);
  }

  .commit-cell {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .included-prs-cell {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    flex-wrap: wrap;
    max-width: 240px;
  }

  .pr-more-btn {
    font-size: 0.65rem;
    font-weight: 600;
    color: var(--text-muted);
    background: rgba(100, 116, 139, 0.1);
    border: 1px solid rgba(100, 116, 139, 0.2);
    padding: 0.1rem 0.35rem;
    border-radius: var(--radius-pill);
    cursor: pointer;
    transition: all 0.15s ease;
    line-height: 1.2;
  }

  .pr-more-btn:hover {
    color: var(--text-main);
    background: rgba(100, 116, 139, 0.2);
  }

  .commit-sha {
    font-family: var(--font-mono);
    font-size: 0.75rem;
    color: var(--text-sub);
    background: rgba(255, 255, 255, 0.6);
    border: 1px solid rgba(203, 213, 225, 0.6);
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
  }

  .time-text {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .muted-dash {
    color: var(--text-sub);
    opacity: 0.6;
  }

  .actions-group {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.45rem;
  }

  .deploy-pill-btn {
    background: var(--accent-coral);
    color: #ffffff;
    border: none;
    padding: 0.35rem 0.75rem 0.35rem 0.85rem;
    border-radius: var(--radius-pill);
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
    box-shadow: none;
    transition: all 0.15s ease;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .deploy-pill-btn:hover {
    background: var(--accent-coral-hover);
    transform: translateY(-1px);
  }

  .deploy-pill-btn .deploy-btn-icon {
    display: none;
  }

  .deploy-pill-btn .deploy-chevron {
    transition: transform 0.2s ease;
  }

  .deploy-pill-btn .deploy-chevron.open {
    transform: rotate(180deg);
  }

  :global(html.dark) .deploy-pill-btn {
    color: #0f172a;
  }

  .row-deploy-wrapper {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .row-deploy-dropdown {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    z-index: 60;
    min-width: 175px;
    background: #ffffff;
    border: 1px solid rgba(226, 232, 240, 0.95);
    border-radius: var(--radius-md);
    box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.15), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
    padding: 0.35rem;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    animation: fadeInMenu 0.12s ease;
  }

  .row-deploy-dropdown.drop-up {
    top: auto;
    bottom: calc(100% + 4px);
  }

  :global(html.dark) .row-deploy-dropdown {
    background: #1e293b;
    border-color: rgba(51, 65, 85, 0.8);
    box-shadow: 0 15px 30px -5px rgba(0, 0, 0, 0.5);
  }

  .row-deploy-header {
    font-size: 0.68rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-sub);
    padding: 0.3rem 0.55rem 0.2rem 0.55rem;
  }

  .row-deploy-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border-radius: var(--radius-sm);
    border: none;
    background: transparent;
    cursor: pointer;
    font-size: 0.78rem;
    font-weight: 500;
    color: var(--text-main);
    transition: background 0.12s ease;
    text-align: left;
    width: 100%;
  }

  .row-deploy-item:hover:not(:disabled) {
    background: rgba(148, 163, 184, 0.12);
  }

  :global(html.dark) .row-deploy-item:hover:not(:disabled) {
    background: rgba(51, 65, 85, 0.6);
  }

  .row-deploy-item.disabled,
  .row-deploy-item:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .row-deploy-item .item-left {
    display: flex;
    align-items: center;
    gap: 0.45rem;
  }

  .row-deploy-item .item-name {
    font-weight: 600;
    text-transform: capitalize;
  }

  .row-deploy-item .item-right {
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .current-badge {
    font-size: 0.65rem;
    font-weight: 600;
    background: rgba(16, 185, 129, 0.15);
    color: #10b981;
    padding: 0.05rem 0.35rem;
    border-radius: var(--radius-pill);
  }

  :global(html.dark) .current-badge {
    background: rgba(16, 185, 129, 0.25);
    color: #34d399;
  }

  /* Kebab menu styling */
  .kebab-wrapper {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .kebab-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-sm);
    border: 1px solid transparent;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .kebab-btn:hover,
  .kebab-btn.active {
    background: rgba(255, 255, 255, 0.75);
    border-color: rgba(203, 213, 225, 0.8);
    color: var(--text-main);
  }

  :global(html.dark) .kebab-btn:hover,
  :global(html.dark) .kebab-btn.active {
    background: rgba(51, 65, 85, 0.6);
    border-color: rgba(71, 85, 105, 0.7);
    color: #f1f5f9;
  }

  .kebab-dropdown {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    z-index: 60;
    min-width: 145px;
    background: #ffffff;
    border: 1px solid rgba(226, 232, 240, 0.95);
    border-radius: var(--radius-md);
    box-shadow:
      0 10px 25px -5px rgba(0, 0, 0, 0.15),
      0 8px 10px -6px rgba(0, 0, 0, 0.1);
    padding: 0.35rem;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    animation: kebabFadeIn 0.12s ease-out;
  }

  .kebab-dropdown.drop-up {
    top: auto;
    bottom: calc(100% + 4px);
  }

  @keyframes kebabFadeIn {
    from {
      opacity: 0;
      transform: translateY(-4px) scale(0.97);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }

  :global(html.dark) .kebab-dropdown {
    background: #1e293b;
    border-color: rgba(51, 65, 85, 0.9);
    box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.6);
  }

  .kebab-item {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    padding: 0.45rem 0.65rem;
    border-radius: var(--radius-sm);
    border: none;
    background: transparent;
    color: var(--text-main);
    font-size: 0.78rem;
    font-weight: 500;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
    transition:
      background 0.12s ease,
      color 0.12s ease;
  }

  .kebab-item:hover {
    background: rgba(241, 245, 249, 0.9);
    color: var(--text-main);
  }

  :global(html.dark) .kebab-item {
    color: #cbd5e1;
  }

  :global(html.dark) .kebab-item:hover {
    background: rgba(51, 65, 85, 0.7);
    color: #ffffff;
  }

  .kebab-item svg {
    flex-shrink: 0;
    opacity: 0.75;
  }

  .kebab-item:hover svg {
    opacity: 1;
  }

  .diff-expand-row td {
    padding: 0;
    border-bottom: 1px solid rgba(226, 232, 240, 0.6);
    background: rgba(248, 250, 252, 0.45);
  }

  :global(html.dark) .diff-expand-row td {
    background: rgba(15, 23, 42, 0.3);
    border-bottom-color: rgba(51, 65, 85, 0.5);
  }

  .diff-expand-cell {
    padding: 0.75rem 1.25rem 1.25rem 1.25rem !important;
  }

  .diff-container-inline {
    width: 100%;
  }

  .loading-box,
  .empty-box {
    text-align: center;
    padding: 3rem 1rem;
    color: var(--text-muted);
    font-size: 0.9rem;
  }

  .spinner {
    width: 28px;
    height: 28px;
    border: 3px solid rgba(99, 102, 241, 0.2);
    border-top-color: var(--accent-coral);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    margin: 0 auto 0.75rem;
  }

  .spinner-sm {
    width: 18px;
    height: 18px;
    border: 2px solid rgba(99, 102, 241, 0.2);
    border-top-color: var(--accent-coral);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    display: inline-block;
  }

  .loading-diff-box {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    padding: 1.5rem;
    justify-content: center;
    color: var(--text-muted);
    font-size: 0.85rem;
  }

  .th-commit-short {
    display: none;
  }

  @keyframes spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }

  /* Desktop breakpoint: hide included PRs when screen narrows <= 1250px */
  @media (max-width: 1250px) {
    .th-prs,
    .td-prs {
      display: none !important;
    }
  }

  /* When screen is <= 1100px and sidebar is open, hide deploy commit column to prevent overflow */
  @media (max-width: 1100px) {
    .th-commit,
    .td-commit {
      display: none !important;
    }
  }

  /* Mobile/tablet breakpoint: flex layout, status to left, icon deploy, keep notes and commit */
  @media (max-width: 860px) {
    .env-section {
      overflow: hidden;
      margin-left: -0.25rem;
      margin-right: -0.25rem;
      padding: 0 0.25rem;
    }

    .env-panes-grid {
      display: flex;
      flex-direction: row;
      flex-wrap: nowrap;
      overflow-x: auto;
      overflow-y: hidden;
      gap: 0.75rem;
      padding-bottom: 0.4rem;
      -webkit-overflow-scrolling: touch;
      scroll-snap-type: x mandatory;
      scrollbar-width: none;
      -ms-overflow-style: none;
    }

    .env-panes-grid::-webkit-scrollbar {
      display: none;
    }

    .env-card-pane {
      flex: 0 0 240px;
      min-width: 240px;
      max-width: 260px;
      scroll-snap-align: start;
    }

    .canvas-flow {
      height: 100%;
      max-height: 100%;
      min-height: 0;
      overflow: hidden;
      padding-bottom: 0;
    }

    .artifacts-pane {
      flex: 1 1 0%;
      min-height: 0;
      overflow: hidden;
    }

    .th-commit-full {
      display: none;
    }

    .th-commit-short {
      display: inline;
    }

    .table-responsive-wrapper {
      flex: 1 1 0%;
      min-height: 0;
      overflow-x: hidden;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
    }

    .artifacts-data-table {
      display: block;
      width: 100%;
    }

    .artifacts-data-table thead,
    .artifacts-data-table tbody {
      display: block;
      width: 100%;
    }

    .artifacts-data-table thead {
      position: sticky;
      top: 0;
      z-index: 10;
    }

    .artifacts-data-table thead tr,
    .artifacts-data-table tr.table-row {
      display: flex;
      align-items: center;
      width: 100%;
      height: 45px;
      box-sizing: border-box;
      padding: 0 0.5rem;
    }

    .artifacts-data-table.has-expanded-notes tr.table-row {
      height: auto;
      min-height: 80px;
      align-items: flex-start;
      padding-top: 0.65rem;
      padding-bottom: 0.65rem;
      border-bottom: 1px solid rgba(148, 163, 184, 0.1);
    }

    .artifacts-data-table th,
    .artifacts-data-table td {
      border: none;
      box-sizing: border-box;
      position: static;
      background: transparent;
      -webkit-backdrop-filter: none;
      backdrop-filter: none;
      display: flex;
      align-items: center;
      height: 45px;
    }

    .artifacts-data-table.has-expanded-notes td {
      height: auto;
      align-items: flex-start;
    }

    .artifacts-data-table.has-expanded-notes .td-status {
      padding-top: 0.22rem !important;
    }

    .artifacts-data-table.has-expanded-notes .th-version,
    .artifacts-data-table.has-expanded-notes .td-version {
      padding-top: 0 !important;
      min-width: 0 !important;
      max-width: none !important;
      flex: 1 1 0% !important;
    }

    .artifacts-data-table.has-expanded-notes .td-commit {
      padding-top: 0.2rem !important;
    }

    .artifacts-data-table.has-expanded-notes .td-time {
      padding-top: 0.22rem !important;
    }

    .artifacts-data-table.has-expanded-notes .td-actions {
      padding-top: 0.05rem !important;
      flex: 0 0 auto !important;
    }

    /* Move deploy status to the very left without a title */
    .th-status,
    .td-status {
      order: 1;
      flex: 0 0 26px;
      width: 26px;
      min-width: 26px;
      padding: 0 0.5rem 0 0.15rem !important;
      display: inline-flex;
      align-items: center;
    }

    .th-status .th-status-title {
      display: none !important;
    }

    /* Version column next */
    .th-version,
    .td-version {
      order: 2;
      flex: 1 1 0%;
      min-width: 0;
      padding: 0 0.5rem 0 0 !important;
      justify-content: flex-start;
    }

    .version-cell {
      justify-content: center;
    }

    /* Keep first line of release notes */
    .changelog-preview-btn {
      max-width: 100%;
      min-width: 0;
    }

    /* Keep deploy commit */
    .th-commit,
    .td-commit {
      order: 3;
      flex: 0 0 auto;
      min-width: 75px;
      padding: 0 0.5rem 0 0 !important;
      justify-content: flex-start;
    }

    /* Included PRs hidden */
    .th-prs,
    .td-prs {
      display: none !important;
    }

    /* Published */
    .th-time,
    .td-time {
      order: 4;
      flex: 0 0 68px;
      min-width: 68px;
      text-align: right;
      padding: 0 0.5rem 0 0 !important;
      white-space: nowrap;
      font-size: 0.72rem;
      justify-content: flex-end;
    }

    /* Actions */
    .th-actions,
    .td-actions {
      order: 5;
      flex: 0 0 64px;
      min-width: 64px;
      text-align: right;
      padding: 0 !important;
      justify-content: flex-end;
    }

    .th-actions {
      display: flex;
      justify-content: flex-end;
    }

    /* Deploy button as icon-only on mobile */
    .deploy-pill-btn {
      padding: 0;
      width: 28px;
      height: 28px;
      min-width: 28px;
      border-radius: var(--radius-pill);
      display: inline-flex;
      align-items: center;
      justify-content: center;
    }

    .deploy-pill-btn .deploy-btn-text,
    .deploy-pill-btn .deploy-chevron {
      display: none !important;
    }

    .deploy-pill-btn .deploy-btn-icon {
      display: inline-flex;
      align-items: center;
      justify-content: center;
    }

    /* Expandable diff full width */
    .diff-expand-row {
      display: block;
      width: 100%;
    }

    .diff-expand-cell {
      display: block;
      width: 100%;
      padding: 0.5rem 0 0.75rem 0 !important;
      box-sizing: border-box;
    }

    /* Adjust dot tooltip on far left */
    .dot-tooltip {
      left: 0;
      transform: translateY(-4px);
    }

    .dot-tooltip::after {
      left: 10px;
      transform: none;
    }

    .status-dot-target:hover .dot-tooltip {
      transform: translateY(0);
    }

    .dot-tooltip.tooltip-up {
      transform: translateY(4px);
    }

    .status-dot-target:hover .dot-tooltip.tooltip-up {
      transform: translateY(0);
    }
  }

  /* Compact mobile breakpoint: drop deploy commit, keep first line of release notes */
  @media (max-width: 620px) {
    .pane-header-row {
      gap: 0.5rem;
    }

    .th-commit,
    .td-commit {
      display: none !important;
    }

    .changelog-preview-btn {
      max-width: 100%;
      min-width: 0;
    }

    .changelog-preview-btn .changelog-more-tag {
      display: inline-flex !important;
    }

    .th-time,
    .td-time {
      order: 3;
      flex: 0 0 68px;
      min-width: 68px;
    }

    .th-actions,
    .td-actions {
      order: 4;
      flex: 0 0 64px;
      min-width: 64px;
    }
  }

  @media (max-width: 520px) {
    .notes-toggle-label .label-full {
      display: none;
    }

    .notes-toggle-label .label-short {
      display: inline;
    }

    .title-with-count {
      gap: 0.35rem;
    }

    .pane-title {
      font-size: 0.92rem;
    }

    .count-pill {
      font-size: 0.62rem;
      padding: 0.1rem 0.4rem;
    }
  }

  /* Ultra-compact mobile breakpoint */
  @media (max-width: 480px) {
    .pane-header-row {
      align-items: center;
    }

    .notes-toggle-pill {
      padding: 0.18rem 0.45rem;
      font-size: 0.68rem;
    }

    .changelog-preview-btn .changelog-more-tag {
      display: none !important;
    }

    .changelog-preview-btn {
      max-width: 100%;
      min-width: 0;
    }

    .changelog-preview-text {
      max-width: 100%;
    }
  }
</style>
