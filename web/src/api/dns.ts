import client, { get, getList, post, put, del } from './client'

// --- DNS Zones ---

export interface DNSZone {
  id: string
  name: string
  type: string
  enabled: boolean
  dnssec_enabled: boolean
  default_ttl: number
  serial: number
  soa_mname: string
  soa_rname: string
  refresh: number
  retry: number
  expire: number
  minimum: number
  records_count: number
  created_at: string
  updated_at: string
}

export interface CreateDNSZoneRequest {
  name: string
  type: string
  enabled?: boolean
  default_ttl?: number
  soa_mname?: string
  soa_rname?: string
  refresh?: number
  retry?: number
  expire?: number
  minimum?: number
}

export function listDNSZones(params?: Record<string, unknown>) {
  return getList<DNSZone>('/dns/zones', params)
}

export function getDNSZone(id: string) {
  return get<DNSZone>(`/dns/zones/${id}`)
}

export function createDNSZone(data: CreateDNSZoneRequest) {
  return post<DNSZone>('/dns/zones', data)
}

export function updateDNSZone(id: string, data: Partial<CreateDNSZoneRequest>) {
  return put<DNSZone>(`/dns/zones/${id}`, data)
}

export function deleteDNSZone(id: string) {
  return del(`/dns/zones/${id}`)
}

