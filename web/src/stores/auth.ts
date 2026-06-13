import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login as apiLogin, logout as apiLogout, getCurrentUser, type UserInfo, type LoginRequest } from '@/api/auth'

// SECURITY NOTE: tokens are stored in sessionStorage (cleared on tab close)
// rather than localStorage. This is a deliberate trade-off:
//   - sessionStorage reduces the attack surface from XSS persistence and
//     accidental leakage to other tabs of the same origin.
//   - The downside is that users must re-authenticate when they reopen
//     the browser, which is acceptable for an internal admin console.
//   - Cookies were considered but would require additional CSRF protection.
// If persistent logins across browser sessions become a requirement, switch
// the storage backend here and add a corresponding audit log entry.
export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(sessionStorage.getItem('token') || '')
  const refreshToken = ref<string>(sessionStorage.getItem('refresh_token') || '')
  const csrfToken = ref<string>(sessionStorage.getItem('csrf_token') || '')
  const user = ref<UserInfo | null>(null)
  const loading = ref(false)

  function isTokenExpired(t: string): boolean {
    if (!t) return true
    try {
      const payload = t.split('.')[1]
      if (!payload) return true
      const decoded = JSON.parse(atob(payload))
      if (!decoded.exp) return false
      return decoded.exp * 1000 < Date.now()
    } catch {
      return true
    }
  }

  const isAuthenticated = computed(() => !!token.value && !isTokenExpired(token.value))
  const permissions = computed(() => {
    if (!user.value) return []
    return user.value.permissions || []
  })

  function setToken(newToken: string, newRefreshToken: string, newCsrfToken?: string) {
    token.value = newToken
    refreshToken.value = newRefreshToken
    if (newCsrfToken) {
      csrfToken.value = newCsrfToken
      sessionStorage.setItem('csrf_token', newCsrfToken)
    }
    sessionStorage.setItem('token', newToken)
    sessionStorage.setItem('refresh_token', newRefreshToken)
  }

  function clearToken() {
    token.value = ''
    refreshToken.value = ''
    csrfToken.value = ''
    user.value = null
    sessionStorage.removeItem('token')
    sessionStorage.removeItem('refresh_token')
    sessionStorage.removeItem('csrf_token')
  }

  async function login(credentials: LoginRequest) {
    loading.value = true
    try {
      const result = await apiLogin(credentials)
      setToken(result.token, '', result.csrf_token)
      // Fetch full user info after login
      try {
        user.value = await getCurrentUser()
      } catch {
        // User info fetch failed, but login was successful
      }
      return result
    } finally {
      loading.value = false
    }
  }

  async function logout() {
    try {
      await apiLogout()
    } catch {
      // Ignore logout API errors
    } finally {
      clearToken()
    }
  }

  async function fetchUser() {
    if (!token.value) return
    try {
      const result = await getCurrentUser()
      user.value = result
    } catch (err: unknown) {
      // Only clear token on 401/auth errors, not on network errors.
      // The API client attaches the numeric `code` from the server response
      // to the rejected Error, so check that field directly.
      const isAuthError =
        err instanceof Error && 'code' in err && (err as Error & { code?: number }).code === 401
      if (isAuthError) {
        clearToken()
      }
      throw err
    }
  }

  function hasPermission(resource: string, action: string): boolean {
    if (!user.value) return false
    return permissions.value.some(
      (p) => p.resource === resource && p.action === action
    )
  }

  function hasAnyPermission(checks: Array<{ resource: string; action: string }>): boolean {
    return checks.some(({ resource, action }) => hasPermission(resource, action))
  }

  return {
    token,
    refreshToken,
    csrfToken,
    user,
    loading,
    isAuthenticated,
    permissions,
    login,
    logout,
    fetchUser,
    hasPermission,
    hasAnyPermission,
    setToken,
    clearToken,
  }
})
