/**
 * Goddi auth token storage.
 *
 * SECURITY NOTE: tokens live in sessionStorage (cleared on tab close), same
 * trade-off as the legacy console — reduced XSS persistence in exchange for
 * re-login on browser reopen. See legacy web/src/stores/auth.ts.
 */
const TOKEN_KEY = 'token';
const REFRESH_KEY = 'refresh_token';
const CSRF_KEY = 'csrf_token';

export function getToken(): string {
  return sessionStorage.getItem(TOKEN_KEY) || '';
}

export function getCsrfToken(): string {
  return sessionStorage.getItem(CSRF_KEY) || '';
}

export function setAuthStorage(token: string, csrfToken: string) {
  sessionStorage.setItem(TOKEN_KEY, token);
  if (csrfToken) {
    sessionStorage.setItem(CSRF_KEY, csrfToken);
  }
}

export function clearAuthStorage() {
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(REFRESH_KEY);
  sessionStorage.removeItem(CSRF_KEY);
}
