import type { Category, Job } from './types'

export interface AuthUser {
  id: number
  username: string
}

let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(fn: (() => void) | null) {
  onUnauthorized = fn
}

async function request<T>(url: string, init?: RequestInit & { skipAuthRedirect?: boolean }): Promise<T> {
  const { skipAuthRedirect, ...rest } = init || {}
  const res = await fetch(url, { ...rest, credentials: 'include' })
  const data = await res.json().catch(() => ({}))
  if (res.status === 401 && !skipAuthRedirect) {
    onUnauthorized?.()
  }
  if (!res.ok) {
    throw new Error((data as { error?: string }).error || res.statusText)
  }
  return data as T
}

export function login(username: string, password: string) {
  return request<AuthUser>('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
    skipAuthRedirect: true,
  })
}

export function logout() {
  return request<{ ok: boolean }>('/api/logout', { method: 'POST', skipAuthRedirect: true })
}

export function getMe() {
  return request<AuthUser>('/api/me', { skipAuthRedirect: true })
}

export function listJobs() {
  return request<Job[]>('/api/jobs')
}

export function listCategories() {
  return request<Category[]>('/api/categories')
}

export function getJob(id: string) {
  return request<Job>(`/api/jobs/${id}`)
}

export function createJob(files: File[], category: string, skipVL: boolean) {
  const body = new FormData()
  body.append('category', category)
  body.append('skip_vl', skipVL ? '1' : '0')
  for (const file of files) {
    body.append('files', file)
  }
  return request<Job>('/api/jobs', { method: 'POST', body })
}

export function deleteJob(id: string) {
  return request<{ ok: boolean }>(`/api/jobs/${id}`, { method: 'DELETE' })
}

export function updateRecords(id: string, records: Record<string, unknown>[]) {
  return request<Job>(`/api/jobs/${id}/records`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ records }),
  })
}

export function fileUrl(jobId: string, storedName: string) {
  return `/api/jobs/${jobId}/files/${encodeURIComponent(storedName)}`
}

export function exportUrl(jobId: string) {
  return `/api/jobs/${jobId}/export`
}
