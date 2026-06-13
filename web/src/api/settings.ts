import { getList, put } from './client'

// The optional `type` field is produced by the backend so the UI can pick the
// right input control (switch / number-input / text) without relying on
// hard-coded key lists that drift out of sync with the server.
export type SettingType = 'bool' | 'int' | 'string'

export interface SystemSetting {
  key: string
  value: string
  description: string
  type?: SettingType
  updated_at: string
}

export function listSystemSettings(params?: Record<string, unknown>) {
  return getList<SystemSetting>('/system/settings', params)
}

export function updateSystemSetting(key: string, value: string) {
  return put(`/system/settings/${key}`, { value })
}
