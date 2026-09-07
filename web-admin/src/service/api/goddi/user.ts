import { getList, get, post, put, del } from './client'

// --- Users ---

export interface User {
  id: string
  username: string
  email: string
  display_name: string
  enabled: boolean
  totp_enabled: boolean
  roles: { id: string; name: string }[]
  groups: { id: string; name: string }[]
  created_at: string
  updated_at: string
}

export interface CreateUserRequest {
  username: string
  email: string
  password: string
  display_name?: string
  enabled?: boolean
}

export function listUsers(params?: Record<string, unknown>) {
  return getList<User>('/users', params)
}

export function getUser(id: string) {
  return get<User>(`/users/${id}`)
}

export function createUser(data: CreateUserRequest) {
  return post<User>('/users', data)
}

export function updateUser(id: string, data: Partial<CreateUserRequest>) {
  return put<User>(`/users/${id}`, data)
}

export function deleteUser(id: string) {
  return del(`/users/${id}`)
}

export function assignUserRoles(userId: string, roleIds: string[]) {
  return post(`/users/${userId}/roles`, { role_ids: roleIds })
}

export function removeUserRole(userId: string, roleId: string) {
  return del(`/users/${userId}/roles/${roleId}`)
}

export function assignUserGroup(userId: string, groupId: string) {
  return post(`/users/${userId}/groups`, { group_id: groupId })
}

export function removeUserGroup(userId: string, groupId: string) {
  return del(`/users/${userId}/groups/${groupId}`)
}

// --- Roles ---

export interface Role {
  id: string
  name: string
  description: string
  is_system: boolean
  permissions: { id: string; resource: string; action: string }[]
  created_at: string
  updated_at: string
}

export interface CreateRoleRequest {
  name: string
  description?: string
}

export function listRoles(params?: Record<string, unknown>) {
  return getList<Role>('/roles', params)
}

export function getRole(id: string) {
  return get<Role>(`/roles/${id}`)
}

export function createRole(data: CreateRoleRequest) {
  return post<Role>('/roles', data)
}

export function updateRole(id: string, data: Partial<CreateRoleRequest>) {
  return put<Role>(`/roles/${id}`, data)
}

export function deleteRole(id: string) {
  return del(`/roles/${id}`)
}

export function assignRolePermissions(roleId: string, permissionIds: string[]) {
  return post(`/roles/${roleId}/permissions`, { permission_ids: permissionIds })
}

export function removeRolePermission(roleId: string, permissionId: string) {
  return del(`/roles/${roleId}/permissions/${permissionId}`)
}

// --- Groups ---

export interface Group {
  id: string
  name: string
  description: string
  roles: { id: string; name: string }[]
  created_at: string
  updated_at: string
}

export interface CreateGroupRequest {
  name: string
  description?: string
}

export function listGroups(params?: Record<string, unknown>) {
  return getList<Group>('/groups', params)
}

export function getGroup(id: string) {
  return get<Group>(`/groups/${id}`)
}

export function createGroup(data: CreateGroupRequest) {
  return post<Group>('/groups', data)
}

export function updateGroup(id: string, data: Partial<CreateGroupRequest>) {
  return put<Group>(`/groups/${id}`, data)
}

export function deleteGroup(id: string) {
  return del(`/groups/${id}`)
}

export function assignGroupRoles(groupId: string, roleIds: string[]) {
  return post(`/groups/${groupId}/roles`, { role_ids: roleIds })
}

export function removeGroupRole(groupId: string, roleId: string) {
  return del(`/groups/${groupId}/roles/${roleId}`)
}

// --- Permissions ---

export interface Permission {
  id: string
  resource: string
  action: string
  description: string
}

export function listPermissions(params?: Record<string, unknown>) {
  return getList<Permission>('/permissions', params)
}

// --- API Tokens ---

export interface APIToken {
  id: string
  name: string
  description: string
  user_id: string
  username: string
  expires_at: string
  last_used_at: string
  created_at: string
}

export interface CreateAPITokenRequest {
  name: string
  description?: string
  expires_at?: string
}

export interface CreateAPITokenResponse {
  id: string
  token: string
}

export function listAPITokens(params?: Record<string, unknown>) {
  return getList<APIToken>('/tokens', params)
}

export function createAPIToken(data: CreateAPITokenRequest) {
  return post<CreateAPITokenResponse>('/tokens', data)
}

export function deleteAPIToken(id: string) {
  return del(`/tokens/${id}`)
}
