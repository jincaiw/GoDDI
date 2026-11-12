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
