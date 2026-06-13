import client, { getList, post, del } from './client'

export interface Backup {
  id: string
  type: string
  status: string
  file_path?: string
  size_bytes: number
  description?: string
  error?: string
  started_at?: string
  completed_at?: string
  created_at: string
}

export function listBackups(params?: Record<string, unknown>) {
  return getList<Backup>('/system/backups', params)
}

// createBackup now requires a human-readable description so the produced
// backup file can be identified later from the listing UI / CLI.
export function createBackup(description: string, type?: string) {
  return post<Backup>('/system/backups', { description, type: type ?? 'full' })
}

export function restoreBackup(id: string) {
  return post(`/system/backups/${id}/restore`)
}

export function deleteBackup(id: string) {
  return del(`/system/backups/${id}`)
}

export async function downloadBackup(id: string) {
  const response = await client.get(`/system/backups/${id}/download`, { responseType: 'blob' })
  const disposition = response.headers['content-disposition'] as string | undefined
  const encodedName = disposition?.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  const fileName = encodedName ? decodeURIComponent(encodedName) : `goddi-backup-${id}.json`
  return { blob: response.data as Blob, fileName }
}
