import client, { get, getList, post, put, del } from './client'

// --- IPAM Spaces ---

export interface IPAMSpace {
  id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

export interface CreateIPAMSpaceRequest {
  name: string
  description?: string
}

export function listIPAMSpaces(params?: Record<string, unknown>) {
  return getList<IPAMSpace>('/ipam/spaces', params)
}

export function getIPAMSpace(id: string) {
  return get<IPAMSpace>(`/ipam/spaces/${id}`)
}

export function createIPAMSpace(data: CreateIPAMSpaceRequest) {
  return post<IPAMSpace>('/ipam/spaces', data)
}

export function updateIPAMSpace(id: string, data: Partial<CreateIPAMSpaceRequest>) {
  return put<IPAMSpace>(`/ipam/spaces/${id}`, data)
}

export function deleteIPAMSpace(id: string) {
  return del(`/ipam/spaces/${id}`)
}

// --- IPAM Subnets ---

export interface IPAMSubnet {
  id: string
  space_id: string
  name: string
  cidr: string
  description: string
  vlan_id: number
  location: string
  created_at: string
  updated_at: string
}

export interface SubnetStats {
  total: number
  used: number
  available: number
  reserved: number
  dhcp: number
  static_count: number
  gateway: number
  excluded: number
  conflict: number
  unknown: number
}

export interface CreateIPAMSubnetRequest {
  space_id: string
  name: string
  cidr: string
  vlan_id?: number
  location?: string
  description?: string
}

export function listIPAMSubnets(params?: Record<string, unknown>) {
  return getList<IPAMSubnet>('/ipam/subnets', params)
}

export function getIPAMSubnet(id: string) {
  return get<IPAMSubnet>(`/ipam/subnets/${id}`)
}

export function createIPAMSubnet(data: CreateIPAMSubnetRequest) {
  return post<IPAMSubnet>('/ipam/subnets', data)
}

export function updateIPAMSubnet(id: string, data: Partial<CreateIPAMSubnetRequest>) {
  return put<IPAMSubnet>(`/ipam/subnets/${id}`, data)
}

export function deleteIPAMSubnet(id: string) {
  return del(`/ipam/subnets/${id}`)
}

export function getIPAMSubnetStats(id: string) {
  return get<SubnetStats>(`/ipam/subnets/${id}/stats`)
}

export function generateDHCPScope(subnetId: string) {
  return post(`/ipam/subnets/${subnetId}/generate-dhcp-scope`)
}

export function generateReverseZone(subnetId: string) {
  return post(`/ipam/subnets/${subnetId}/generate-reverse-zone`)
}

// --- IPAM Addresses ---

export interface IPAMAddress {
  id: string
  subnet_id: string
  ip_address: string
  status: string
  hostname: string
  mac_address: string
  owner: string
  device: string
  location: string
  description: string
  last_seen: string
  created_at: string
  updated_at: string
}

export interface AllocateIPRequest {
  subnet_id: string
  ip_address?: string
  hostname?: string
  mac_address?: string
  owner?: string
  device?: string
  location?: string
  status?: string
  description?: string
}

export function listIPAMAddresses(params?: Record<string, unknown>) {
  return getList<IPAMAddress>('/ipam/addresses', params)
}

export function getIPAMAddress(id: string) {
  return get<IPAMAddress>(`/ipam/addresses/${id}`)
}

export function updateIPAMAddress(id: string, data: Partial<IPAMAddress>) {
  return put<IPAMAddress>(`/ipam/addresses/${id}`, data)
}

export function allocateIP(data: AllocateIPRequest) {
  return post<IPAMAddress>('/ipam/addresses/allocate', data)
}

export function releaseIP(id: string) {
  return post(`/ipam/addresses/release`, { id })
}

// --- IPAM Import/Export ---

export function importIPAMData(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return client.post('/ipam/import', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function exportIPAMData(params?: Record<string, unknown>) {
  return client.get('/ipam/export', { params, responseType: 'blob' })
}
