/**
 * Starter-style central request configuration.
 *
 * The Goddi backend owns its auth/CSRF/refresh semantics in the client
 * interceptors; this module only makes the host/baseURL/timeout policy
 * explicit and reusable, matching the official starter configuration layer.
 */
const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';

export const GODDI_REQUEST_CONFIG = {
  baseURL: isHttpProxy ? '/proxy-default/api/v1' : '/api/v1',
  timeout: 30_000,
  headers: {
    'Content-Type': 'application/json'
  },
} as const;

/** Header used to mark the refresh call so it does not recurse. */
export const SKIP_REFRESH_HEADER = 'X-Skip-Refresh-Interceptor';