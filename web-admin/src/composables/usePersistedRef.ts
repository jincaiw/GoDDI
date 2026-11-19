import { ref, watch } from 'vue';
import type { Ref } from 'vue';

/**
 * A `ref` whose value survives a route change by mirroring into sessionStorage.
 *
 * Why not KeepAlive: this project's router registers views as lazy wrapper
 * objects whose `name` is the route name, so `<KeepAlive :include>` cannot
 * match the SFC `defineOptions` name (verified in task 35). Persisting the
 * few filter values explicitly is simpler, testable and decoupled from the
 * router.
 *
 * @param key sessionStorage key
 * @param initial value used when nothing is stored
 */
export function usePersistedRef<T>(key: string, initial: T): Ref<T> {
  const storageKey = `goddi:filter:${key}`;

  let stored: T = initial;
  try {
    const raw = sessionStorage.getItem(storageKey);
    if (raw !== null) stored = JSON.parse(raw) as T;
  } catch {
    stored = initial;
  }

  const state = ref(stored) as Ref<T>;

  watch(
    state,
    value => {
      try {
        sessionStorage.setItem(storageKey, JSON.stringify(value));
      } catch {
        // Storage can be unavailable (private mode/quota); filtering still works.
      }
    },
    { deep: true }
  );

  return state;
}