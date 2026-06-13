import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

export function setupRouterGuards(router: Router) {
  router.beforeEach(async (to, _from, next) => {
    const authStore = useAuthStore()

    // Check if route requires authentication
    const requiresAuth = to.meta.requiresAuth !== false

    if (requiresAuth && !authStore.isAuthenticated) {
      next({ name: 'Login', query: { redirect: to.fullPath } })
      return
    }

    // If user is authenticated but user info not loaded, fetch it
    if (authStore.isAuthenticated && !authStore.user) {
      try {
        await authStore.fetchUser()
      } catch (err: unknown) {
        // Distinguish between 401 (needs re-login) and other errors (network issue).
        // The API client attaches the numeric `code` from the server response to the
        // rejected Error, so check that field directly instead of substring-matching
        // the human-readable message (which is locale-dependent and brittle).
        const isAuthError =
          err instanceof Error && 'code' in err && (err as Error & { code?: number }).code === 401
        if (isAuthError) {
          next({ name: 'Login', query: { redirect: to.fullPath } })
        } else {
          // Network or other error - don't clear token, just redirect to login with retry option
          next({ name: 'Login', query: { redirect: to.fullPath, error: 'network' } })
        }
        return
      }
    }

    // Check permission
    const permission = to.meta.permission as { resource: string; action: string } | undefined
    if (permission && authStore.user) {
      if (!authStore.hasPermission(permission.resource, permission.action)) {
        // Show permission denied message
        const { t } = useI18n()
        try { (window as any).$message?.warning(t('common.noPermission')) } catch {}
        next({ name: 'Dashboard' })
        return
      }
    }

    // Redirect to dashboard if already logged in and trying to access login
    if (to.name === 'Login' && authStore.isAuthenticated) {
      next({ name: 'Dashboard' })
      return
    }

    next()
  })
}
