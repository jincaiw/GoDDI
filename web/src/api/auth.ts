import client, { get, post } from './client'

export interface LoginRequest {
  username: string
  password: string
  totp_code?: string
}

export interface LoginResponse {
  token: string
  csrf_token: string
  expires_in: number
  user_id: string
  username: string
  totp_enabled: boolean
}

export interface UserInfo {
  id: string
  username: string
  email: string
  display_name: string
  enabled: boolean
  totp_enabled: boolean
  roles: RoleInfo[]
  groups: GroupInfo[]
  permissions: PermissionInfo[]
}

export interface RoleInfo {
  id: string
  name: string
  description: string
}

export interface GroupInfo {
  id: string
  name: string
  description: string
}

export interface PermissionInfo {
  id: string
  resource: string
  action: string
}

export interface SessionInfo {
  id: string
  user_id: string
  ip: string
  user_agent: string
  created_at: string
  expires_at: string
}

export function login(data: LoginRequest) {
  return post<LoginResponse>('/auth/login', data)
}

export function logout() {
  return post('/auth/logout')
}

export function getCurrentUser() {
  return get<UserInfo>('/auth/me')
}

export function changePassword(data: { old_password: string; new_password: string }) {
  return post('/auth/change-password', data)
}

export function setupTOTP() {
  return post<{ secret: string; qr_url: string }>('/auth/totp/setup')
}

export function verifyAndEnableTOTP(code: string, secret: string) {
  return post('/auth/totp/verify', { code, secret })
}

export function disableTOTP(password: string) {
  return post('/auth/totp/disable', { password })
}

export function listSessions() {
  return get<SessionInfo[]>('/auth/sessions')
}

export function deleteSession(id: string) {
  return client.delete(`/auth/sessions/${id}`)
}
