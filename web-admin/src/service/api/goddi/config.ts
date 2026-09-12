import { get, getList, post } from './client'

// Configuration publishing (配置版本).
//
// These endpoints govern the publishing of an existing resource's
// configuration: a change is recorded as a numbered, append-only revision and
// a data-plane release is queued for it. The console page is read-mostly --
// it lists history, renders diffs and lets an operator roll back -- so the
// shapes here mirror internal/configver exactly rather than reshaping it.

/** A resource class the server can publish. */
export type ConfigResourceType = 'dns_zone' | 'dhcp_scope' | 'ipam_subnet' | 'dns_records'

/**
 * The furthest stage a revision reached.
 *
 * `failed` means the release exhausted its retries: the configuration is
 * recorded but the data plane never confirmed it, so the last `applied`
 * revision is still what is being served. It is the one state an operator
 * must not miss.
 */
export type ConfigRevisionStatus = 'persisted' | 'staged' | 'applied' | 'failed'

export interface ConfigRevision {
  id: string
  resource_type: ConfigResourceType
  resource_id: string
  revision: number
  status: ConfigRevisionStatus
  /** Full content of this revision; the shape depends on resource_type. */
  content: unknown
  content_hash: string
  base_revision: number
  idempotency_key?: string
  actor?: string
  note?: string
  error?: string
  applied_generation: number
  created_at: string
  updated_at: string
  applied_at?: string
}

/** One field-level difference between two revisions. */
export interface ConfigFieldChange {
  field: string
  old: unknown
  new: unknown
}

/** One queued data-plane release. */
export interface ConfigRelease {
  id: number
  revision_id: string
  resource_type: string
  resource_id: string
  revision: number
  status: string
  attempts: number
  resource_applied: boolean
  applied_generation: number
  last_error?: string
  created_at: string
  applied_at?: string
}

export interface ConfigReleaseQueue {
  releases: ConfigRelease[]
  stats: { pending: number; failed: number }
}

export interface ConfigPublishResult {
  /** Absent for a dry run, which stores nothing. */
  revision?: ConfigRevision
  changes: ConfigFieldChange[]
  base_revision: number
  replayed: boolean
  dry_run: boolean
}

export interface ConfigPublishRequest {
  resource_type: string
  resource_id: string
  content: unknown
  expected_revision: number
  idempotency_key?: string
  note?: string
  dry_run?: boolean
}

export interface ConfigRollbackRequest {
  resource_type: string
  resource_id: string
  to_revision: number
  expected_revision: number
  idempotency_key?: string
  note?: string
}

/** Resource types this instance can publish. */
export function listConfigTypes() {
  return get<{ resource_types: ConfigResourceType[] }>('/config/types')
}

/** Revisions newest first, filterable by resource type and id. */
export function listConfigRevisions(params?: Record<string, unknown>) {
  return getList<ConfigRevision>('/config/revisions', params)
}

export function getConfigRevision(id: string, withChanges = false) {
  return get<{ revision: ConfigRevision; changes?: ConfigFieldChange[] }>(`/config/revisions/${id}`, {
    with_changes: withChanges ? 'true' : undefined,
  })
}

/** Field-level diff between two revision numbers of one resource. */
export function diffConfigRevisions(params: {
  resource_type: string
  resource_id: string
  from: number
  to: number
}) {
  return get<{ from: number; to: number; changes: ConfigFieldChange[] }>('/config/diff', params)
}

export function publishConfig(req: ConfigPublishRequest) {
  return post<ConfigPublishResult>('/config/publish', req)
}

export function rollbackConfig(req: ConfigRollbackRequest) {
  return post<ConfigPublishResult>('/config/rollback', req)
}

/** The release queue plus pending/failed counts. */
export function listConfigReleases(params?: Record<string, unknown>) {
  return get<ConfigReleaseQueue>('/config/releases', params)
}

/** Reset failed releases (optionally narrowed to one resource) so the worker retries them. */
export function retryConfigReleases(params?: { resource_type?: string; resource_id?: string }) {
  return post<{ reset: number }>('/config/releases/retry', params ?? {})
}
