import type { Category, Job } from './types'

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error((data as { error?: string }).error || res.statusText)
  }
  return data as T
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

export function fileUrl(jobId: string, storedName: string) {
  return `/api/jobs/${jobId}/files/${encodeURIComponent(storedName)}`
}

export function exportUrl(jobId: string) {
  return `/api/jobs/${jobId}/export`
}
