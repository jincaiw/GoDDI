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

/**
 * Where a subnet's addresses would be published in reverse DNS.
 *
 * `zone_name` is the most specific delegation -- a /24 belongs in its own /24
 * zone even though a /16 would also cover it. `candidates` carries the less
 * specific alternatives, most specific first, because the choice is the
 * operator's: a site that delegates 192.0.0.0/16 publishes the /16.
 *
 * This describes a computation, not a write. The zone is created by
 * `createDNSZone`, which needs a different permission -- which is why the
 * console shows this and then confirms, rather than reporting success here.
 */
export interface ReverseZonePlan {
  subnet_id: string
  cidr: string
  zone_name: string
  candidates: string[]
}

export function planReverseZone(subnetId: string) {
  return post<ReverseZonePlan>(`/ipam/subnets/${subnetId}/generate-reverse-zone`)
}

// --- Building a DHCP scope from a subnet ---

/** One address inside the proposed range that IPAM has fenced off. */
export interface PlanAddress {
  ip: string
  status: string
  hostname?: string
  owner?: string
}

/**
 * What IPAM says about the scope that would be created from a subnet.
 *
 * `fingerprint` identifies the state the plan was derived from. It is sent back
 * with the create; a mismatch means the world moved in between (an address was
 * allocated, released or fenced) and the plan has to be looked at again. The
 * check exists because the two steps are a conversation with a person in the
 * middle of it, and the data underneath does not wait.
 */
export interface DHCPScopePlan {
  subnet_id: string
  subnet_name: string
  space_id?: string
  cidr: string
  start_ip: string
  end_ip: string
  router?: string
  /** Where the router came from: named in the subnet, guessed, or nothing. */
  router_source?: string
  /** Addresses in the range IPAM has fenced off. A DHCP scope has no
   *  exclusion list, so this list is the operator's, not the allocator's. */
  excluded: PlanAddress[]
  /** Reasons the scope should not be created as proposed. */
  conflicts: string[]
  /** Things the operator must know and may accept. */
  warnings: string[]
  fingerprint: string
}

/**
 * The scope the backend would create, filled in with its defaults.
 *
 * It is a `ScopeOptions` with two differences from the create request: the
 * pointers are optional and `router` has been filled in from the plan, which is
 * the one field the draft generator never set -- which is how a scope ends up
 * handing clients no default route.
 */
export interface ScopeDraft {
  name?: string
  subnet?: string
  start_ip?: string
  end_ip?: string
  subnet_mask?: string
  router?: string
  dns_servers?: string
  ntp_servers?: string
  domain_name?: string
  lease_time?: number
  max_lease_time?: number
  enabled?: boolean
  ping_check_enabled?: boolean
  dns_updates?: boolean
  comment?: string
}

export function getIPAMPoolPlan(subnetId: string) {
  return post<{ plan: DHCPScopePlan; draft: ScopeDraft }>(`/ipam/subnets/${subnetId}/dhcp-scope-plan`)
}

// --- IPAM Addresses ---

export interface IPAMAddress {
  id: string
  subnet_id: string
  space_id: string
  ip_address: string
  status: string
  hostname: string
  mac_address: string
  owner: string
  device: string
  location: string
  description: string
  dhcp_lease_id?: string
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

// --- IPAM Address 360° view ---
//
// One address is claimed by four subsystems at once, and each of them can be
// wrong about it. The view answers all four in one request: which DNS records
// publish it, which DHCP scopes cover it (and whether it falls in their pool,
// in their subnet, or is the gateway they hand out), which leases and
// reservations name it, how it got here, and where the four disagree.

/** A DNS record that points at this address. Read from `dns_records`. */
export interface PublishingRecord {
  id: string
  zone_id?: string
  name: string
  type: string
  value: string
  ttl: number
  enabled: boolean
  comment?: string
  owner_ref?: string
  /**
   * False for a record an operator configured, true for one a data plane
   * wrote. A locally authored row reaches the control database by a push that
   * lags, so a name listed here may already be withdrawn in the plane that
   * answers -- which is worth saying in the UI rather than hiding.
   */
  authored_locally: boolean
}

/** A DHCP scope that covers this address, and how. */
export interface ScopeSummary {
  id: string
  name: string
  subnet: string
  start_ip: string
  end_ip: string
  router?: string
  enabled: boolean
  /** The address falls inside the scope's declared subnet. */
  in_subnet: boolean
  /** The address falls inside the range the scope actually hands out. */
  in_pool: boolean
  /** The address is the gateway this scope tells clients to use. */
  is_gateway: boolean
}

export interface LeaseSummary {
  id: string
  scope_id: string
  mac_address: string
  hostname?: string
  status: string
  generation: number
  lease_start: string
  lease_end: string
}

export interface ReservationSummary {
  id: string
  scope_id: string
  mac_address: string
  hostname?: string
  enabled: boolean
}

export interface AddressHistoryEntry {
  action: string
  old_status?: string
  new_status?: string
  changed_by?: string
  reason?: string
  source?: string
  created_at: string
}

export interface AddressView {
  address: IPAMAddress
  subnet?: IPAMSubnet
  space?: IPAMSpace
  dns_records: PublishingRecord[]
  dhcp_scopes: ScopeSummary[]
  dhcp_leases: LeaseSummary[]
  dhcp_reservations: ReservationSummary[]
  history: AddressHistoryEntry[]
  /**
   * `dhcp_scopes` is a partial list because the scope table is larger than the
   * view reads in one request. An absence from `dhcp_scopes` must not be shown
   * as "no scope claims this address" unless this is false.
   */
  scopes_truncated: boolean
  /** Places where the subsystems disagree. Computed, so it cannot go stale. */
  conflicts: string[]
}

export function getIPAMAddressView(spaceId: string, ip: string) {
  return get<AddressView>('/ipam/addresses/view', { space_id: spaceId, ip })
}

// --- IPAM Import/Export ---

/**
 * The import body.
 *
 * `data` is the file's text, not an upload: the backend reads a JSON body and
 * bounds it at 5MB. `parent_id` is the subnet for `type: 'addresses'`.
 */
export interface ImportRequest {
  type: string
  format: string
  parent_id: string
  data: string
}

/** One row's outcome: what it was, what it becomes, and why. */
export interface ImportPlan {
  line: number
  ip: string
  action: string
  previous_status?: string
  status: string
  changed_fields?: string[]
}

/**
 * The outcome of an import, or of a preview of one. Both endpoints answer with
 * this shape: a preview whose report differed from the import's would be a
 * document about a different operation.
 */
export interface ImportReport {
  type: string
  parent_id: string
  total: number
  creates: number
  updates: number
  unchanged: number
  /** Rows that stop the import, each naming its line. */
  errors: string[]
  changes: ImportPlan[]
  /** `changes` is a sample; the counts above are always exact. */
  truncated: boolean
  /** False for a preview. */
  applied: boolean
}

/**
 * Preview an import without writing anything.
 *
 * A rejected preview answers 400 with the report attached to the error's
 * `payload`, not only with a message: the per-line reasons are the point.
 */
export function previewIPAMImport(req: ImportRequest) {
  return post<ImportReport>('/ipam/import/preview', req)
}

/**
 * Apply an import.
 *
 * Like the preview, a rejected import answers 400 with the report on
 * `error.payload`.
 */
export function importIPAMData(req: ImportRequest) {
  return post<ImportReport>('/ipam/import', req)
}

export function exportIPAMData(params?: Record<string, unknown>) {
  return client.get('/ipam/export', { params, responseType: 'blob' })
}
