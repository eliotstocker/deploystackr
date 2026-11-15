import type {
  ServiceResponse,
  VersionInfo,
  DeploymentEvent,
  ChangelogDiff,
  PromotionRequest,
  PromotionResult,
  AuthMeResponse,
  AuthProvidersResponse,
  ClusterStatus
} from '../types';
import {
  demoFetchAuthMe,
  demoFetchAuthProviders,
  demoLoginDev,
  demoLogoutUser,
  demoFetchServices,
  demoFetchVersions,
  demoFetchHistory,
  demoFetchDiff,
  demoPromote,
  demoTriggerSync,
  demoFetchServiceStatus,
  demoFetchReleasePRs
} from './demoBackend';

const IS_DEMO = import.meta.env.VITE_DEMO_MODE === 'true';

export const API_BASE =
  import.meta.env.VITE_API_BASE ??
  (typeof window !== 'undefined' && window.location.port === '5173'
    ? 'http://localhost:8080/api'
    : '/api');

export async function fetchAuthMe(): Promise<AuthMeResponse> {
  if(IS_DEMO) return demoFetchAuthMe();
  const res = await fetch(`${API_BASE}/auth/me`, { credentials: 'include' });
  if (!res.ok) throw new Error(`Failed to check auth status: ${res.statusText}`);
  return res.json();
}

export async function fetchAuthProviders(): Promise<AuthProvidersResponse> {
  if(IS_DEMO) return demoFetchAuthProviders();
  const res = await fetch(`${API_BASE}/auth/providers`, { credentials: 'include' });
  if (!res.ok) throw new Error(`Failed to fetch auth providers: ${res.statusText}`);
  return res.json();
}

export async function loginDev(personaId: string): Promise<AuthMeResponse> {
  if(IS_DEMO) return demoLoginDev(personaId);
  const res = await fetch(
    `${API_BASE}/auth/login/dev?persona=${encodeURIComponent(personaId)}&format=json`,
    {
      credentials: 'include',
      headers: { Accept: 'application/json' }
    }
  );
  if (!res.ok) throw new Error(`Dev login failed: ${res.statusText}`);
  return res.json();
}

export async function logoutUser(): Promise<void> {
  if(IS_DEMO) return demoLogoutUser();
  const res = await fetch(`${API_BASE}/auth/logout`, {
    method: 'POST',
    credentials: 'include'
  });
  if (!res.ok) throw new Error(`Logout failed: ${res.statusText}`);
}

export async function fetchServices(): Promise<ServiceResponse[]> {
  if(IS_DEMO) return demoFetchServices();
  const res = await fetch(`${API_BASE}/services`, { credentials: 'include' });
  if (!res.ok) throw new Error(`Failed to fetch services: ${res.statusText}`);
  return res.json();
}

export async function fetchVersions(service: string): Promise<VersionInfo[]> {
  if(IS_DEMO) return demoFetchVersions(service);
  const res = await fetch(`${API_BASE}/services/${encodeURIComponent(service)}/versions`, { credentials: 'include' });
  if (!res.ok) throw new Error(`Failed to fetch versions for ${service}: ${res.statusText}`);
  return res.json();
}

export async function fetchHistory(service: string): Promise<Record<string, DeploymentEvent[]>> {
  if(IS_DEMO) return demoFetchHistory(service);
  const res = await fetch(`${API_BASE}/services/${encodeURIComponent(service)}/history`, { credentials: 'include' });
  if (!res.ok) throw new Error(`Failed to fetch history for ${service}: ${res.statusText}`);
  return res.json();
}

export async function fetchDiff(
  service: string,
  fromVersion?: string,
  toVersion?: string
): Promise<ChangelogDiff> {
  if(IS_DEMO) return demoFetchDiff(service, fromVersion, toVersion);
  const params = new URLSearchParams();
  if (fromVersion) params.set('from', fromVersion);
  if (toVersion) params.set('to', toVersion);
  const res = await fetch(
    `${API_BASE}/services/${encodeURIComponent(service)}/diff?${params.toString()}`,
    { credentials: 'include' }
  );
  if (!res.ok) throw new Error(`Failed to fetch diff: ${res.statusText}`);
  return res.json();
}

export async function promote(payload: PromotionRequest): Promise<PromotionResult> {
  if(IS_DEMO) return demoPromote(payload);
  const res = await fetch(`${API_BASE}/promote`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify(payload)
  });
  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.error || 'Promotion failed');
  }
  return data;
}

export async function triggerSync(): Promise<{ status: string; message: string }> {
  if(IS_DEMO) return demoTriggerSync();
  const res = await fetch(`${API_BASE}/webhook`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ event: 'manual_sync' })
  });
  if (!res.ok) throw new Error('Failed to trigger sync');
  return res.json();
}

export async function fetchServiceStatus(service: string, env?: string): Promise<Record<string, ClusterStatus>> {
    if(IS_DEMO) return demoFetchServiceStatus(service, env);
    const url = `${API_BASE}/services/${encodeURIComponent(service)}/status${env ? `?env=${encodeURIComponent(env)}` : ''}`;
    const res = await fetch(url, {credentials: 'include'});
    if(!res.ok) {
        throw new Error(`Failed to fetch status for ${service}: ${res.statusText}`);
    }
    return res.json();
}

export async function fetchReleasePRs(service: string): Promise<Record<string, number[]>> {
    if(IS_DEMO) return demoFetchReleasePRs(service);
    const res = await fetch(`${API_BASE}/services/${encodeURIComponent(service)}/prs`, {credentials: 'include'});
    if(!res.ok) return {};
    return res.json();
}
