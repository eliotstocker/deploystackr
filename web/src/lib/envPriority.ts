export function getEnvRank(envName: string, priorityList?: string[]): number {
    const name = envName.toLowerCase().trim();

    //check explicit priority list if provided
    if(priorityList && priorityList.length > 0) {
        const idx = priorityList.findIndex(p => p.toLowerCase().trim() === name);
        if(idx > -1) {
            return idx;
        }
    }

    const base = 100;

    //pattern matching heuristics
    if(name.includes('prod') || name.includes('live') || name === 'main' || name === 'release') {
        return base + 10;
    }
    if(name.includes('pre') || name.includes('canary') || name.includes('uat')) {
        return base + 20;
    }
    if(name.includes('stag') || name.includes('qa') || name.includes('test')) {
        return base + 30;
    }
    if(name.includes('dev') || name.includes('sandbox') || name.includes('local') || name.includes('preview')) {
        return base + 40;
    }

    return base + 50;
}

export function sortEnvironments<T>(
    envs: Record<string, T> | [string, T][],
    priorityList?: string[]
): [string, T][] {
    const entries: [string, T][] = Array.isArray(envs) ? [...envs] : Object.entries(envs || {});
    return entries.sort(([aName], [bName]) => {
        const rankA = getEnvRank(aName, priorityList);
        const rankB = getEnvRank(bName, priorityList);
        if(rankA !== rankB) {
            return rankA - rankB;
        }
        return aName.localeCompare(bName);
    });
}

export function formatShortVersion(v?: string): string {
    if(!v || v === 'unknown') return 'n/a';
    if(v.includes(':')) {
        const parts = v.split(':');
        return parts[parts.length - 1];
    }
    return v;
}

export function cleanTag(v?: string): string {
    if(!v || v === 'unknown') return '';
    let tag = v.trim();
    if(tag.includes(':')) {
        tag = tag.split(':').pop() || tag;
    }
    if(tag.includes('@')) {
        tag = tag.split('@')[0];
    }
    return tag.replace(/^v/, '');
}

export function areVersionsEqual(v1?: string, v2?: string): boolean {
    if(!v1 || !v2) return false;
    if(v1 === v2) return true;
    const c1 = cleanTag(v1);
    const c2 = cleanTag(v2);
    if(c1 && c1 === c2) return true;
    if(v1.endsWith(':' + v2) || v2.endsWith(':' + v1)) return true;
    return false;
}

export function getTopEnvStatus(svc: {
    environments?: Record<string, any>;
    environment_priority?: string[];
    top_env_status?: any;
    latest_version?: string;
}) {
    if(svc.top_env_status) {
        return svc.top_env_status;
    }
    const envs = sortEnvironments(svc.environments || {}, svc.environment_priority);
    if(envs.length < 1) return null;
    const [envName, envView] = envs[0];
    const active = envView.active_version || 'unknown';
    const latest = svc.latest_version || active;
    const isUp = areVersionsEqual(active, latest);
    return {
        environment: envName,
        active_version: active,
        latest_version: latest,
        versions_behind: isUp ? 0 : 1,
        is_up_to_date: isUp,
        pending_pr: envView.pending_pr,
    };
}
