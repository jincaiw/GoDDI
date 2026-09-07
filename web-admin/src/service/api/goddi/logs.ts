import { getList } from './client'

export interface AuditLog {
  id: string
  user_id: string
  username: string
  action: string
  resource: string
  resource_id: string
  details: string
  ip: string
  created_at: string
}

export interface LoginHistory {
  id: string
  user_id: string
  username: string
  ip: string
  user_agent: string
  success: boolean
  created_at: string
}

export function listAuditLogs(params?: Record<string, unknown>) {
  return getList<AuditLog>('/logs/audit', params)
}

export function listLoginHistory(params?: Record<string, unknown>) {
  return getList<LoginHistory>('/logs/login', params)
}