export function importZoneFile(id: string, file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return client.post(`/dns/zones/${id}/import`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function exportZoneFile(id: string) {
  return client.get(`/dns/zones/${id}/export`, { responseType: 'blob' })
}

export function syncSecondaryZone(id: string) {
  return post(`/dns/zones/${id}/sync`)
}

// --- DNSSEC ---

export interface DNSSECStatus {
  enabled: boolean
  keys: DNSSECKey[]
}

export interface DNSSECKey {
  id: string
  algorithm: string
  key_type: string
  created_at: string
  expires_at: string
}

export function getDNSSECStatus(zoneId: string) {
  return get<DNSSECStatus>(`/dns/zones/${zoneId}/dnssec`)
}

export function enableDNSSEC(zoneId: string) {
  return post(`/dns/zones/${zoneId}/dnssec/enable`)
}

export function disableDNSSEC(zoneId: string) {
  return post(`/dns/zones/${zoneId}/dnssec/disable`)
}

export function rotateDNSSECKeys(zoneId: string) {
  return post(`/dns/zones/${zoneId}/dnssec/rotate`)
}

// --- DNS Records ---

export interface DNSRecord {
  id: string
  zone_id: string
  name: string
  type: string
  value: string
  ttl: number
  priority?: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateDNSRecordRequest {
  name: string
  type: string
  value: string
  ttl?: number
  priority?: number
  enabled?: boolean
}

export function listDNSRecords(zoneId: string, params?: Record<string, unknown>) {
  return getList<DNSRecord>(`/dns/zones/${zoneId}/records`, params)
}

export function getDNSRecord(zoneId: string, id: string) {
  return get<DNSRecord>(`/dns/zones/${zoneId}/records/${id}`)
}

export function createDNSRecord(zoneId: string, data: CreateDNSRecordRequest) {
  return post<DNSRecord>(`/dns/zones/${zoneId}/records`, data)
}

export function updateDNSRecord(zoneId: string, id: string, data: Partial<CreateDNSRecordRequest>) {
  return put<DNSRecord>(`/dns/zones/${zoneId}/records/${id}`, data)
}

export function deleteDNSRecord(zoneId: string, id: string) {
  return del(`/dns/zones/${zoneId}/records/${id}`)
}

export function batchCreateRecords(records: CreateDNSRecordRequest[]) {
  return post('/dns/records/batch', { records })
}

export function batchDeleteRecords(ids: string[]) {
  return client.delete('/dns/records/batch', { data: { ids } })
}

// --- DNS Forwarders ---

export interface DNSForwarder {
  id: string
  name: string
  protocol: string
  address: string
  enabled: boolean
  priority: number
}

export function listDNSForwarders(params?: Record<string, unknown>) {
  return getList<DNSForwarder>('/dns/forwarders', params)
}

export function createDNSForwarder(data: Partial<DNSForwarder>) {
  return post<DNSForwarder>('/dns/forwarders', data)
}

export function updateDNSForwarder(id: string, data: Partial<DNSForwarder>) {
  return put<DNSForwarder>(`/dns/forwarders/${id}`, data)
}

export function deleteDNSForwarder(id: string) {
  return del(`/dns/forwarders/${id}`)
}

// --- DNS Conditional Forwarders ---

export interface ConditionalForwarder {
  id: string
  domain: string
  forwarder_ids: string[]
  enabled: boolean
}

export function listConditionalForwarders(params?: Record<string, unknown>) {
  return getList<ConditionalForwarder>('/dns/conditional-forwarders', params)
}

export function createConditionalForwarder(data: Partial<ConditionalForwarder>) {
  return post<ConditionalForwarder>('/dns/conditional-forwarders', data)
}

export function updateConditionalForwarder(id: string, data: Partial<ConditionalForwarder>) {
  return put<ConditionalForwarder>(`/dns/conditional-forwarders/${id}`, data)
}

export function deleteConditionalForwarder(id: string) {
  return del(`/dns/conditional-forwarders/${id}`)
}

// --- DNS Client Policies ---

export interface ClientPolicy {
  id: string
  name: string
  source_cidr: string
  action: string // allow, block, apply_lists
  block_list_ids: string[]
  allow_rule_ids: string[]
  enabled: boolean
  priority: number
}

export function listClientPolicies(params?: Record<string, unknown>) {
  return getList<ClientPolicy>('/dns/security/policies', params)
}

export function createClientPolicy(data: Partial<ClientPolicy>) {
  return post<ClientPolicy>('/dns/security/policies', data)
}

export function updateClientPolicy(id: string, data: Partial<ClientPolicy>) {
  return put<ClientPolicy>(`/dns/security/policies/${id}`, data)
}

export function deleteClientPolicy(id: string) {
  return del(`/dns/security/policies/${id}`)
}

// --- DNS Cache ---

export interface CacheStats {
  entries: number
  max_entries: number
  hit_rate: number
  miss_rate: number
}

export function getDNSCacheStats() {
  return get<CacheStats>('/dns/cache')
}

export function flushDNSCache() {
  return del('/dns/cache')
}

export function flushDNSCacheEntry(name: string, type: string) {
  return del(`/dns/cache/${name}/${type}`)
}

// --- DNS Client (debug query) ---

export interface DNSQueryRequest {
  name: string
  type: string
  upstream?: string
}

export interface DNSQueryResponse {
  answers: DNSQueryAnswer[]
  additional?: DNSQueryAnswer[]
  authority?: DNSQueryAnswer[]
  query_name: string
  query_type: string
  upstream: string
  response_code: string
  duration: number
  error?: string
}

export interface DNSQueryAnswer {
  name: string
  type: string
  data: string
  ttl: number
}

export function executeDNSQuery(data: DNSQueryRequest) {
  return post<DNSQueryResponse>('/dns/client', data)
}

// --- DNS Security - Block Lists ---

export interface BlockList {
  id: string
  name: string
  type: string // custom, external
  url: string
  enabled: boolean
  entry_count: number
  last_fetch_at: string | null
  last_fetch_status: string | null // ok, error
  last_fetch_error: string | null
  created_at: string
  updated_at: string
}

export interface BlockRule {
  id: string
  list_id: string
  pattern: string
  match_type: string // exact, suffix, wildcard, regex
  response_type: string // NXDOMAIN, NODATA, REFUSED, CUSTOM_IP, DROP
  response_data: string
  enabled: boolean
}

export function listBlockLists(params?: Record<string, unknown>) {
  return getList<BlockList>('/dns/security/blocklists', params)
}

export function getBlockList(id: string) {
  return get<BlockList>(`/dns/security/blocklists/${id}`)
}

export function createBlockList(data: Partial<BlockList>) {
  return post<BlockList>('/dns/security/blocklists', data)
}

export function updateBlockList(id: string, data: Partial<BlockList>) {
  return put<BlockList>(`/dns/security/blocklists/${id}`, data)
}

export function deleteBlockList(id: string) {
  return del(`/dns/security/blocklists/${id}`)
}

export function listBlockRules(listId: string, params?: Record<string, unknown>) {
  return getList<BlockRule>(`/dns/security/blocklists/${listId}/rules`, params)
}

export function addBlockRule(listId: string, data: Partial<BlockRule>) {
  return post<BlockRule>(`/dns/security/blocklists/${listId}/rules`, data)
}

export function deleteBlockRule(listId: string, ruleId: string) {
  return del(`/dns/security/blocklists/${listId}/rules/${ruleId}`)
}

// --- DNS Security - Allow Lists ---

export interface AllowRule {
  id: string
  pattern: string
  match_type: string // exact, suffix, wildcard, regex
  enabled: boolean
}

export function listAllowRules(params?: Record<string, unknown>) {
  return getList<AllowRule>('/dns/security/allowlists', params)
}

export function addAllowRule(data: Partial<AllowRule>) {
  return post<AllowRule>('/dns/security/allowlists', data)
}

export function deleteAllowRule(id: string) {
  return del(`/dns/security/allowlists/${id}`)
}

// --- DNS Query Logs ---

export interface DNSQueryLog {
  id: string
  query_name: string
  query_type: string
  client_ip: string
  response_code: string
  response_time: number
  cached: boolean
  blocked: boolean
  upstream: string
  created_at: string
}

export function listDNSQueryLogs(params?: Record<string, unknown>) {
  return getList<DNSQueryLog>('/logs/dns', params)
}

// --- Dashboard Stats ---

export interface HourlyDNSStats {
  hour: string
  queries: number
  blocked: number
  cached: number
}

export interface DashboardStats {
  uptime: number
  dns_queries_today: number
  cache_hit_rate: number
  active_leases: number
  ipam_usage: number
  recent_dns_stats: HourlyDNSStats[]
  system_info: Record<string, unknown>
}

export function getDashboardStats() {
  return get<DashboardStats>('/dashboard')
}

// --- Dashboard Top-N Statistics ---

export interface TopEntry {
  name: string
  count: number
}

export interface TopStats {
  range: string
  top_clients: TopEntry[]
  top_domains: TopEntry[]
  top_blocked: TopEntry[]
}

export function getDashboardTop(range: 'hour' | 'day' | 'week', limit = 10) {
  return get<TopStats>('/dashboard/top', { range, limit })
}

// --- DNS Cache Entries ---

export interface CacheEntry {
  qname: string
  qtype: string
  ttl_left: number
  expires_at: string
  stale_until: string
  hit_count: number
  last_access: string
  size_bytes: number
}

export function listCacheEntries(params?: Record<string, unknown>) {
  return getList<CacheEntry>('/dns/cache/entries', params)
}

// --- DNS Security: Blocking Switch / Temporary Disable ---

export interface BlockingStatus {
  blocking_enabled: boolean
  disabled_until: string | null
}

export function getBlockingStatus() {
  return get<BlockingStatus>('/dns/security/blocking-status')
}

export function temporaryDisableBlocking(minutes: number) {
  return post<BlockingStatus>('/dns/security/temporary-disable', { minutes })
}

export function refreshBlockList(id: string) {
  return post<BlockList>(`/dns/security/blocklists/${id}/refresh`)
}
