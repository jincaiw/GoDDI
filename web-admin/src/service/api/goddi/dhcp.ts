import client, { get, getList, post, put, del } from './client'

// --- DHCP Scopes ---

export interface DHCPScope {
  id: string
  name: string
  subnet: string
  start_ip: string
  end_ip: string
  lease_time: number
  enabled: boolean
  active_leases: number
  total_addresses: number
  description: string
  created_at: string
  updated_at: string
}

export interface CreateDHCPScopeRequest {
  name: string
  subnet: string
  start_ip: string
  end_ip: string
  lease_time?: number
  enabled?: boolean
  description?: string
}

export function listDHCPScopes(params?: Record<string, unknown>) {
  return getList<DHCPScope>('/dhcp/scopes', params)
}

export function getDHCPScope(id: string) {
  return get<DHCPScope>(`/dhcp/scopes/${id}`)
}

export function createDHCPScope(data: CreateDHCPScopeRequest) {
  return post<DHCPScope>('/dhcp/scopes', data)
}

export function updateDHCPScope(id: string, data: Partial<CreateDHCPScopeRequest>) {
  return put<DHCPScope>(`/dhcp/scopes/${id}`, data)
}

export function deleteDHCPScope(id: string) {
  return del(`/dhcp/scopes/${id}`)
}

// --- DHCP Leases ---

export interface DHCPLease {
  id: string
  ip: string
  mac: string
  hostname: string
  scope_id: string
  scope_name: string
  status: string
  start_time: string
  end_time: string
  client_id: string
}

export function listDHCPLeases(params?: Record<string, unknown>) {
  return getList<DHCPLease>('/dhcp/leases', params)
}

export function getDHCPLease(id: string) {
  return get<DHCPLease>(`/dhcp/leases/${id}`)
}

export function deleteDHCPLease(id: string) {
  return del(`/dhcp/leases/${id}`)
}

// --- DHCP Reservations ---

export interface DHCPReservation {
  id: string
  ip_address: string
  mac_address: string
  hostname: string
  scope_id: string
  scope_name: string
  enabled: boolean
  description: string
  created_at: string
  updated_at: string
}

export interface CreateDHCPReservationRequest {
  ip_address: string
  mac_address: string
  hostname?: string
  scope_id: string
  enabled?: boolean
  description?: string
}

export function listDHCPReservations(params?: Record<string, unknown>) {
  return getList<DHCPReservation>('/dhcp/reservations', params)
}

export function getDHCPReservation(id: string) {
  return get<DHCPReservation>(`/dhcp/reservations/${id}`)
}

export function createDHCPReservation(data: CreateDHCPReservationRequest) {
  return post<DHCPReservation>('/dhcp/reservations', data)
}

export function updateDHCPReservation(id: string, data: Partial<CreateDHCPReservationRequest>) {
  return put<DHCPReservation>(`/dhcp/reservations/${id}`, data)
}

export function deleteDHCPReservation(id: string) {
  return del(`/dhcp/reservations/${id}`)
}

// --- DHCP Options ---

export interface DHCPOption {
  id: string
  code: number
  value: string
  scope_id: string
  reservation_id: string
  priority: string
  created_at: string
  updated_at: string
}

export interface CreateDHCPOptionRequest {
  code: number
  value: string
  scope_id?: string
  reservation_id?: string
  priority?: string
}

export function listDHCPOptions(params?: Record<string, unknown>) {
  return getList<DHCPOption>('/dhcp/options', params)
}

export function createDHCPOption(data: CreateDHCPOptionRequest) {
  return post<DHCPOption>('/dhcp/options', data)
}

export function updateDHCPOption(id: string, data: Partial<CreateDHCPOptionRequest>) {
  return put<DHCPOption>(`/dhcp/options/${id}`, data)
}

export function deleteDHCPOption(id: string) {
  return del(`/dhcp/options/${id}`)
}

// --- DHCP Logs ---

export interface DHCPLog {
  id: string
  event_type: string
  client_mac: string
  client_ip: string
  hostname: string
  scope_id: string
  scope_name: string
  message: string
  created_at: string
}

export function listDHCPLogs(params?: Record<string, unknown>) {
  return getList<DHCPLog>('/logs/dhcp', params)
}
