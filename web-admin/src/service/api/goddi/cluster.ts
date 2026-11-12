/**
 * Cluster / HA API — TYPE-ONLY RESERVATION (stage 3, dual-node).
 *
 * SINGLE-NODE-FIRST: this module deliberately contains **no requests**. The
 * backend /api/v1/cluster endpoints return 501 (`notImplementedExtension`),
 * `internal/cluster` coordination is not implemented, and the B8.5 dual-host
 * / power-loss / partition acceptance evidence does not exist yet. Emitting
 * real calls now would put dead HA state into the single-node UI.
 *
 * Stage 3 fills this file with the status/role/epoch/fencing endpoints once the
 * backend contract is frozen and independently reviewed. The shapes below are
 * the *reserved* interface so pages can be written against a stable type later.
 * See docs/tdesign-migration.md §3.
 */

export type NodeMode = 'single' | 'ha-primary' | 'ha-standby' | 'ha-degraded';

export interface ClusterStatus {
  mode: NodeMode;
  /** locally trusted epoch; must never be inferred from a peer's claim */
  epoch: number;
  /** role is independently validated, never taken from a config row alone */
  role: 'primary' | 'standby' | 'fenced';
  /** fencing evidence id; absence means no protected writes are allowed */
  fencingEvidenceId?: string;
  peerReachable: boolean;
  /** last applied replication sequence on this node */
  appliedSeq: number;
  /** the peer's observed sequence, or null when the peer is unreachable */
  peerSeq: number | null;
}

export interface ClusterNode {
  id: string;
  address: string;
  role: 'primary' | 'standby' | 'fenced';
  reachable: boolean;
}

/** Marker so the single-node build can assert no cluster request is wired. */
export const CLUSTER_API_RESERVED = true as const;
