import type { Router } from 'vue-router';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';

/**
 * Goddi RBAC permission guard.
 *
 * Runs after the auth route guard: routes declare
 * `meta.permission = { resource, action }` (configured in build/plugins/router.ts)
 * and access is checked against the permissions returned by /auth/me.
 * Unauthorized navigation is redirected to the 403 page.
 */
export function createPermissionGuard(router: Router) {
  router.beforeEach(to => {
    const permission = to.meta.permission as { resource: string; action: string } | null | undefined;

    if (!permission) {
      return true;
    }

    const authStore = useAuthStore();

    if (authStore.hasPermission(permission.resource, permission.action)) {
      return true;
    }

    window.$message?.warning($t('common.noPermission'));

    return { name: '403' };
  });
}
