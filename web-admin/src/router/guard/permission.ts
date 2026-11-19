import type { Router } from 'vue-router';
import { MessagePlugin } from 'tdesign-vue-next';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';
import { NAV, findNavPermissionMismatches } from '@/components/starter/nav';

let navConsistencyChecked = false;

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
    // Dev-only: a menu item whose permission differs from its route meta can
    // either hide a reachable page or show one the router will reject.
    if (import.meta.env.DEV && !navConsistencyChecked) {
      navConsistencyChecked = true;
      for (const mismatch of findNavPermissionMismatches(NAV, router.getRoutes())) {
        console.warn(`[nav/route permission mismatch] ${mismatch}`);
      }
    }

    const permission = to.meta.permission as { resource: string; action: string } | null | undefined;

    if (!permission) {
      return true;
    }

    const authStore = useAuthStore();

    if (authStore.hasPermission(permission.resource, permission.action)) {
      return true;
    }

    void MessagePlugin.warning($t('common.noPermission'));

    return { name: '403' };
  });
}
