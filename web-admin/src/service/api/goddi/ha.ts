import { get } from './client'

/**
 * Read-only DHCP HA status, as answered by GET /api/v1/dhcp/ha.
 *
 * The API is deliberately a snapshot: it never mutates HA state and never
 * exposes the peer token. A node without HA reports ha_enabled=false rather
 * than an error, so a standalone console can render this page too.
 */
export interface DHCPHAStatus {
  ha_enabled: boolean
  node_id?: string
  configured_role?: string
  role_in_force?: string
  role?: string
  state?: string
  redundant?: boolean
  promising?: boolean
  sequence?: number
  acknowledged_sequence?: number
  peer_applied_sequence?: number
  peer_applied_facts_sequence?: number
  applied_sequence?: number
  peer_last_seen?: string
}

export function getHAStatus() {
  return get<DHCPHAStatus>('/dhcp/ha')
}

/**
 * ADR-0013 configuration replication health, as answered by
 * GET /api/v1/cluster/config-replication. A follower with a failed apply or a
 * stale/unknown watermark is healthy=false: the console must warn rather than
 * imply the standby holds the author's current configuration.
 */
export interface ConfigReplicationStatus {
  enabled: boolean
  role: 'disabled' | 'author' | 'follower'
  source?: string
  expected_version: number
  applied_version: number
  applied_rows: number
  last_error: string
  synced_at: string
  healthy: boolean
}

export function getConfigReplicationStatus() {
  return get<ConfigReplicationStatus>('/cluster/config-replication')
}
