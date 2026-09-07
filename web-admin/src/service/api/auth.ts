import { login as goddiLogin, logout as goddiLogout, getCurrentUser } from './goddi/auth';
import type { LoginResponse, UserInfo } from './goddi/auth';

/**
 * Flat request result shape consumed by the auth store.
 * Goddi's axios client rejects with a plain Error (with numeric `code`
 * attached for auth errors), so we normalize to { data, error } here.
 */
type Flat<T> = { data: T; error: null } | { data: null; error: Error };

function toFlat<T>(fn: () => Promise<T>): Promise<Flat<T>> {
  return fn()
    .then(data => ({ data, error: null }) as Flat<T>)
    .catch((err: unknown) => ({ data: null, error: (err instanceof Error ? err : new Error(String(err))) }) as Flat<T>);
}

/** Login with username/password (+ optional TOTP code). */
export function fetchLogin(userName: string, password: string, totpCode?: string) {
  return toFlat(() => goddiLogin({ username: userName, password, totp_code: totpCode || undefined })) as Promise<
    Flat<LoginResponse>
  >;
}

/** Get current user info from /auth/me. */
export function fetchGetUserInfo() {
  return toFlat(getCurrentUser) as Promise<Flat<UserInfo>>;
}

/** Server-side logout. Errors are swallowed by the store. */
export function fetchLogout() {
  return toFlat(goddiLogout);
}

/**
 * Template-compat token refresh (used by service/request/shared.ts).
 * Goddi refreshes via the bearer-authenticated /auth/refresh endpoint; the
 * Goddi flow itself is handled inside service/api/goddi/client.ts, so this
 * wrapper exists only to keep the template's request layer compiling.
 */
export async function fetchRefreshToken(_refreshToken: string) {
  return toFlat(async () => {
    const { post } = await import('./goddi/client');
    const result = await post<{ token: string; csrf_token: string; expires_in: number }>('/auth/refresh', {});
    return { token: result.token, refreshToken: '' };
  });
}
