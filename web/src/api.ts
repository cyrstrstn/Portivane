import type { AppState, HostnameCheck, Service } from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', 'X-Portivane-Request': '1', ...init?.headers },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(body.error ?? 'The request could not be completed.')
  }
  return response.json() as Promise<T>
}

export const api = {
  authStatus: () => request<{ configured: boolean }>('/api/auth/status'),
  authSetup: (password: string) => request<{ configured: boolean }>('/api/auth/setup', { method: 'POST', body: JSON.stringify({ password }) }),
  authLogin: (password: string) => request<{ authenticated: boolean }>('/api/auth/login', { method: 'POST', body: JSON.stringify({ password }) }),
  state: () => request<AppState>('/api/state'),
  refreshPorts: () => request<AppState>('/api/ports/refresh', { method: 'POST' }),
  login: () => request<{ message: string }>('/api/cloudflare/login', { method: 'POST' }),
  checkHostname: (subdomain: string) => request<HostnameCheck>(`/api/hostnames/check?subdomain=${encodeURIComponent(subdomain)}`),
  saveService: (service: Partial<Service>) => request<AppState>('/api/services', { method: 'POST', body: JSON.stringify(service) }),
  start: (id: string, subdomain: string) => request<AppState>(`/api/services/${encodeURIComponent(id)}/start`, { method: 'POST', body: JSON.stringify({ subdomain }) }),
  stop: (id: string) => request<AppState>(`/api/services/${encodeURIComponent(id)}/stop`, { method: 'POST' }),
  deleteDeployment: (id: string, hostname: string) => request<AppState>(`/api/services/${encodeURIComponent(id)}/deployment`, { method: 'DELETE', body: JSON.stringify({ hostname }) }),
  installCloudflared: () => request<AppState>('/api/cloudflared/install', { method: 'POST' }),
}
