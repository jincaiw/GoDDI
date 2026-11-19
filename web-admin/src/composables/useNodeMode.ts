import { readonly, ref } from 'vue';
import { getHAStatus, type DHCPHAStatus } from '@/service/api/goddi/ha';

/**
 * Deployment-mode state shared by the header badge and the dashboard tag.
 *
 * The console used to hardcode 'single'. In an HA pair that is not just a
 * cosmetic lie: an operator reading "Single-Node" on a standby cannot tell
 * which node may author configuration. The mode is fetched from the same
 * read-only /api/v1/dhcp/ha facts the HA page shows, refreshed periodically.
 *
 * FAIL-CLOSED: until the first successful read, and on any read error, the
 * mode is 'ha-degraded' (unknown), never 'single'. An unknown two-node state
 * must not be presented as a healthy standalone deployment.
 */
export type NodeMode = 'single' | 'ha-primary' | 'ha-standby' | 'ha-degraded';

const mode = ref<NodeMode>('ha-degraded');
let started = false;
let inFlight: Promise<void> | null = null;

/** Pure mapping, exported so the state contract can be reviewed and tested. */
export function mapHAStatusToNodeMode(s: DHCPHAStatus): NodeMode {
  if (!s.ha_enabled) return 'single';
  const role = s.role_in_force ?? s.role ?? '';
  const state = s.state ?? '';
  // fenced / paused / operator-degraded / unknown are never a healthy role.
  if (state === 'fenced' || state === 'paused' || state === 'primary-degraded' || state === '') {
    return 'ha-degraded';
  }
  if (role === 'primary') return 'ha-primary';
  if (role === 'standby') return 'ha-standby';
  return 'ha-degraded';
}

export function useNodeMode() {
  async function refresh() {
    if (inFlight) return inFlight;
    inFlight = (async () => {
      try {
        mode.value = mapHAStatusToNodeMode(await getHAStatus());
      } catch {
        mode.value = 'ha-degraded';
      } finally {
        inFlight = null;
      }
    })();
    return inFlight;
  }
  if (!started) {
    started = true;
    void refresh();
    if (typeof window !== 'undefined') {
      window.setInterval(() => void refresh(), 15000);
    }
  }
  return { mode: readonly(mode), refresh };
}
